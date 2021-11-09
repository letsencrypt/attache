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

	currAttempt := 0
	maxAttempts := 3
	ticks := time.Tick(5 * time.Second)
	for range ticks {
		currAttempt++
		if currAttempt > maxAttempts {
			log.Printf("no nodes appeared in service %q\n", *awaitServiceName)
			break
		}

		check := check.NewServiceCatalogClient(client, *awaitServiceName, "primary", true)
		nodesInAwait, err := check.GetAddresses()
		if err != nil {
			log.Fatalf("cannot query consul for service %q\n", *awaitServiceName)
		}
		log.Printf("found nodes %q in service %q\n", nodesInAwait, *awaitServiceName)

		nodesMissing := *primaryShardCount - len(nodesInAwait)
		if nodesMissing == 0 {
			log.Println("all expected shard primary nodes are ready, attempting to initialize cluster")
			break
		} else {
			log.Printf("missing %d shard primary nodes, continuing to wait\n", nodesMissing)
		}
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
		log.Println("aquired lock, initializing redis cluster")

		// Spin off a go-routine to renew our session until cluster
		// initialization is complete.
		doneChan := make(chan struct{})
		go session.Renew(doneChan)

		err := control.RedisCLICreateCluster(client, *awaitServiceName)
		if err != nil {
			close(doneChan)
			log.Fatal(err)
		}
		close(doneChan)

		log.Println("initialization complete, cleaning up session")
		err = session.Cleanup()
		if err != nil {
			log.Println("failed to cleanup session")
		}
		return
	} else {
		log.Println("leader already chosen")
		os.Exit(0)
	}
}
