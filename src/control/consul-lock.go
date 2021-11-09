package control

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

type ConsulLock struct {
	client         *api.Client
	key            string
	sessionID      string
	sessionTimeout string
}

func NewConsulLock(client *api.Client, key string, sessionTimeout string) *ConsulLock {
	return &ConsulLock{
		client:         client,
		key:            key,
		sessionTimeout: sessionTimeout,
	}
}

// Create defines and initializes a new session using the Consul client.
func (l *ConsulLock) Create() error {
	sessionConf := &api.SessionEntry{
		TTL:      l.sessionTimeout,
		Behavior: "delete",
	}

	sessionID, _, err := l.client.Session().Create(sessionConf, nil)
	if err != nil {
		return err
	}

	l.sessionID = sessionID
	return nil
}

// Acquire creates a mutex lock on the Consul key. After this has been aquired,
// all other clients attempting to aquire a session for the same Consul key will
// fail.
func (l *ConsulLock) Acquire() (bool, error) {
	kvPair := &api.KVPair{
		Key:     l.key,
		Value:   []byte(l.sessionID),
		Session: l.sessionID,
	}

	aquired, _, err := l.client.KV().Acquire(kvPair, nil)
	return aquired, err
}

// Renew takes a channel that we later use (by closing it) to signal that no
// more renewals are necessary.
func (l *ConsulLock) Renew(doneChan <-chan struct{}) error {
	err := l.client.Session().RenewPeriodic(l.sessionTimeout, l.sessionID, nil, doneChan)
	if err != nil {
		return err
	}
	return nil
}

// Cleanup destroys the session by triggering the behavior. This deletes the
// configured key as well.
func (l *ConsulLock) Cleanup() error {
	_, err := l.client.Session().Destroy(l.sessionID, nil)
	if err != nil {
		return fmt.Errorf("cannot delete key %s: %s", l.key, err)
	}
	return nil
}
