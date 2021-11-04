package control

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/letsencrypt/attache/src/check"
)

func execRedisCLI(command []string) error {
	redisCli, _ := exec.LookPath("redis-cli")
	cmd := &exec.Cmd{
		Path:   redisCli,
		Args:   append([]string{redisCli}, command...),
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(
			"problem encountered while running command[%q %q] got stdout[%q]: %w",
			redisCli,
			command,
			cmd.Stdout,
			err,
		)
	}

	return nil
}

func makeClusterCreateOpts(addresses []string, replicasPerShard int) []string {
	var opts []string
	opts = append(opts, "--cluster", "create")
	opts = append(opts, addresses...)
	return append(opts, "--cluster-yes", "--cluster-replicas", fmt.Sprint(replicasPerShard))
}

func makeAddNewShardPrimaryOpts(newNodeAddr, destNodeAddr string) []string {
	return []string{"--cluster", "add-node", newNodeAddr, destNodeAddr}
}

func makeClusterRebalanceSlotsOpts(newNodeAddr string) []string {
	return []string{"--cluster", "rebalance", newNodeAddr, "--cluster-use-empty-masters"}
}

func makeAddNewShardReplicaOpts(newNodeAddr, primaryAddr, primaryID string) []string {
	return []string{
		"--cluster",
		"add-node",
		newNodeAddr,
		primaryAddr,
		"--cluster-slave",
		"--cluster-master-id",
		primaryID,
	}
}

func RedisCLICreateCluster(nodes []string, replicasPerShard int) error {
	err := execRedisCLI(makeClusterCreateOpts(nodes, replicasPerShard))
	if err != nil {
		return err
	}

	return nil
}

func RedisCLIAddNewShardPrimary(newNodeAddr, destNodeAddr string) error {
	err := execRedisCLI(makeAddNewShardPrimaryOpts(newNodeAddr, destNodeAddr))
	if err != nil {
		return err
	}

	err = execRedisCLI(makeClusterRebalanceSlotsOpts(newNodeAddr))
	if err != nil {
		return err
	}

	return nil
}

func RedisCLIAddNewShardReplica(newNodeAddr, destNodeAddr string) error {
	check := check.NewRedisClient(destNodeAddr, "")
	primaryAddr, primaryID, err := check.GetPrimaryWithLeastReplicas()
	if err != nil {
		return err
	}

	err = execRedisCLI(makeAddNewShardReplicaOpts(newNodeAddr, primaryAddr, primaryID))
	if err != nil {
		return err
	}

	return nil
}
