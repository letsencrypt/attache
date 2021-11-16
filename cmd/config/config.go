package config

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	consul "github.com/hashicorp/consul/api"
	logger "github.com/sirupsen/logrus"
)

type CLIOpts struct {
	RedisNodeAddr     string
	RedisPrimaryCount int
	AttemptInterval   time.Duration
	AttemptLimit      int
	AwaitServiceName  string
	DestServiceName   string
	LogLevel          string
	ConsulOpts        ConsulOpts
}

type ConsulOpts struct {
	DC        string
	Address   string
	ACLToken  string
	TLSEnable bool
	TLSCACert string
	TLSCert   string
	TLSKey    string
}

func (c *ConsulOpts) MakeConsulClient() (*consul.Client, error) {
	config := consul.DefaultConfig()
	config.Datacenter = c.DC
	config.Address = c.Address
	config.Token = c.ACLToken
	if c.TLSEnable {
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
	flag.StringVar(&conf.RedisNodeAddr, "redis-node-addr", "", "redis-server listening address")
	flag.IntVar(&conf.RedisPrimaryCount, "redis-primary-count", 0, "Total number of expected Redis shard primary nodes")
	flag.DurationVar(&conf.AttemptInterval, "attempt-interval", 3*time.Second, "Duration to wait between attempts to join or create a cluster")
	flag.IntVar(&conf.AttemptLimit, "attempt-limit", 20, "Number of times to attempt joining or creating a cluster before exiting")
	flag.StringVar(&conf.AwaitServiceName, "await-service-name", "", "Consul Service for any newly created Redis Cluster Nodes")
	flag.StringVar(&conf.LogLevel, "log-level", "info", "Set the log level")

	// Consul
	flag.StringVar(&conf.DestServiceName, "dest-service-name", "", "Consul Service for any existing Redis Cluster Nodes")
	flag.StringVar(&conf.ConsulOpts.DC, "consul-dc", "dev-general", "Consul client datacenter")
	flag.StringVar(&conf.ConsulOpts.Address, "consul-addr", "127.0.0.1:8500", "Consul client address")
	flag.StringVar(&conf.ConsulOpts.ACLToken, "consul-acl-token", "", "Consul client ACL token")
	flag.BoolVar(&conf.ConsulOpts.TLSEnable, "consul-tls-enable", false, "Enable TLS for the Consul client")
	flag.StringVar(&conf.ConsulOpts.TLSCACert, "consul-tls-ca-cert", "", "Consul client CA certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSCert, "consul-tls-cert", "", "Consul client certificate file")
	flag.StringVar(&conf.ConsulOpts.TLSKey, "consul-tls-key", "", "Consul client key file")

	flag.Parse()

	return conf
}

func ValidateConfig(conf CLIOpts) {
	if conf.ConsulOpts.TLSEnable {
		if conf.ConsulOpts.TLSCACert == "" {
			logger.Fatal("Missing required opt: 'consul-tls-ca-cert")
		}

		if conf.ConsulOpts.TLSCert == "" {
			logger.Fatal("Missing required opt: 'consul-tls-cert")
		}

		if conf.ConsulOpts.TLSKey == "" {
			logger.Fatal("Missing required opt: 'consul-tls-key")
		}
	}

	if conf.RedisNodeAddr == "" {
		logger.Fatal("Missing required opt: 'redis-node-addr'")
	}

	if conf.RedisPrimaryCount == 0 {
		logger.Fatal("Missing required opt: 'redis-primary-count'")
	}

	if conf.DestServiceName == "" {
		logger.Fatal("Missing required opt: 'dest-service-name'")
	}

	if conf.AwaitServiceName == "" {
		logger.Fatal("Missing required opt: 'await-service-name'")
	}
}
