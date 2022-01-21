package client

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
	"gopkg.in/yaml.v3"
)

// Client is a convenience wrapper for an inner `*consul.Client`.
type Client struct {
	*consul.Client
	serviceName string
}

// New creates a new Consul client and returns a `*Client` to the caller.
func New(conf config.ConsulOpts, serviceName string) (*Client, error) {
	consulConfig, err := conf.MakeConsulConfig()
	if err != nil {
		return nil, err
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, err
	}
	return &Client{client, serviceName}, nil
}

// GetNodeAddresses queries the Consul Service Catalog for members of the
// `s.ServiceName`, constructs a slice of addresses in the format <ip>:<port>
// which it returns to the caller. When `onlyHealthy` is true Consul will only
// return nodes that are currently passing all health checks.
func (s *Client) GetNodeAddresses(onlyHealthy bool) ([]string, error) {
	nodes, _, err := s.Health().Service(s.serviceName, "", onlyHealthy, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot query consul for service %q: %w", s.serviceName, err)
	}

	var addresses []string
	for _, entry := range nodes {
		addresses = append(addresses, fmt.Sprintf("%s:%d", entry.Service.Address, entry.Service.Port))
	}
	return addresses, nil
}

// ScalingOpts defines the expected number of primary and replica nodes in the
// Redis Cluster being orchestrated by Attaché.
type ScalingOpts struct {
	// PrimaryCount is the count of primary Redis nodes you expect to be present
	// in the final Redis Cluster.
	PrimaryCount int `yaml:"primary-count"`

	// ReplicaCount is the count of replica Redis nodes you expect to be present
	// in the final Redis Cluster.
	ReplicaCount int `yaml:"replica-count"`
}

// totalCount returns the total count of expected replica and primary nodes.
func (s *ScalingOpts) totalCount() int {
	return s.PrimaryCount + s.ReplicaCount
}

// NodesMissing returns the total count of expected replica and primary nodes
// minus nodesInAwait.
func (s *ScalingOpts) NodesMissing(nodesInAwait int) int {
	return s.totalCount() - nodesInAwait
}

// ReplicasPerPrimary returns the number of replica nodes per primary shard.
func (s *ScalingOpts) ReplicasPerPrimary() int {
	return s.ReplicaCount / s.PrimaryCount
}

// GetScalingOpts fetches the count of Redis primary and replica nodes from KV
// path: "service/destServiceName/scaling", and return them as a `*ScalingOpts`
// to the caller.
func (c *Client) GetScalingOpts() (*ScalingOpts, error) {
	kv := c.KV()

	scalingOptsKey := fmt.Sprintf("service/%s/scaling", c.serviceName)
	scalingOptsKV, _, err := kv.Get(scalingOptsKey, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get value for key %q: %w", scalingOptsKey, err)
	}

	// Per the consul API docs, the returned pointer will be nil if the key does
	// not exist.
	if scalingOptsKV == nil {
		return nil, fmt.Errorf("key %q is not defined", scalingOptsKey)
	}

	var opts ScalingOpts
	err = yaml.Unmarshal(scalingOptsKV.Value, &opts)
	if err != nil {
		return nil, err
	}
	return &opts, nil
}
