package clusterinit

import (
	"fmt"
	"log"

	"github.com/hashicorp/consul/api"
)

type Lock struct {
	Path      string
	DistMutex *api.Lock
	Chan      <-chan struct{}
}

func (l *Lock) Lock() (err error) {
	log.Println("Trying to get consul lock")
	l.Chan, err = l.DistMutex.Lock(nil)
	if err != nil {
		return err
	}
	log.Println("Lock acquired")
	return
}

func (l *Lock) Unlock() {
	log.Println("Releasing consul lock")
	l.DistMutex.Unlock()
}

func NewLock(service string) (*Lock, error) {
	path := fmt.Sprintf("service/%s/leader", service)
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	lock, err := client.LockKey(path)
	if err != nil {
		return nil, err
	}
	return &Lock{Path: path, DistMutex: lock}, nil
}
