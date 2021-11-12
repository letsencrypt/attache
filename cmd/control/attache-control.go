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
	dc            string
	address       string
	aclToken      string
	tlsEnable     bool
	tlsCACert     string
	tlsCert       string
	tlsKey        string
	tlsSkipVerify bool
}

func (c *consulClientOptions) makeConsulClient() (*api.Client, error) {
	config := api.DefaultConfig()
	config.Datacenter = c.dc
	config.Address = c.address
	config.Token = c.aclToken
	if c.tlsEnable {
		config.Scheme = "https"
		tlsConfig := api.TLSConfig{
			Address:            c.address,
			CAFile:             c.tlsCACert,
			CertFile:           c.tlsCert,
			KeyFile:            c.tlsKey,
			InsecureSkipVerify: c.tlsSkipVerify,
		}
		tlsClientConf, err := api.SetupTLSConfig(&tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating tls client config for consul: %v", err)
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
	log.Println("Attache has started")

	redisNodeAddr := flag.String(
		"redis-node-addr", "", "Address of the localhost Redis server (example: '127.0.0.1:6049')",
	)

	shardPrimaryCount := flag.Int(
		"shard-primary-count", 0, "Total number of expected Redis shard primary nodes",
	)

	attemptToJoinEvery := flag.Duration(
		"attempt-to-join-every",
		5*time.Second,
		"Duration to wait between attempts to join or create a cluster",
	)

	timesToAttempt := flag.Int(
		"times-to-attempt-join",
		20,
		"Number of times to attempt joining or creating a cluster before exiting",
	)

	destServiceName := flag.String(
		"dest-service-name",
		"",
		"Name of the Consul Service containing nodes that this Redis server should attempt to cluster with",
	)

	awaitServiceName := flag.String(
		"await-service-name",
		"",
		"Name of the Consul Service that this node will idle in until it has joined or created a cluster",
	)

	// Consul Flags
	consulDC := flag.String("consul-dc", "", "Path to the Consul CA cert file")
	consulAddr := flag.String(
		"consul-addr",
		"",
		"Address of the localhost Consul client with scheme (example: '127.0.0.1:8501')",
	)
	consulACLToken := flag.String(
		"consul-acl-token",
		"",
		"The contents of the Consul ACL token this client should use",
	)
	consulTLSEnable := flag.Bool("consul-tls-enable", false, "Use TLS for the Consul client connection")
	consulCACert := flag.String("consul-tls-ca-cert", "", "Path to the Consul CA cert file")
	consulCert := flag.String("consul-tls-cert", "", "Path to the Consul client cert file")
	consulKey := flag.String("consul-tls-key", "", "Path to the Consul client key file")
	philTest := flag.Bool("phil-test", false, "This is for Phil!")

	log.Println("Parsing flags")
	flag.Parse()

	consulClientOpts := consulClientOptions{
		dc:        *consulDC,
		address:   *consulAddr,
		aclToken:  *consulACLToken,
		tlsEnable: *consulTLSEnable,
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
			log.Fatalf("cannot query consul for service 'consul': %w", err)
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
	var primaryNodesInDest []string
	var currAttempt int
	ticks := time.Tick(*attemptToJoinEvery)
	for range ticks {
		currAttempt++

		// TODO: Exit loop if some other node won the lock and formed a cluster
		// with this node this will only happen on initial node creation and can
		// be detected with a call to check.ClusterInfo.Ok

		// Check the Consul service catalog for an existing Redis Cluster that
		// we can join. We're limiting the scope of our search to nodes tagged
		// as 'primary', in the `destServiceName` Consul service that are
		// healthy according to Consul health checks.
		destService := check.NewServiceCatalogClient(consulClient, *destServiceName, "primary", true)
		primaryNodesInDest, err = destService.GetAddresses()
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("found nodes %q in service %q\n", primaryNodesInDest, *destServiceName)

		// If 0 existing nodes can be found with this criteria, we know that we
		// need to initialize a new cluster.
		if len(primaryNodesInDest) == 0 {
			createNewCluster = true

			// Check the Consul service catalog for other nodes that are waiting
			// to form a cluster. We're limiting the scope of our search in the
			// `awaitServiceName` Consul service that are healthy according to
			// Consul health checks.
			awaitService := check.NewServiceCatalogClient(consulClient, *awaitServiceName, "", true)
			nodesInAwait, err := awaitService.GetAddresses()
			if err != nil {
				log.Fatalln(err)
			}
			log.Printf("found nodes %q in service %q\n", nodesInAwait, *awaitServiceName)

			// We should only attempt to initialize a new cluster if all of the
			// nodes that we expect in said cluster have finished starting up
			// and reside in the `awaitServiceName` Consul service.
			nodesMissing := *shardPrimaryCount - len(nodesInAwait)
			if nodesMissing == 0 {
				log.Println("all expected shard primary nodes are ready, attempting to initialize cluster")
				break

			}
			log.Printf("cannot initialize cluster, missing %d shard primary nodes\n", nodesMissing)

		} else if len(primaryNodesInDest) < *shardPrimaryCount {
			// The current cluster has less than `shardPrimaryCount` shard
			// primary nodes. This node should be added as a new primary and the
			// existing cluster shardslots should be rebalanced.
			addNodeAsPrimary = true
			log.Println("adding this node as a new shard primary in the existing cluster")

		} else if len(primaryNodesInDest) == *shardPrimaryCount {
			// All `shardPrimaryCount` shard primary nodes exist in the current
			// cluster. This node should be added as a replica to the primary
			// node with the least number of replicas.
			addNodeAsReplica = true
			log.Println("adding this node as a replica in the existing cluster")
		}

		if currAttempt == *timesToAttempt {
			log.Println("failed to join or initialize a cluster during the time permitted")
			break
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

			err := control.RedisCLICreateCluster(consulClient, *awaitServiceName)
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
			// redis-cli --cluster add-node newNodeAddr existingNodeAddr
			// redis-cli --cluster rebalance newNodeAddr --cluster-use-empty-masters
			err := control.RedisCLICreateCluster(consulClient, *awaitServiceName)
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
			log.Printf("attempting to join %q to the cluster that %q belongs to", *redisNodeAddr, primaryNodesInDest[0])
			// redis-cli --cluster add-node newNodeAddr existingNodeAddr

			// Use check.ClusterCheck to fetch the shard primary with the least
			// replicas and grab it's node ID

			// redis-cli --cluster add-node newNodeAddr existingPrimaryNodeAddr --cluster-slave --cluster-master-id existingPrimaryNodeID
			err := control.RedisCLICreateCluster(consulClient, *awaitServiceName)
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
		os.Exit(0)
	}
}
