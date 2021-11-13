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
	redisNodeAddr := flag.String("redis-node-addr", "", "redis-server listening address")
	shardPrimaryCount := flag.Int("shard-primary-count", 0, "Total number of expected Redis shard primary nodes")
	attemptToJoinEvery := flag.Duration("attempt-to-join-every", 3*time.Second, "Duration to wait between attempts to join or create a cluster")
	timesToAttempt := flag.Int("times-to-attempt-join", 20, "Number of times to attempt joining or creating a cluster before exiting")
	destServiceName := flag.String("dest-service-name", "", "Consul Service for any existing Redis Cluster Nodes")
	awaitServiceName := flag.String("await-service-name", "", "Consul Service for any newly created Redis Cluster Nodes")
	consulDC := flag.String("consul-dc", "", "Consul client datacenter")
	consulAddr := flag.String("consul-addr", "", "Consul client address")
	consulACLToken := flag.String("consul-acl-token", "", "Consul client ACL token")
	consulEnableTLS := flag.Bool("consul-tls-enable", false, "Enable TLS for the Consul client")
	consulCACert := flag.String("consul-tls-ca-cert", "", "Consul client CA certificate file")
	consulCert := flag.String("consul-tls-cert", "", "Consul client certificate file")
	consulKey := flag.String("consul-tls-key", "", "Consul client key file")

	log.Print("Starting...")
	start := time.Now()
	log.Print("Parsing configuration flags")
	flag.Parse()

	if *consulDC == "" {
		log.Fatal("Missing required opt: 'consul-dc")
	}

	if *consulAddr == "" {
		log.Fatal("Missing required opt: 'consul-addr")
	}

	if *consulEnableTLS {

		if *consulCACert == "" {
			log.Fatal("Missing required opt: 'consul-tls-ca-cert")
		}

		if *consulCert == "" {
			log.Fatal("Missing required opt: 'consul-tls-cert")
		}

		if *consulKey == "" {
			log.Fatal("Missing required opt: 'consul-tls-key")
		}
	}

	if *redisNodeAddr == "" {
		log.Fatal("Missing required opt: 'redis-node-addr'")
	}

	if *destServiceName == "" {
		log.Fatal("Missing required opt: 'dest-service-name'")
	}

	if *awaitServiceName == "" {
		log.Fatal("Missing required opt: 'await-service-name'")
	}

	if *shardPrimaryCount == 0 {
		log.Fatal("Missing required opt: 'shard-primary-count'")
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

	log.Print("Initializing Consul client")
	consulClient, err := consulClientOpts.makeConsulClient()
	if err != nil {
		log.Fatal(err)
	}

	var nodesInDest []string
	var nodesInAwait []string

	var currAttempt int
	var ticks = time.Tick(*attemptToJoinEvery)
	for range ticks {
		currAttempt++
		redisClient := check.NewRedisClient(*redisNodeAddr, "")
		nodeIsNew, err := redisClient.StateNewCheck()
		if err != nil {
			log.Fatal(err)
		}
		if !nodeIsNew {
			log.Print("Node has already joined a cluster")
			break
		}

		session := control.NewLock(consulClient, "service/attache/leader", "10s")
		err = session.Initialize()
		if err != nil {
			log.Fatal(err)
		}

		nodeHasLock, err := session.Acquire()
		if err != nil {
			log.Fatal(err)
		}

		// If forced to exit early, cleanup our session.
		catchSignals := make(chan os.Signal, 1)
		signal.Notify(catchSignals, os.Interrupt)
		go func() {
			<-catchSignals
			log.Print("interrupted, cleaning up session")

			err := session.Cleanup()
			if err != nil {
				log.Print("failed to cleanup session")
			}
			os.Exit(1)
		}()

		var completed bool
		if nodeHasLock {
			log.Print("aquired lock")

			// Spin-off a goroutine to periodically renew our leader lock until
			// our work is complete.
			doneChan := make(chan struct{})
			go session.Renew(doneChan)

			// Check the Consul service catalog for an existing Redis Cluster
			// that we can join. We're limiting the scope of our search to nodes
			// in the `destServiceName` Consul service that Consul considers
			// healthy.
			destService := check.NewConsulServiceClient(consulClient, *destServiceName, "", true)
			nodesInDest, err = destService.GetNodeAddresses()
			if err != nil {
				close(doneChan)
				session.Cleanup()
				log.Fatal(err)
			}
			log.Printf("found nodes %q in service %q", nodesInDest, *destServiceName)

			// If 0 existing nodes can be found with this criteria, we know that
			// we need to initialize a new cluster.
			if len(nodesInDest) == 0 {
				// Check the Consul service catalog for other nodes that are
				// waiting to form a cluster. We're limiting the scope of our
				// search to nodes in the `awaitServiceName` Consul service that
				// Consul considers healthy.
				awaitService := check.NewConsulServiceClient(consulClient, *awaitServiceName, "", true)
				nodesInAwait, err = awaitService.GetNodeAddresses()
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("found nodes %q in service %q", nodesInAwait, *awaitServiceName)

				// We should only attempt to initialize a new cluster if all of
				// the nodes that we expect in said cluster have finished
				// starting up and reside in the `awaitServiceName` Consul
				// service.
				nodesMissing := *shardPrimaryCount - len(nodesInAwait)
				if nodesMissing <= 0 {
					log.Printf("creating new cluster with nodes %q", nodesInAwait[0:*shardPrimaryCount])

					err := control.RedisCLICreateCluster(nodesInAwait[0:*shardPrimaryCount])
					if err != nil {
						close(doneChan)
						session.Cleanup()
						log.Fatal(err)
					} else {
						completed = true
					}

				}

			} else if len(nodesInDest) < *shardPrimaryCount {
				// The current cluster has less than `shardPrimaryCount` shard
				// primary nodes. This node should be added as a new primary and
				// the existing cluster shardslots should be rebalanced.
				log.Printf("adding node %q as a new shard primary", *redisNodeAddr)
				log.Printf("attempting to join %q to the cluster that %q belongs to", *redisNodeAddr, nodesInDest[0])

				err := control.RedisCLIAddNewShardPrimary(*redisNodeAddr, nodesInDest[0])
				if err != nil {
					log.Print("issue encountered, releasing lock")
					close(doneChan)
					session.Cleanup()
					log.Fatal(err)
				} else {
					completed = true
				}

			} else if len(nodesInDest) >= *shardPrimaryCount {
				// All `shardPrimaryCount` shard primary nodes exist in the
				// current cluster. This node should be added as a replica to
				// the primary node with the least number of replicas.
				log.Printf("adding node %q as a new shard replica", *redisNodeAddr)
				log.Printf("attempting to join %q to the cluster that %q belongs to", *redisNodeAddr, nodesInDest[0])

				err := control.RedisCLIAddNewShardReplica(*redisNodeAddr, nodesInDest[0])
				if err != nil {
					log.Print("issue encountered, releasing lock")
					close(doneChan)
					session.Cleanup()
					log.Fatal(err)
				} else {
					completed = true
				}

			}
			// Close our channel and cleanup our Consul session so that other
			// attache nodes can pickup the lock to complete their work.
			log.Print("releasing lock")
			close(doneChan)
			session.Cleanup()

			// If we've completed our work, let's exit.
			if completed {
				log.Print("completed successfully")
				break
			}
		} else {
			if currAttempt == *timesToAttempt {
				log.Fatal("failed to join or initialize a cluster during the time permitted")
			}
			log.Print("another node currently has the lock")
			log.Printf("continuing to wait, %d attempts remain", (*timesToAttempt - currAttempt))
		}
	}

	// TODO: Remove once https://github.com/hashicorp/nomad/issues/10058 has
	// been solved.
	duration := time.Since(start)
	if duration < time.Minute*10 {
		time.Sleep(time.Minute*10 - duration)
	}
	log.Print("exiting...")
}
