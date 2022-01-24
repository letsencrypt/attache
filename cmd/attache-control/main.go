package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	consul "github.com/letsencrypt/attache/src/consul/client"
	lockClient "github.com/letsencrypt/attache/src/consul/lock"
	redisCLI "github.com/letsencrypt/attache/src/redis/cli"
	redis "github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
	logger "github.com/sirupsen/logrus"
)

var errContinue = errors.New("continuing")

func setLogLevel(level string) {
	parsedLevel, err := logger.ParseLevel(level)
	if err != nil {
		logger.Fatalf("initializing: %s is not a valid log-level: %s", level, err)
	}
	logger.SetLevel(parsedLevel)
}

type leader struct {
	cliOpts
	lock         *lockClient.Lock
	scalingOpts  *consul.ScalingOpts
	destClient   *consul.Client
	nodesInDest  []string
	nodesInAwait []string
}

func (l *leader) createNewRedisCluster() error {
	// Check the Consul service catalog for other nodes that are waiting to form
	// a cluster. We're limiting the scope of our search to nodes in the
	// awaitClient Consul service that Consul considers healthy.
	awaitClient, err := consul.New(l.ConsulOpts, l.awaitServiceName)
	if err != nil {
		return err
	}

	l.nodesInAwait, err = awaitClient.GetNodeAddresses(true)
	if err != nil {
		return err
	}
	numNodesInAwait := len(l.nodesInAwait)
	logger.Infof("found %d nodes in consul service %s", numNodesInAwait, l.awaitServiceName)

	// We should only attempt to initialize a new cluster if all of the nodes
	// that we expect in said cluster have finished starting up and reside in
	// the awaitService Consul service.
	if l.scalingOpts.NodesMissing(numNodesInAwait) >= 1 {
		return fmt.Errorf("still waiting for nodes to startup, releasing lock: %w", errContinue)

	} else {
		var nodesToCluster []string
		if l.scalingOpts.ReplicasPerPrimary() == 0 {
			// This handles a special case for clusters that are started with
			// less than enough replicas to give at least one to each primary.
			// Once the first primary only cluster is started and the lock is
			// released our remaining replica nodes will be able to add
			// themselves to the newly created cluster.
			nodesToCluster = l.nodesInAwait[:l.scalingOpts.PrimaryCount]
		} else {
			nodesToCluster = l.nodesInAwait
		}

		logger.Infof("attempting to create a new cluster with nodes %s", strings.Join(nodesToCluster, " "))
		err := redisCLI.CreateCluster(l.RedisOpts, nodesToCluster, l.scalingOpts.ReplicasPerPrimary())
		if err != nil {
			return err
		}
		return nil
	}
}

func (l *leader) joinOrCreateRedisCluster() error {
	logger.Info("attempting to join or create a cluster")

	// Check the Consul service catalog for an existing Redis Cluster that we
	// can join. We're limiting the scope of our search to nodes in the
	// destService Consul service that Consul considers healthy.
	var err error
	l.nodesInDest, err = l.destClient.GetNodeAddresses(true)
	if err != nil {
		return err
	}
	numNodesInDest := len(l.nodesInDest)

	// If no existing nodes can be found with this criteria, we know that we
	// need to initialize a new cluster.
	if numNodesInDest <= 0 {
		err = l.createNewRedisCluster()
		if err != nil {
			return err
		}
		logger.Info("new cluster created successfully")
		return nil
	}
	existingClusterNode := l.nodesInDest[0]
	logger.Infof("found %d cluster nodes in consul service %s", numNodesInDest, l.destServiceName)

	logger.Infof("gathering info from the cluster that %s belongs to", existingClusterNode)
	clusterClient, err := redis.New(config.RedisOpts{
		NodeAddr:       existingClusterNode,
		Username:       l.RedisOpts.Username,
		PasswordConfig: l.RedisOpts.PasswordConfig,
		TLSConfig:      l.RedisOpts.TLSConfig,
	})
	if err != nil {
		return err
	}

	primaryNodesInCluster, err := clusterClient.GetPrimaryNodes()
	if err != nil {
		return err
	}

	replicaNodesInCluster, err := clusterClient.GetReplicaNodes()
	if err != nil {
		return err
	}

	if len(primaryNodesInCluster) < l.scalingOpts.PrimaryCount {
		// The current cluster has less than the expected shard primary nodes.
		// This node should be added as a new primary and the existing cluster
		// shardslots should be rebalanced.
		logger.Infof("%s should be added as a shard primary", l.RedisOpts.NodeAddr)
		logger.Infof("attempting to add %s to the cluster that %s belongs to", l.RedisOpts.NodeAddr, existingClusterNode)
		err := redisCLI.AddNewShardPrimary(l.RedisOpts, existingClusterNode)
		if err != nil {
			return err
		}
		logger.Infof("%s was successfully added as a shard primary", l.RedisOpts.NodeAddr)
		return nil

	} else if len(replicaNodesInCluster) < l.scalingOpts.ReplicaCount {
		// All expected shard primary nodes exist in the current cluster. This
		// node should be added as a replica to the primary node with the least
		// number of replicas.
		logger.Infof("%s should be added as a new shard replica", l.RedisOpts.NodeAddr)
		logger.Infof("attempting to add %s to the cluster that %s belongs to", l.RedisOpts.NodeAddr, existingClusterNode)
		err := redisCLI.AddNewShardReplica(l.RedisOpts, existingClusterNode)
		if err != nil {
			return err
		}
		logger.Infof("%s was successfully added as a shard replica", l.RedisOpts.NodeAddr)
		return nil
	}

	// This should never happen as long as the job and scaling opts match.
	return fmt.Errorf("%s couldn't be added to an existing cluster", l.RedisOpts.NodeAddr)
}

func attemptLeaderLock(c cliOpts, scaling *consul.ScalingOpts, dest *consul.Client) error {
	lock, err := lockClient.New(c.ConsulOpts, c.lockPath, "10s")
	if err != nil {
		return err
	}
	defer lock.Cleanup()

	lockAcquired, err := lock.Acquire()
	if err != nil {
		return err
	}

	if !lockAcquired {
		return fmt.Errorf("another node currently has the lock: %w", errContinue)
	}

	logger.Info("acquired the lock")
	leader := &leader{
		cliOpts:     c,
		lock:        lock,
		scalingOpts: scaling,
		destClient:  dest,
	}
	return leader.joinOrCreateRedisCluster()
}

func main() {
	c := ParseFlags()
	err := c.Validate()
	if err != nil {
		logger.Fatal(err)
	}

	setLogLevel(c.logLevel)
	logger.Infof("starting %s", os.Args[0])

	logger.Info("initializing a new redis client")
	thisNode, err := redis.New(c.RedisOpts)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Info("initializing a new consul client")
	dest, err := consul.New(c.ConsulOpts, c.destServiceName)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Infof("fetching scaling options from consul path 'service/%s/scaling'", c.destServiceName)
	scaling, err := dest.GetScalingOpts()
	if err != nil {
		logger.Fatal(err)
	}

	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)

	ticker := time.NewTicker(c.attemptInterval)
	done := make(chan bool, 1)

	go func() {
		for {
			select {
			case <-done:
				// Exit.
				return

			case <-catchSignals:
				// Gracefully shutdown.
				ticker.Stop()
				done <- true

			case <-ticker.C:
				// Attempt to join or modify a cluster.
				thisNodeIsNew, err := thisNode.IsNew()
				if err != nil {
					logger.Errorf("while attempting to check that status of %s: %s", c.RedisOpts.NodeAddr, err)
					continue
				}
				if !thisNodeIsNew {
					logger.Info("this node is already part of an existing cluster")

					// Stop the ticker and run until killed due to:
					// https://github.com/hashicorp/nomad/issues/10058
					ticker.Stop()
					logger.Info("running until killed...")
					continue
				}
				err = attemptLeaderLock(c, scaling, dest)
				if err != nil {
					if errors.Is(err, errContinue) {
						logger.Info(err)
						continue
					}
					logger.Errorf("while attempting to join or create a cluster: %s", err)
					continue
				}
				continue
			}

		}
	}()
	<-done
	logger.Info("exiting...")
}
