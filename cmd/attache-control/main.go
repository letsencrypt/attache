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

		logger.Infof("attempting to create a new cluster with nodes %q", strings.Join(nodesToCluster, " "))
		err := redisCLI.CreateCluster(l.RedisOpts, nodesToCluster, l.scalingOpts.ReplicasPerPrimary())
		if err != nil {
			return err
		}
		return nil
	}
}

func (l *leader) joinOrCreateRedisCluster() error {
	logger.Info("attempting to join or create a cluster")
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
	existingNode := l.nodesInDest[0]
	logger.Infof("found %d cluster nodes in consul service %q", numNodesInDest, l.destServiceName)

	logger.Infof("gathering info from the cluster that %q belongs to", existingNode)
	clusterClient, err := redisClient.New(config.RedisOpts{
		NodeAddr:       existingNode,
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
		logger.Infof("attempting to add %q to the cluster that %q belongs to", l.RedisOpts.NodeAddr, existingNode)
		err := redisCLI.AddNewShardPrimary(l.RedisOpts, existingNode)
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
		logger.Infof("attempting to add %q to the cluster that %q belongs to", l.RedisOpts.NodeAddr, existingNode)
		err := redisCLI.AddNewShardReplica(l.RedisOpts, existingNode)
		if err != nil {
			return err
		}
		logger.Infof("%q was successfully added as a shard replica", l.RedisOpts.NodeAddr)
		return nil
	}
	// This should not happen, attache doesn't currently support adding nodes
	// with 0 hash slots.
	logger.Info("nothing to do")
	return nil
}

func attemptLock(c cliOpts, scaling *consulClient.ScalingOpts, dest *consulClient.Client) error {
	lock, err := lockClient.New(c.ConsulOpts, c.lockPath, "10s")
	if err != nil {
		return err
	}

	lockAcquired, err := lock.Acquire()
	if err != nil {
		return err
	}

	if !lockAcquired {
		return fmt.Errorf("another node currently has the lock: %w", errContinue)
	}

	logger.Info("acquired leader lock")
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
	thisNode, err := redisClient.New(c.RedisOpts)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Info("initializing a new consul client")
	dest, err := consulClient.New(c.ConsulOpts, c.destServiceName)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Infof("fetching scaling options from consul path 'service/%s/scaling'", c.destServiceName)
	scaling, err := dest.GetScalingOpts()
	if err != nil {
		logger.Fatal(err)
	}

	// If forced to exit early, cleanup our session.
	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)

	ticker := time.NewTicker(c.attemptInterval)
	done := make(chan bool, 1)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-catchSignals:
				ticker.Stop()
				done <- true
			case <-ticker.C:
				isNew, err := thisNode.IsNew()
				if err != nil {
					logger.Errorf("while attempting to check that status of %q: %s", c.RedisOpts.NodeAddr, err)
					continue
				}
				if !isNew {
					logger.Info("this node is already part of an existing cluster")
					continue
				}
				err = attemptLock(c, scaling, dest)
				if err != nil {
					if errors.Is(err, errContinue) {
						logger.Info(err)
						continue
					}
					logger.Errorf("while attempting to join or create a cluster: %s", err)
					continue
				}
				logger.Info("no work to perform")
			}
		}
	}()
	<-done
	logger.Info("exiting...")
}
