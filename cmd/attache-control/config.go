package main

import (
	"errors"
	"flag"
	"time"

	c "github.com/letsencrypt/attache/src/consul/config"
	s "github.com/letsencrypt/attache/src/consul/scaling"
	r "github.com/letsencrypt/attache/src/redis/config"
)

// CLIOpts contains all of the configuration used to orchestrate the Redis
// Cluster under management by Attaché. It's fields are exported for use with
// `flag.Parse()`.
type CLIOpts struct {
	// LockPath is the Consul KV path to use as a leader lock for Redis Cluster
	// operations.
	LockPath string

	// AttemptInterval is duration to wait between attempts to join or create a
	// cluster.
	AttemptInterval time.Duration

	// Number of times to attempt joining or creating a cluster before Attache
	// should exit as failed.
	AttemptLimit int

	// AwaitServiceName is the name of the Consul Service that newly created
	// Redis Cluster nodes will join when they're first started but have yet to
	// form or join a cluster. This field is required.
	AwaitServiceName string

	// DestServiceName is the name of the Consul Service that Redis Cluster
	// nodes will join once they are part of a cluster. This field is required.
	DestServiceName string

	// LogLevel is the level that Attaché should log at.
	LogLevel string

	// RedisOpts contains the configuration for interacting with the node this
	// serves as a sidecar to and, if one exists, the Redis Cluster. This field
	// is required.
	RedisOpts r.RedisOpts

	// ConsulOpts contains the configuration for interacting with the Consul
	// cluster that Attaché uses for leader lock and to retrieve the scaling
	// options in the Consul KV store. This field is required.
	ConsulOpts c.ConsulOpts

	// ScalingOpts defines the expected number of primary and replica nodes in
	// the Redis Cluster being orchestrated by Attaché. This field is required.
	ScalingOpts *s.ScalingOpts
}

// Validate checks that the required opts for `attache-control` were passed via
// the CLI. User friendly errors are returned when this is not the case.
func (c CLIOpts) Validate() error {
	if c.DestServiceName == "" {
		return errors.New("missing required opt: 'dest-service-name'")
	}

	if c.AwaitServiceName == "" {
		return errors.New("missing required opt: 'await-service-name'")
	}

	err := c.ConsulOpts.Validate()
	if err != nil {
		return err
	}

	err = c.RedisOpts.Validate()
	if err != nil {
		return err
	}
	return nil
}

func ParseFlags() CLIOpts {
	var conf CLIOpts

	// CLI
	flag.StringVar(&conf.LockPath, "lock-kv-path", "service/attache/leader", "Consul KV path to use as a leader lock for Redis Cluster operations")
	flag.DurationVar(&conf.AttemptInterval, "attempt-interval", 3*time.Second, "Duration to wait between attempts to join or create a cluster (e.g. '1s')")
	flag.IntVar(&conf.AttemptLimit, "attempt-limit", 20, "Number of times to attempt for or join a cluster before exiting")
	flag.StringVar(&conf.AwaitServiceName, "await-service-name", "", "Consul Service for newly created Redis Cluster Nodes, (required)")
	flag.StringVar(&conf.DestServiceName, "dest-service-name", "", "Consul Service for healthy Redis Cluster Nodes, (required)")
	flag.StringVar(&conf.LogLevel, "log-level", "info", "Set the log level")

	// Redis
	flag.StringVar(&conf.RedisOpts.NodeAddr, "redis-node-addr", "", "redis-server listening address, (required)")
	flag.StringVar(&conf.RedisOpts.Username, "redis-auth-username", "", "Redis username, (required)")
	flag.StringVar(&conf.RedisOpts.PasswordFile, "redis-auth-password-file", "", "Redis password file path, (required)")
	flag.StringVar(&conf.RedisOpts.CACertFile, "redis-tls-ca-cert", "", "Redis client CA certificate file, (required)")
	flag.StringVar(&conf.RedisOpts.CertFile, "redis-tls-cert-file", "", "Redis client certificate file, (required)")
	flag.StringVar(&conf.RedisOpts.KeyFile, "redis-tls-key-file", "", "Redis client key file, (required)")

	// Consul
	flag.StringVar(&conf.ConsulOpts.DC, "consul-dc", "dev-general", "Consul client datacenter")
	flag.StringVar(&conf.ConsulOpts.Address, "consul-addr", "127.0.0.1:8500", "Consul client address")
	flag.StringVar(&conf.ConsulOpts.ACLToken, "consul-acl-token", "", "Consul client ACL token")
	flag.BoolVar(&conf.ConsulOpts.EnableTLS, "consul-tls-enable", false, "Enable mTLS for the Consul client (requires 'consul-tls-ca-cert', 'consul-tls-cert', 'consul-tls-key')")
	flag.StringVar(&conf.ConsulOpts.TLSCACertFile, "consul-tls-ca-cert", "", "Consul client CA certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSCertFile, "consul-tls-cert", "", "Consul client certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSKeyFile, "consul-tls-key", "", "Consul client key file")

	flag.Parse()
	return conf
}
