package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/check"
	"github.com/letsencrypt/attache/src/control"
)

type consulClientOptions struct {
	dc        string
	address   string
	aclToken  string
	tlsEnable bool
	tlsCACert string
	tlsCert   string
	tlsKey    string
}

func (c *consulClientOptions) makeConsulClient() (*api.Client, error) {
	config := api.DefaultConfig()
	config.Datacenter = c.dc
	config.Address = c.address
	config.Token = c.aclToken
	if c.tlsEnable {
		config.Scheme = "https"
		tlsConfig := api.TLSConfig{
			Address:  c.address,
			CAFile:   c.tlsCACert,
			CertFile: c.tlsCert,
			KeyFile:  c.tlsKey,
		}
		tlsClientConf, err := api.SetupTLSConfig(&tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating TLS client config for consul: %w", err)
		}
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsClientConf,
		}
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func main() {
	log.Println("attache-control has started")
	redisNodeAddr := flag.String("redis-node-addr", "", "redis-server listening address")
	shardPrimaryCount := flag.Int("shard-primary-count", 0, "Total number of expected Redis shard primary nodes")
	attemptToJoinEvery := flag.Duration("attempt-to-join-every", 20*time.Second, "Duration to wait between attempts to join or create a cluster")
	timesToAttempt := flag.Int("times-to-attempt-join", 10, "Number of times to attempt joining or creating a cluster before exiting")
	destServiceName := flag.String("dest-service-name", "", "Consul Service for any existing Redis Cluster Nodes")
	awaitServiceName := flag.String("await-service-name", "", "Consul Service for any newly created Redis Cluster Nodes")

	// Consul Flags
	consulDC := flag.String("consul-dc", "", "Consul client datacenter")
	consulAddr := flag.String("consul-addr", "", "Consul client address")
	consulACLToken := flag.String("consul-acl-token", "", "Consul client ACL token")
	consulEnableTLS := flag.Bool("consul-tls-enable", false, "Enable TLS for the Consul client")
	consulCACert := flag.String("consul-tls-ca-cert", "", "Consul client CA certificate file")
	consulCert := flag.String("consul-tls-cert", "", "Consul client certificate file")
	consulKey := flag.String("consul-tls-key", "", "Consul client key file")
	philTest := flag.Bool("phil-test", false, "This is for Phil!")

	log.Println("Parsing configuration flags")
	flag.Parse()

	if *consulDC == "" {
		log.Fatalln("Missing required opt: 'consul-dc")
	}

	if *consulAddr == "" {
		log.Fatalln("Missing required opt: 'consul-addr")
	}

	if *consulEnableTLS {

		if *consulCACert == "" {
			log.Fatalln("Missing required opt: 'consul-tls-ca-cert")
		}

		if *consulCert == "" {
			log.Fatalln("Missing required opt: 'consul-tls-cert")
		}

		if *consulKey == "" {
			log.Fatalln("Missing required opt: 'consul-tls-key")
		}
	}

	consulClientOpts := consulClientOptions{
		dc:        *consulDC,
		address:   *consulAddr,
		aclToken:  *consulACLToken,
		tlsEnable: *consulEnableTLS,
		tlsCACert: *consulCACert,
		tlsCert:   *consulCert,
		tlsKey:    *consulKey,
	}

	log.Println("Initializing Consul client")
	consulClient, err := consulClientOpts.makeConsulClient()
	if err != nil {
		log.Fatal(err)
	}

	if *philTest {
		log.Println("Beginning Phil Test")
		nodes, _, err := consulClient.Health().Service("consul", "", true, nil)
		if err != nil {
			log.Fatalf("cannot query consul for service 'consul': %s", err)
			os.Exit(2)
		}
		for _, node := range nodes {
			log.Println(node.Node.Address, node.Service.Port)
		}
		os.Exit(0)
	}

	if *redisNodeAddr == "" {
		log.Fatalln("Missing required opt: 'redis-node-addr'")
	}

	if *destServiceName == "" {
		log.Fatalln("Missing required opt: 'dest-service-name'")
	}

	if *awaitServiceName == "" {
		log.Fatalln("Missing required opt: 'await-service-name'")
	}

	if *shardPrimaryCount == 0 {
		log.Fatalln("Missing required opt: 'shard-primary-count'")
	}

	var createNewCluster bool
	var addNodeAsPrimary bool
	var addNodeAsReplica bool
	var nodesInDest []string
	var nodesInAwait []string

	var currAttempt int
	ticks := time.Tick(*attemptToJoinEvery)
	for range ticks {
		currAttempt++

		redisClient := check.NewRedisClient(*redisNodeAddr, "")
		nodeIsNew, err := redisClient.StateNewCheck()
		if err != nil {
			log.Fatalln(err)
		}
		if !nodeIsNew {
			log.Print("Node has already joined a cluster")
			os.Exit(0)
		}

		// Check the Consul service catalog for an existing Redis Cluster that
		// we can join. We're limiting the scope of our search to nodes in the
		// `destServiceName` Consul service that Consul considers healthy.
		destService := check.NewConsulServiceClient(consulClient, *destServiceName, "", true)
		nodesInDest, err = destService.GetNodeAddresses()
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("found nodes %q in service %q\n", nodesInDest, *destServiceName)

		// If 0 existing nodes can be found with this criteria, we know that we
		// need to initialize a new cluster.
		if len(nodesInDest) == 0 {
			createNewCluster = true

			// Check the Consul service catalog for other nodes that are waiting
			// to form a cluster. We're limiting the scope of our search in the
			// `awaitServiceName` Consul service that Consul considers healthy.
			awaitService := check.NewConsulServiceClient(consulClient, *awaitServiceName, "", true)
			nodesInAwait, err = awaitService.GetNodeAddresses()
			if err != nil {
				log.Fatalln(err)
			}
			log.Printf("found nodes %q in service %q\n", nodesInAwait, *awaitServiceName)

			// We should only attempt to initialize a new cluster if all of the
			// nodes that we expect in said cluster have finished starting up
			// and reside in the `awaitServiceName` Consul service.
			nodesMissing := *shardPrimaryCount - len(nodesInAwait)
			if nodesMissing <= 0 {
				log.Println("attempting to initialize a new cluster")
				break

			}
			log.Printf("cannot initialize cluster, missing %d shard primary nodes\n", nodesMissing)

		} else if len(nodesInDest) < *shardPrimaryCount {
			// The current cluster has less than `shardPrimaryCount` shard
			// primary nodes. This node should be added as a new primary and the
			// existing cluster shardslots should be rebalanced.
			addNodeAsPrimary = true
			log.Println("adding this node as a new shard primary in the existing cluster")
			break

		} else if len(nodesInDest) >= *shardPrimaryCount {
			// All `shardPrimaryCount` shard primary nodes exist in the current
			// cluster. This node should be added as a replica to the primary
			// node with the least number of replicas.
			addNodeAsReplica = true
			log.Println("adding this node as a replica in the existing cluster")
			break
		}

		if currAttempt == *timesToAttempt {
			log.Fatalln("failed to join or initialize a cluster during the time permitted")
		}
		log.Printf("continuing to wait, %d attempts remain\n", (*timesToAttempt - currAttempt))
	}

	session := control.NewLock(consulClient, "service/attache/leader", "15s")
	err = session.Initialize()
	if err != nil {
		log.Fatalln(err)
	}
	defer session.Cleanup()

	isLeader, err := session.Acquire()
	if err != nil {
		log.Fatalln(err)
	}

	// If forced to exit early, cleanup our session.
	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	go func() {
		<-catchSignals
		log.Println("interrupted, cleaning up session")

		err := session.Cleanup()
		if err != nil {
			log.Println("failed to cleanup session")
		}
		os.Exit(2)
	}()

	if isLeader {
		log.Println("aquired leader lock")

		// Spin-off a goroutine to periodically renew our leader lock until our
		// work is complete.
		doneChan := make(chan struct{})
		go session.Renew(doneChan)
		log.Println("automatically renewing leader lock until work has been completed")

		if createNewCluster {
			log.Println("initializing a new redis cluster")

			err := control.RedisCLICreateCluster(nodesInAwait)
			if err != nil {
				close(doneChan)
				log.Fatalln(err)
			}
			close(doneChan)

			log.Println("initialization complete, cleaning up session")
			err = session.Cleanup()
			if err != nil {
				log.Println("failed to cleanup session")
			}
			return
		}

		if addNodeAsPrimary {
			log.Printf("adding node %q as a new shard primary", *redisNodeAddr)
			log.Printf("attempting to join %q to the cluster that %q belongs to", *redisNodeAddr, nodesInDest[0])
			err := control.RedisCLIAddNewShardPrimary(*redisNodeAddr, nodesInDest[0])
			if err != nil {
				close(doneChan)
				log.Fatalln(err)
			}
			close(doneChan)

			log.Println("initialization complete, cleaning up session")
			err = session.Cleanup()
			if err != nil {
				log.Println("failed to cleanup session")
			}
			return
		}

		if addNodeAsReplica {
			log.Printf("adding node %q as a new shard replica", *redisNodeAddr)
			log.Printf("attempting to join %q to the cluster that %q belongs to", *redisNodeAddr, nodesInDest[0])
			err := control.RedisCLIAddNewShardReplica(*redisNodeAddr, nodesInDest[0])
			if err != nil {
				close(doneChan)
				log.Fatalln(err)
			}
			close(doneChan)

			log.Println("initialization complete, cleaning up session")
			err = session.Cleanup()
			if err != nil {
				log.Println("failed to cleanup session")
			}
			return
		}
	} else {
		log.Println("leader already chosen")
		os.Exit(2)
	}
}
