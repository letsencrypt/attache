package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/leader"
	"github.com/letsencrypt/attache/src/service"
)

func execRedisCLI(command string) error {
	redisCli, _ := exec.LookPath("redis-cli")
	cmd := &exec.Cmd{
		Path:   redisCli,
		Args:   []string{redisCli, command},
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("cannot run command %q with opts %q: %w", redisCli, command, err)
	}
	return nil
}

func makeClusterCreateOpts(client *api.Client, awaitServiceName string) (string, error) {
	service := service.NewServiceClient(client, awaitServiceName, "primary", true)
	addresses, err := service.GetAddresses()
	if err != nil {
		return "", err
	}

	var clusterCreateOpts []string
	clusterCreateOpts = append(clusterCreateOpts, "--cluster", "create")
	clusterCreateOpts = append(clusterCreateOpts, addresses...)
	return strings.Join(append(clusterCreateOpts, "--cluster-yes", "--cluster-replicas", "0"), " "), nil
}

func newConsulClient() (*api.Client, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func main() {
	client, err := newConsulClient()
	if err != nil {
		log.Fatalln(err)
	}

	session := leader.NewExclusiveSession(client, "service/attache/leader", "15s")
	err = session.Create()
	if err != nil {
		log.Fatalln(err)
	}
	defer session.Cleanup()

	isLeader, err := session.Acquire()
	if err != nil {
		log.Fatalln(err)
	}

	// If forced to exit early, cleanup our session.
	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	go func() {
		<-catchSignals
		log.Println("interrupted, cleaning up session")

		err := session.Cleanup()
		if err != nil {
			log.Println("failed to cleanup session")
		}
		os.Exit(2)
	}()

	if isLeader {
		log.Println("aquired lock, initializing redis cluster")

		// Spin off a go-routine to renew our session until cluster
		// initialization is complete.
		doneChan := make(chan struct{})
		go session.Renew(doneChan)

		createClusterOps, err := makeClusterCreateOpts(client, "redis-ocsp-awaiting-intro")
		if err != nil {
			log.Fatal(err)
		}

		err = execRedisCLI(createClusterOps)
		if err != nil {
			log.Fatal(err)
		}
		close(doneChan)

		log.Println("initialization complete, cleaning up session")
		err = session.Cleanup()
		if err != nil {
			log.Println("failed to cleanup session")
		}
		return
	} else {
		fmt.Println("leader already chosen")
		os.Exit(0)
	}
}
