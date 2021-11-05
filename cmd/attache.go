package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/leader"
	"github.com/letsencrypt/attache/src/service"
)

func redisCommandLineExec(command string) {
	echo, _ := exec.LookPath("echo")
	cmdEcho := &exec.Cmd{
		Path:   echo,
		Args:   []string{echo, command},
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	}

	err := cmdEcho.Run()
	if err != nil {
		log.Printf("error running command %q: %s\n", command, err)
	}
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

	entries, err := service.Query(client, "redis-ocsp-awaiting-intro", "primary")
	if err != nil {
		log.Fatalln(err)
	}
	for _, entry := range entries {
		fmt.Println(entry.Service.Address, entry.Service.Port)
	}
	os.Exit(0)

	session := leader.NewExclusiveSession(client, "service/attache/leader", "15s")
	session.Create()
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

		redisCommandLineExec("pretend I made a cluster lol")
		time.Sleep(10 * time.Second)
		close(doneChan)

		log.Println("initialization complete, cleaning up session")
		err := session.Cleanup()
		if err != nil {
			log.Println("failed to cleanup session")
		}
		return
	} else {
		fmt.Println("leader already chosen")
		os.Exit(0)
	}
}
