package control

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

type Lock struct {
	client         *api.Client
	key            string
	sessionID      string
	sessionTimeout string
}

func NewLock(client *api.Client, key string, sessionTimeout string) *Lock {
	return &Lock{
		client:         client,
		key:            key,
		sessionTimeout: sessionTimeout,
	}
}

// Initialize creates a new ephemeral session using the Consul client. Any data
// stored during this session will be deleted once the session expires.
func (l *Lock) Initialize() error {
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

// Acquire obtains a lock for the path of `l.key`. After this has been aquired,
// all other clients attempting to aquire a session for the same Consul key will
// fail.
func (l *Lock) Acquire() (bool, error) {
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
func (l *Lock) Renew(doneChan <-chan struct{}) error {
	err := l.client.Session().RenewPeriodic(l.sessionTimeout, l.sessionID, nil, doneChan)
	if err != nil {
		return err
	}
	return nil
}

// Cleanup destroys the session by triggering the behavior. This deletes the
// configured key as well.
func (l *Lock) Cleanup() error {
	_, err := l.client.Session().Destroy(l.sessionID, nil)
	if err != nil {
		return fmt.Errorf("cannot delete key %s: %s", l.key, err)
	}
	return nil
}
