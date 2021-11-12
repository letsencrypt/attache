package control

import (
	"fmt"
	"os"
	"os/exec"
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
		return fmt.Errorf("cannot run command %q with opts %q: %w", redisCli, command, err)
	}

	return nil
}

func makeClusterCreateOpts(addresses []string) []string {
	var clusterCreateOpts []string
	clusterCreateOpts = append(clusterCreateOpts, "--cluster", "create")
	clusterCreateOpts = append(clusterCreateOpts, addresses...)
	return append(clusterCreateOpts, "--cluster-yes", "--cluster-replicas", "0")
}

func RedisCLICreateCluster(nodes []string) error {
	err := execRedisCLI(makeClusterCreateOpts(nodes[0:2]))
	if err != nil {
		return err
	}

	return nil
}

func RedisCLIAddNewShardPrimary(newNodeAddr, destNodeAddr string) error {
	// redis-cli --cluster add-node newNodeAddr destNodeAddr
	// redis-cli --cluster rebalance newNodeAddr --cluster-use-empty-masters
	// err := execRedisCLI()
	// if err != nil {
	// 	return err
	// }

	return nil
}

func RedisCLIAddNewShardReplica(newNodeAddr, destNodeAddr string) error {
	// redis-cli --cluster add-node newNodeAddr destNodeAddr --cluster-slave --cluster-master-id destNodeAddr
	// err := execRedisCLI()
	// if err != nil {
	// 	return err
	// }

	return nil
}
