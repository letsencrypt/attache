package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/letsencrypt/attache/src/redis/client"
)

func execute(command []string) error {
	redisCli, _ := exec.LookPath("redis-cli")
	cmd := &exec.Cmd{
		Path:   redisCli,
		Args:   append([]string{redisCli}, command...),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	err := cmd.Run()
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

func CreateCluster(nodes []string, replicasPerShard int) error {
	var opts []string
	opts = append(opts, "--cluster", "create")
	opts = append(opts, nodes...)
	opts = append(opts, "--cluster-yes", "--cluster-replicas", fmt.Sprint(replicasPerShard))
	return execute(opts)
}

func AddNewShardPrimary(newNodeAddr, destNodeAddr string) error {
	err := execute([]string{"--cluster", "add-node", newNodeAddr, destNodeAddr})
	if err != nil {
		return err
	}
	return execute([]string{"--cluster", "rebalance", newNodeAddr, "--cluster-use-empty-masters"})
}

func AddNewShardReplica(newNodeAddr, destNodeAddr string) error {
	redisClient := client.New(destNodeAddr, "")
	primaryAddr, primaryID, err := redisClient.GetPrimaryWithLeastReplicas()
	if err != nil {
		return err
	}

	return execute(
		[]string{
			"--cluster",
			"add-node",
			newNodeAddr,
			primaryAddr,
			"--cluster-slave",
			"--cluster-master-id",
			primaryID,
		},
	)
}
