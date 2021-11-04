package main

import (
	"fmt"

	"github.com/letsencrypt/attache/src/clusterinit"
)

func lockExample(lockPath string) error {
	init, err := clusterinit.NewLock(lockPath)
	if err != nil {
		fmt.Println("Unable to create consul lock:", err.Error())
		return err
	}
	if err := init.Lock(); err != nil {
		fmt.Println("Error while trying to acquire lock:", err.Error())
		return err
	}
	defer init.Unlock()
	fmt.Println("I win!")
	return nil
}
