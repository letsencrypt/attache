package main

import (
	"os"
	"os/signal"
	"time"

	"github.com/letsencrypt/attache/cmd/config"
	"github.com/letsencrypt/attache/src/check"
	"github.com/letsencrypt/attache/src/control"
	logger "github.com/sirupsen/logrus"
)

func setLogLevel(level string) {
	parsedLevel, err := logger.ParseLevel(level)
	if err != nil {
		logger.Fatalf("%s is not a valid log-level: %s", level, err)
	}
	logger.SetLevel(parsedLevel)
}

func main() {
	start := time.Now()
	conf := config.ParseFlags()
	config.ValidateConfig(conf)
	setLogLevel(conf.LogLevel)
	logger.Info("Starting...")

	logger.Info("Initializing Consul client")
	consulClient, err := conf.ConsulOpts.MakeConsulClient()
	if err != nil {
		logger.Fatal(err)
	}

	var nodesInDest []string
	var nodesInAwait []string

	var attemptCount int
	var ticks = time.Tick(conf.AttemptInterval)
	for range ticks {
		attemptCount++
		redisClient := check.NewRedisClient(conf.RedisNodeAddr, "")
		nodeIsNew, err := redisClient.StateNewCheck()
		if err != nil {
			logger.Fatal(err)
		}

		if !nodeIsNew {
			logger.Info("node has already joined a cluster")
			break
		}

		session := control.NewLock(consulClient, "service/attache/leader", "10s")
		err = session.Initialize()
		if err != nil {
			logger.Fatal(err)
		}

		nodeHasLock, err := session.Acquire()
		if err != nil {
			logger.Fatal(err)
		}

		// If forced to exit early, cleanup our session.
		catchSignals := make(chan os.Signal, 1)
		signal.Notify(catchSignals, os.Interrupt)
		go func() {
			<-catchSignals
			logger.Error("operation interrupted, cleaning up session and exiting")

			err := session.Cleanup()
			if err != nil {
				logger.Error("failed to cleanup session")
			}
			os.Exit(1)
		}()

		var completed bool
		if nodeHasLock {
			logger.Info("aquired lock")

			// Spin-off a goroutine to periodically renew our leader lock until
			// our work is complete.
			doneChan := make(chan struct{})
			go session.Renew(doneChan)

			cleanup := func() {
				close(doneChan)
				session.Cleanup()

			}

			// Check the Consul service catalog for an existing Redis Cluster
			// that we can join. We're limiting the scope of our search to nodes
			// in the destService Consul service that Consul considers healthy.
			destService := check.NewConsulServiceClient(consulClient, conf.DestServiceName, "", true)
			nodesInDest, err = destService.GetNodeAddresses()
			if err != nil {
				cleanup()
				logger.Fatal(err)
			}
			logger.Infof("found nodes %q in service %q", nodesInDest, conf.DestServiceName)

			// If 0 existing nodes can be found with this criteria, we know that
			// we need to initialize a new cluster.
			if len(nodesInDest) == 0 {
				// Check the Consul service catalog for other nodes that are
				// waiting to form a cluster. We're limiting the scope of our
				// search to nodes in the awaitService Consul service that
				// Consul considers healthy.
				awaitService := check.NewConsulServiceClient(consulClient, conf.AwaitServiceName, "", true)
				nodesInAwait, err = awaitService.GetNodeAddresses()
				if err != nil {
					logger.Fatal(err)
				}
				logger.Infof("found nodes %q in service %q", nodesInAwait, conf.AwaitServiceName)

				// We should only attempt to initialize a new cluster if all of
				// the nodes that we expect in said cluster have finished
				// starting up and reside in the awaitService Consul service.
				nodesMissing := (conf.RedisPrimaryCount + conf.RedisReplicaCount) - len(nodesInAwait)
				if nodesMissing == 0 {
					logger.Infof("creating new cluster with nodes %q", nodesInAwait)

					err := control.RedisCLICreateCluster(nodesInAwait, (conf.RedisReplicaCount / conf.RedisPrimaryCount))
					if err != nil {
						cleanup()
						logger.Fatal(err)
					} else {
						completed = true
					}
				}
				logger.Infof("still waiting for %d nodes to startup, relinquishing lock", nodesMissing)
				cleanup()
				continue

			}

			clusterClient := check.NewRedisClient(nodesInDest[0], "")
			primaryNodesInCluster, err := clusterClient.GetPrimaryNodes()
			if err != nil {
				cleanup()
				logger.Fatal(err)
			}
			replicaNodesInCluster, err := clusterClient.GetReplicaNodes()
			if err != nil {
				cleanup()
				logger.Fatal(err)
			}

			logger.Info(conf.RedisReplicaCount, conf.RedisPrimaryCount)
			logger.Info(len(replicaNodesInCluster), len(primaryNodesInCluster))

			if len(primaryNodesInCluster) < conf.RedisPrimaryCount {
				// The current cluster has less than `shardPrimaryCount` shard
				// primary nodes. This node should be added as a new primary and
				// the existing cluster shardslots should be rebalanced.
				logger.Infof("adding node %q as a new shard primary", conf.RedisNodeAddr)
				logger.Infof("attempting to join %q to the cluster that %q belongs to", conf.RedisNodeAddr, nodesInDest[0])

				err := control.RedisCLIAddNewShardPrimary(conf.RedisNodeAddr, nodesInDest[0])
				if err != nil {
					cleanup()
					logger.Fatal(err)
				} else {
					completed = true
				}

			}

			if len(replicaNodesInCluster) < conf.RedisReplicaCount {
				// All `shardPrimaryCount` shard primary nodes exist in the
				// current cluster. This node should be added as a replica to
				// the primary node with the least number of replicas.
				logger.Infof("adding node %q as a new shard replica", conf.RedisNodeAddr)
				logger.Infof("attempting to join %q to the cluster that %q belongs to", conf.RedisNodeAddr, nodesInDest[0])

				err := control.RedisCLIAddNewShardReplica(conf.RedisNodeAddr, nodesInDest[0])
				if err != nil {
					cleanup()
					logger.Fatal(err)
				} else {
					completed = true
				}

			}

			logger.Info("releasing lock")
			cleanup()

			// If we've completed our work, let's exit.
			if completed {
				logger.Info("completed successfully")
				break
			}
		} else {
			if attemptCount == conf.AttemptLimit {
				logger.Fatal("failed to join or initialize a cluster during the time permitted")
			}
			logger.Info("another node currently has the lock")
			logger.Infof("continuing to wait, %d attempts remain", (conf.AttemptLimit - attemptCount))
		}
	}

	// TODO: Remove once https://github.com/hashicorp/nomad/issues/10058 has
	// been solved.
	duration := time.Since(start)
	if duration < time.Minute*10 {
		time.Sleep(time.Minute*10 - duration)
	}
	logger.Info("exiting...")
}
