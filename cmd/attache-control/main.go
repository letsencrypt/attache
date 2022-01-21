package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	consulClient "github.com/letsencrypt/attache/src/consul/client"
	lockClient "github.com/letsencrypt/attache/src/consul/lock"
	redisCLI "github.com/letsencrypt/attache/src/redis/cli"
	redisClient "github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
	logger "github.com/sirupsen/logrus"
)

var ErrContinue = errors.New("Continue")

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
	scalingOpts  *consulClient.ScalingOpts
	destClient   *consulClient.Client
	nodesInDest  []string
	nodesInAwait []string
}

// periodicallyRenewLock spins off a goroutine to periodically renew our leader
// lock until our work is complete and returns a cleanup func that can be called
// to delete the Consul session and release the lock.
func (l *leader) periodicallyRenewLock() func() {
	doneChan := make(chan struct{})
	go l.lock.Renew(doneChan)
	return func() {
		// Stop renewing the lock session.
		close(doneChan)
		l.lock.Cleanup()
	}
}

func (l *leader) createNewRedisCluster() error {
	// Check the Consul service catalog for other nodes that are waiting to form
	// a cluster. We're limiting the scope of our search to nodes in the
	// awaitClient Consul service that Consul considers healthy.
	awaitClient, err := consulClient.New(l.ConsulOpts, l.awaitServiceName)
	if err != nil {
		return err
	}

	l.nodesInAwait, err = awaitClient.GetNodeAddresses(true)
	if err != nil {
		return err
	}
	numNodesInAwait := len(l.nodesInAwait)
	logger.Infof("found %d nodes in consul service %q", numNodesInAwait, l.awaitServiceName)

	// We should only attempt to initialize a new cluster if all of the nodes
	// that we expect in said cluster have finished starting up and reside in
	// the awaitService Consul service.
	if l.scalingOpts.NodesMissing(numNodesInAwait) >= 1 {
		logger.Info("still waiting for nodes to startup, releasing lock")
		return ErrContinue

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

		logger.Infof("attempting to create a new cluster with nodes %q", strings.Join(nodesToCluster, " "))
		err := redisCLI.CreateCluster(l.RedisOpts, nodesToCluster, l.scalingOpts.ReplicasPerPrimary())
		if err != nil {
			return err
		}
		return nil
	}
}

func (l *leader) joinOrCreateRedisCluster() error {
	cleanup := l.periodicallyRenewLock()
	defer cleanup()

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
	logger.Infof("found %d cluster nodes in consul service %q", numNodesInDest, l.destServiceName)

	logger.Infof("gathering info from the cluster that %q belongs to", existingClusterNode)
	clusterClient, err := redisClient.New(config.RedisOpts{
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
		logger.Infof("%q should be added as a shard primary", l.RedisOpts.NodeAddr)
		logger.Infof("attempting to add %q to the cluster that %q belongs to", l.RedisOpts.NodeAddr, existingClusterNode)
		err := redisCLI.AddNewShardPrimary(l.RedisOpts, existingClusterNode)
		if err != nil {
			return err
		}
		logger.Infof("%q was successfully added as a shard primary", l.RedisOpts.NodeAddr)
		return nil

	} else if len(replicaNodesInCluster) < l.scalingOpts.ReplicaCount {
		// All expected shard primary nodes exist in the current cluster. This
		// node should be added as a replica to the primary node with the least
		// number of replicas.
		logger.Infof("%q should be added as a new shard replica", l.RedisOpts.NodeAddr)
		logger.Infof("attempting to add %q to the cluster that %q belongs to", l.RedisOpts.NodeAddr, existingClusterNode)
		err := redisCLI.AddNewShardReplica(l.RedisOpts, existingClusterNode)
		if err != nil {
			return err
		}
		logger.Infof("%q was successfully added as a shard replica", l.RedisOpts.NodeAddr)
		return nil
	}
	// This should not happen, attache doesn't currently support adding nodes
	// with 0 hash slots.
	return fmt.Errorf("%q coud not be added as a shard primary or replica", l.RedisOpts.NodeAddr)
}

func main() {
	start := time.Now()
	c := ParseFlags()
	err := c.Validate()
	if err != nil {
		logger.Fatal(err)
	}

	setLogLevel(c.logLevel)
	logger.Infof("starting %s", os.Args[0])

	logger.Info("initializing a new redis client")
	newNodeClient, err := redisClient.New(c.RedisOpts)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Info("initializing a new consul client")
	destClient, err := consulClient.New(c.ConsulOpts, c.destServiceName)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Infof("fetching scaling options from consul path 'service/%s/scaling'", c.destServiceName)
	scalingOpts, err := destClient.GetScalingOpts()
	if err != nil {
		logger.Fatal(err)
	}

	var attemptCount int
	ticker := time.NewTicker(c.attemptInterval)

	lock, err := lockClient.New(c.ConsulOpts, c.lockPath, "10s")
	if err != nil {
		logger.Fatal(err)
	}

	// If forced to exit early, cleanup our session.
	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	go func() {
		<-catchSignals
		ticker.Stop()
		lock.Cleanup()
		logger.Fatal("operation interrupted, cleaning up session and exiting")
	}()

	for range ticker.C {
		attemptCount++

		nodeIsNew, err := newNodeClient.StateNewCheck()
		if err != nil {
			logger.Fatal(err)
		}
		if !nodeIsNew {
			logger.Info("this node has already joined a cluster")
			break
		}

		lockAcquired, err := lock.Acquire()
		if err != nil {
			logger.Fatal(err)
		}

		if !lockAcquired {
			logger.Info("another node currently has the lock")
			if attemptCount >= c.attemptLimit {
				logger.Fatal("failed to join or create a cluster after %q attempts", attemptCount)
			}
			logger.Infof("continuing to wait, %d attempts remain", (c.attemptLimit - attemptCount))
		}

		logger.Info("acquired leader lock")
		leader := &leader{
			cliOpts:     c,
			lock:        lock,
			scalingOpts: scalingOpts,
			destClient:  destClient,
		}
		logger.Info("attempting to join or create a cluster")
		err = leader.joinOrCreateRedisCluster()
		if err != nil {
			if errors.Is(err, ErrContinue) {
				logger.Info(err)
				continue
			}
			logger.Fatalf("while attempting to join or create a cluster: %s", err)
		}
		break
	}
	ticker.Stop()

	// TODO: Remove once https://github.com/hashicorp/nomad/issues/10058 has
	// been solved. Nomad Post-Start tasks need to stay healthy for at least 10s
	// after the Main Tasks are marked healthy.
	duration := time.Since(start)
	minHealthyTime := time.Second * 30
	if duration < minHealthyTime {
		timeToWait := minHealthyTime - duration
		logger.Infof("waiting %s to exit", timeToWait.String())
		time.Sleep(timeToWait)
	}
	logger.Info("exiting...")
}
