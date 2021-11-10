package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/check"
	"github.com/letsencrypt/attache/src/control"
)

func newConsulClient() (*api.Client, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func main() {
	log.Println("Attache has started")

	redisNodeAddr := flag.String("redis-node-addr", "", "Name of the Consul Service that this Redis Node should attempt to cluster with")
	destServiceName := flag.String("dest-service-name", "", "Name of the Consul Service that this Redis Node should attempt to cluster with")
	awaitServiceName := flag.String("await-service-name", "", "Name of the Consul Service that this Redis Node will idle in until it's clustered")
	primaryShardCount := flag.Int("primary-shard-count", 0, "Total number of Redis Shard Primary Nodes")
	attemptToJoinEvery := flag.Duration("attempt-to-join-every", 5*time.Second, "Duration to wait between attempts to join a cluster")
	timesToAttempt := flag.Int("times-to-attempt-join", 3, "Numver of times to attempt joining before exiting")

	log.Println("Parsing flags")
	flag.Parse()

	if *redisNodeAddr == "" {
		log.Fatalln("Missing required opt: 'redis-node-addr'")
	}

	if *destServiceName == "" {
		log.Fatalln("Missing required opt: 'dest-service-name'")
	}

	if *awaitServiceName == "" {
		log.Fatalln("Missing required opt: 'await-service-name'")
	}

	if *primaryShardCount == 0 {
		log.Fatalln("Missing required opt: 'primary-shard-count'")
	}

	client, err := newConsulClient()
	if err != nil {
		log.Fatalln(err)
	}

	var createNewCluster bool
	var addNodeAsPrimary bool
	var addNodeAsReplica bool
	var currAttempt int
	ticks := time.Tick(*attemptToJoinEvery)
	for range ticks {
		currAttempt++

		// Check the Consul service catalog for an existing Redis Cluster that
		// we can join. We're limiting the scope of our search to nodes tagged
		// as 'primary', in the `destServiceName` Consul service that are
		// healthy according to Consul health checks.
		destService := check.NewServiceCatalogClient(client, *destServiceName, "primary", true)
		nodesInDest, err := destService.GetAddresses()
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("found nodes %q in service %q\n", nodesInDest, *destServiceName)

		// If 0 existing nodes can be found with this criteria, we know that we
		// need to initialize a new cluster.
		if len(nodesInDest) == 0 {
			createNewCluster = true

			// Check the Consul service catalog for other primary nodes that are
			// waiting to form a cluster. We're limiting the scope of our search
			// in the `awaitServiceName` Consul service that are healthy
			// according to Consul health checks.
			awaitService := check.NewServiceCatalogClient(client, *awaitServiceName, "primary", true)
			nodesInAwait, err := awaitService.GetAddresses()
			if err != nil {
				log.Fatalln(err)
			}
			log.Printf("found nodes %q in service %q\n", nodesInAwait, *awaitServiceName)

			// We should only attempt to initialize a new cluster if all of the
			// nodes that we expect in said cluster have finished starting up
			// and reside in the `awaitServiceName` Consul service.
			nodesMissing := *primaryShardCount - len(nodesInAwait)
			if nodesMissing == 0 {
				log.Println("all expected shard primary nodes are ready, attempting to initialize cluster")
				break

			}
			log.Printf("cannot initialize cluster, missing %d shard primary nodes\n", nodesMissing)

		} else if len(nodesInDest) < *primaryShardCount {
			// A current cluster, less than `primaryShardCount` exists. This
			// node should be added as a new primary and the existing cluster
			// shardslots should be rebalanced.
			addNodeAsPrimary = true
			log.Println("adding this node as a new shard primary in the existing cluster")

		} else if len(nodesInDest) == *primaryShardCount {
			// A current cluster, of `primaryShardCount` nodes or greater,
			// exists. This node should be added as a replica to the primary
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

	session := control.NewConsulLock(client, "service/attache/leader", "15s")
	err = session.Create()
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

			err := control.RedisCLICreateCluster(client, *awaitServiceName)
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
			log.Println("aquired lock, initializing redis cluster")

			err := control.RedisCLICreateCluster(client, *awaitServiceName)
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
			log.Println("aquired lock, initializing redis cluster")

			err := control.RedisCLICreateCluster(client, *awaitServiceName)
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
