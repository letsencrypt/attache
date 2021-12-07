package client

import (
	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
	logger "github.com/sirupsen/logrus"
)

type Lock struct {
	client         *consul.Client
	key            string
	sessionID      string
	sessionTimeout string
}

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

// Acquire attempts to obtain a lock for the Consul KV path of `l.key`. Returns
// true on success or false on failure.
func (l *Lock) Acquire() (bool, error) {
	kvPair := &consul.KVPair{
		Key:     l.key,
		Value:   []byte(l.sessionID),
		Session: l.sessionID,
	}

	acquired, _, err := l.client.KV().Acquire(kvPair, nil)
	return acquired, err
}

// Renew is used to periodically invoke Session.Renew() on a session until a
// `doneChan` is closed, it should only be called from a long running goroutine.
func (l *Lock) Renew(doneChan <-chan struct{}) {
	err := l.client.Session().RenewPeriodic(l.sessionTimeout, l.sessionID, nil, doneChan)
	if err != nil {
		logger.Error(err)
	}
}

// Cleanup releases our leader lock by deleting the KV pair and destroying the
// session that was used to create it in the first place. These calls only need
// to be best-effort. In the event that either of them fail the lock will
// release once the session expires and any KV pair created during that session
// will be deleted as well.
func (l *Lock) Cleanup() {
	_, err := l.client.KV().Delete(l.key, nil)
	if err != nil {
		logger.Errorf("cannot delete lock key %q: %s", l.key, err)
	}

	_, err = l.client.Session().Destroy(l.sessionID, nil)
	if err != nil {
		logger.Errorf("cannot cleanup session %q: %s", l.sessionID, err)
	}
}
