package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
	logger "github.com/sirupsen/logrus"
)

func makeAuthArgs(conf config.RedisOpts) ([]string, error) {
	password, err := conf.LoadPassword()
	if err != nil {
		return nil, err
	}

	return []string{
		"--user",
		conf.Username,
		"--pass",
		password,
	}, nil
}

func makeTLSArgs(conf config.RedisOpts) ([]string, error) {
	_, err := conf.LoadTLS()
	if err != nil {
		return nil, err
	}

	return []string{
		"--tls",
		"--cert",
		conf.TLSConfig.CertFile,
		"--key",
		conf.TLSConfig.KeyFile,
		"--cacert",
		conf.TLSConfig.CACertFile,
	}, nil
}

func execute(conf config.RedisOpts, command []string) error {
	redisCli, err := exec.LookPath("redis-cli")
	if err != nil {
		return err
	}

	tlsArgs, err := makeTLSArgs(conf)
	if err != nil {
		return err
	}
	command = append(command, tlsArgs...)

	authArgs, err := makeAuthArgs(conf)
	if err != nil {
		return err
	}
	command = append(command, authArgs...)

	cmd := &exec.Cmd{
		Path:   redisCli,
		Args:   append([]string{redisCli}, command...),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf(
			"problem encountered while running command[%q %q]: %w",
			redisCli,
			command,
			err,
		)
	}
	return nil
}

// CreateCluster uses the redis-cli to create a new Redis cluster.
func CreateCluster(conf config.RedisOpts, nodes []string, replicasPerShard int) error {
	var opts []string
	opts = append(opts, "--cluster", "create")
	opts = append(opts, nodes...)
	opts = append(opts, "--cluster-yes", "--cluster-replicas", fmt.Sprint(replicasPerShard))
	return execute(conf, opts)
}

// AddNewShardPrimary introduces this Redis node (the node that this instance of
// `attache-control` is acting as a sidecar to) to an existing Redis Cluster as
// a new shard primary then rebalances the existing cluster shard slots.
func AddNewShardPrimary(conf config.RedisOpts, destNodeAddr string) error {
	err := execute(conf, []string{"--cluster", "add-node", conf.NodeAddr, destNodeAddr})
	if err != nil {
		return err
	}
	logger.Info("cluster MEET succeeded")

	// Retry shard slot belance for a full minute before failing. Occasionally a
	// cluster won't be ready for a shard slot rebalance immediately after
	// meeting a new primary node because gossip about this new master hasn't
	// propogated yet.
	logger.Info("attempting cluster shard slot rebalance")
	var attempts int
	ticker := time.NewTicker(6 * time.Second)
	for range ticker.C {
		attempts++
		err = execute(conf, []string{"--cluster", "rebalance", conf.NodeAddr, "--cluster-use-empty-masters"})
		if err != nil {
			if attempts >= 10 {
				return err
			}
			continue
		}
		logger.Info("cluster shard slot rebalance succeeded")
		break
	}
	return nil
}

// AddNewShardReplica introduces this Redis node (the node that this instance of
// `attache-control` is acting as a sidecar to) to an existing Redis Cluster as
// a replica to the shard primary with the least number of replicas.
func AddNewShardReplica(conf config.RedisOpts, destNodeAddr string) error {
	clusterClient, err := client.New(
		config.RedisOpts{
			NodeAddr:       destNodeAddr,
			Username:       conf.Username,
			PasswordConfig: conf.PasswordConfig,
			TLSConfig:      conf.TLSConfig,
		},
	)
	if err != nil {
		return err
	}

	primaryAddr, primaryID, err := clusterClient.GetPrimaryWithLeastReplicas()
	if err != nil {
		return err
	}

	return execute(
		conf,
		[]string{
			"--cluster",
			"add-node",
			conf.NodeAddr,
			primaryAddr,
			"--cluster-slave",
			"--cluster-master-id",
			primaryID,
		},
	)
}
