package client

import (
	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
	logger "github.com/sirupsen/logrus"
)

// Lock is a convenience wrapper around an inner `*consul.Client` with methods
// to aquire and release a mutually exclusive distributed lock using Consul
// sessions. This is used by attache-control to ensure that only one Redis
// Cluster node operation (create, add, remove) happens at once.
type Lock struct {
	Acquired       bool
	client         *consul.Client
	key            string
	sessionID      string
	sessionTimeout string
	renewChan      chan struct{}
}

// New creates a new Consul client, aquires an ephemeral session with that
// client, and returns a `*Lock` to the caller.
func New(conf config.ConsulOpts, key string, sessionTimeout string) (*Lock, error) {
	consulConfig, err := conf.MakeConsulConfig()
	if err != nil {
		return nil, err
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, err
	}
	lock := &Lock{
		client:         client,
		key:            key,
		sessionTimeout: sessionTimeout,
	}

	err = lock.createSession()
	if err != nil {
		return nil, err
	}
	return lock, err
}

// createSession creates a new ephemeral session using the Consul client. Any
// data stored during this session will be deleted once the session expires.
func (l *Lock) createSession() error {
	sessionConf := &consul.SessionEntry{
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

// Acquire attempts to obtain a lock for the Consul KV path of `l.key`. Sets `l.Acquired`
// true on success and false on failure.
func (l *Lock) Acquire() error {
	kvPair := &consul.KVPair{
		Key:     l.key,
		Value:   []byte(l.sessionID),
		Session: l.sessionID,
	}

	var err error
	l.Acquired, _, err = l.client.KV().Acquire(kvPair, nil)
	if l.Acquired {
		// Spin off a long-running go-routine to continuously renew our session.
		go l.periodicallyRenew()
	}
	return err
}

// periodicallyRenew will invoke periodicallyRenew() before l.sessionTimeout on
// a session until a l.renewChan is closed, it should only be called from a long
// running goroutine.
func (l *Lock) periodicallyRenew() {
	l.renewChan = make(chan struct{})
	err := l.client.Session().RenewPeriodic(l.sessionTimeout, l.sessionID, nil, l.renewChan)
	if err != nil {
		logger.Error(err)
	}
}

// Cleanup stops periodic session renewals used to hold the lock, releases the
// lock by deleting the key, and destroys the session. Deleting the key and
// destroying the session only need to be best effort. In the event that either
// of these calls fail the lock will be released and the session will be
// destroyed l.sessionTimeout after l.renewChan is closed.
func (l *Lock) Cleanup() {
	if l.Acquired {
		// Halt periodic session renewals.
		close(l.renewChan)

		// Delete the key holding the lock.
		_, err := l.client.KV().Delete(l.key, nil)
		if err != nil {
			logger.Errorf("cannot lock key %q: %s", l.key, err)
		}
		l.Acquired = false

	}
	if l.sessionID != "" {
		// Destroy the session.
		_, err := l.client.Session().Destroy(l.sessionID, nil)
		if err != nil {
			logger.Errorf("cannot cleanup session %q: %s", l.sessionID, err)
		}
		l.sessionID = ""
	}
}
