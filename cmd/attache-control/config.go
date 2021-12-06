package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/redis/config"
)

type CLIOpts struct {
	// Exported for use with flag.Parse()
	RedisOpts config.RedisConfig
	// Exported for use with flag.Parse()
	RedisPrimaryCount int
	// Exported for use with flag.Parse()
	RedisReplicaCount int
	// Exported for use with flag.Parse()
	LockPath string
	// Exported for use with flag.Parse()
	AttemptInterval time.Duration
	// Exported for use with flag.Parse()
	AttemptLimit int
	// Exported for use with flag.Parse()
	AwaitServiceName string
	// Exported for use with flag.Parse()
	DestServiceName string
	// Exported for use with flag.Parse()
	LogLevel string
	// Exported for use with flag.Parse()
	ConsulOpts ConsulOpts
}

func (c CLIOpts) Validate() error {
	if c.RedisPrimaryCount == 0 {
		return errors.New("Missing required opt: 'redis-primary-count'")
	}

	if c.DestServiceName == "" {
		return errors.New("Missing required opt: 'dest-service-name'")
	}

	if c.AwaitServiceName == "" {
		return errors.New("Missing required opt: 'await-service-name'")
	}

	err := c.RedisOpts.Validate()
	if err != nil {
		return err
	}

	if c.ConsulOpts.EnableTLS {
		if c.ConsulOpts.TLSCACert == "" {
			return errors.New("Missing required opt: 'consul-tls-ca-cert")
		}

		if c.ConsulOpts.TLSCert == "" {
			return errors.New("Missing required opt: 'consul-tls-cert")
		}

		if c.ConsulOpts.TLSKey == "" {
			return errors.New("Missing required opt: 'consul-tls-key")
		}
	}
	return nil
}

type ConsulOpts struct {
	// Exported for use with flag.Parse()
	DC string
	// Exported for use with flag.Parse()
	Address string
	// Exported for use with flag.Parse()
	ACLToken string
	// Exported for use with flag.Parse()
	EnableTLS bool
	// Exported for use with flag.Parse()
	TLSCACert string
	// Exported for use with flag.Parse()
	TLSCert string
	// Exported for use with flag.Parse()
	TLSKey string
}

func (c *ConsulOpts) MakeConsulClient() (*consul.Client, error) {
	config := consul.DefaultConfig()
	config.Datacenter = c.DC
	config.Address = c.Address
	config.Token = c.ACLToken
	if c.EnableTLS {
		config.Scheme = "https"
		tlsConfig := consul.TLSConfig{
			Address:  c.Address,
			CAFile:   c.TLSCACert,
			CertFile: c.TLSCert,
			KeyFile:  c.TLSKey,
		}
		tlsClientConf, err := consul.SetupTLSConfig(&tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating TLS client config for consul: %w", err)
		}
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsClientConf,
		}
	}

	client, err := consul.NewClient(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func ParseFlags() CLIOpts {
	var conf CLIOpts

	// CLI
	flag.IntVar(&conf.RedisPrimaryCount, "redis-primary-count", 0, "Total number of expected Redis shard primary nodes")
	flag.IntVar(&conf.RedisReplicaCount, "redis-replica-count", 0, "Total number of expected Redis shard replica nodes")
	flag.StringVar(&conf.LockPath, "lock-kv-path", "service/attache/leader", "Consul KV path used as a distributed lock for operations")
	flag.DurationVar(&conf.AttemptInterval, "attempt-interval", 3*time.Second, "Duration to wait between attempts to join or create a cluster")
	flag.IntVar(&conf.AttemptLimit, "attempt-limit", 20, "Number of times to attempt joining or creating a cluster before exiting")
	flag.StringVar(&conf.AwaitServiceName, "await-service-name", "", "Consul Service for any newly created Redis Cluster Nodes")
	flag.StringVar(&conf.LogLevel, "log-level", "info", "Set the log level")

	// Redis
	flag.StringVar(&conf.RedisOpts.NodeAddr, "redis-node-addr", "", "redis-server listening address")
	flag.BoolVar(&conf.RedisOpts.EnableAuth, "redis-auth-enable", false, "Enable auth for the Redis client and redis-cli")
	flag.StringVar(&conf.RedisOpts.Username, "redis-auth-username", "", "Redis username")
	flag.StringVar(&conf.RedisOpts.PasswordFile, "redis-auth-password-file", "", "Redis password file path")
	flag.BoolVar(&conf.RedisOpts.EnableTLS, "redis-tls-enable", false, "Enable mTLS for the Redis client")
	flag.StringVar(&conf.RedisOpts.CACertFile, "redis-tls-ca-cert", "", "Redis client CA certificate file")
	flag.StringVar(&conf.RedisOpts.CertFile, "redis-tls-cert-file", "", "Redis client certificate file")
	flag.StringVar(&conf.RedisOpts.KeyFile, "redis-tls-key-file", "", "Redis client key file")

	// Consul
	flag.StringVar(&conf.DestServiceName, "dest-service-name", "", "Consul Service for any existing Redis Cluster Nodes")
	flag.StringVar(&conf.ConsulOpts.DC, "consul-dc", "dev-general", "Consul client datacenter")
	flag.StringVar(&conf.ConsulOpts.Address, "consul-addr", "127.0.0.1:8500", "Consul client address")
	flag.StringVar(&conf.ConsulOpts.ACLToken, "consul-acl-token", "", "Consul client ACL token")
	flag.BoolVar(&conf.ConsulOpts.EnableTLS, "consul-tls-enable", false, "Enable mTLS for the Consul client")
	flag.StringVar(&conf.ConsulOpts.TLSCACert, "consul-tls-ca-cert", "", "Consul client CA certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSCert, "consul-tls-cert", "", "Consul client certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSKey, "consul-tls-key", "", "Consul client key file")

	flag.Parse()
	return conf
}
