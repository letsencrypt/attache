package control

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/hashicorp/consul/api"
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

func RedisCLICreateCluster(client *api.Client, awaitServiceName string) error {
	catalog := check.NewServiceCatalogClient(client, awaitServiceName, "primary", true)
	addresses, err := catalog.GetAddresses()
	if err != nil {
		return err
	}

	err = execRedisCLI(makeClusterCreateOpts(addresses))
	if err != nil {
		return err
	}

	return nil
}

// func RedisCLIJoinCluster(client *api.Client, awaitServiceName string) error {
// 	catalog := check.NewServiceCatalogClient(client, awaitServiceName, "primary", true)
// 	addresses, err := catalog.GetAddresses()
// 	if err != nil {
// 		return err
// 	}

// 	err = execRedisCLI(makeClusterCreateOpts(addresses))
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func RedisCLICreateCluster(client *api.Client, awaitServiceName string) error {
// 	catalog := check.NewServiceCatalogClient(client, awaitServiceName, "primary", true)
// 	addresses, err := catalog.GetAddresses()
// 	if err != nil {
// 		return err
// 	}

// 	err = execRedisCLI(makeClusterCreateOpts(addresses))
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }
