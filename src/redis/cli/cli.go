package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
)

func makeTLSArgs(conf config.RedisConfig) ([]string, error) {
	password, err := conf.Password.Pass()
	if err != nil {
		return nil, fmt.Errorf("cannot load password: %w", err)
	}

	_, err = conf.TLSConfig.Load()
	if err != nil {
		return nil, err
	}

	return []string{
		"--tls",
		"--cert",
		*conf.TLSConfig.CertFile,
		"--key",
		*conf.TLSConfig.KeyFile,
		"--cacert",
		*conf.TLSConfig.CACertFile,
		"--user",
		conf.Username,
		"--pass",
		password,
	}, nil
}

func execute(conf config.RedisConfig, command []string) error {
	redisCli, err := exec.LookPath("redis-cli")
	if err != nil {
		return err
	}

	tlsArgs, err := makeTLSArgs(conf)
	if err != nil {
		return err
	}
	command = append(command, tlsArgs...)

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

func CreateCluster(conf config.RedisConfig, nodes []string, replicasPerShard int) error {
	var opts []string
	opts = append(opts, "--cluster", "create")
	opts = append(opts, nodes...)
	opts = append(opts, "--cluster-yes", "--cluster-replicas", fmt.Sprint(replicasPerShard))
	return execute(conf, opts)
}

func AddNewShardPrimary(conf config.RedisConfig, destNodeAddr string) error {
	err := execute(conf, []string{"--cluster", "add-node", conf.NodeAddr, destNodeAddr})
	if err != nil {
		return err
	}

	// Occasionally a cluster won't be ready for a shard slot rebalance
	// immediately after meeting a new primary node because gossip about this
	// new master hasn't propogated yet. This should be reattempted a few times.
	var attempts int
	var ticks = time.Tick(5 * time.Second)
	for range ticks {
		attempts++
		err = execute(conf, []string{"--cluster", "rebalance", conf.NodeAddr, "--cluster-use-empty-masters"})
		if err != nil {
			if attempts >= 5 {
				return err
			}
			continue
		}
		break
	}
	return nil
}

func AddNewShardReplica(conf config.RedisConfig, destNodeAddr string) error {
	redisClient, err := client.New()
	if err != nil {
		return err
	}

	primaryAddr, primaryID, err := redisClient.GetPrimaryWithLeastReplicas()
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
