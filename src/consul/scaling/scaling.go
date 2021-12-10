package kv

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
	"gopkg.in/yaml.v3"
)

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

// TotalCount returns the total count of expected replica and primary Redis
// Cluster nodes.
func (s *ScalingOpts) TotalCount() int {
	return s.PrimaryCount + s.ReplicaCount
}

// GetScalingOpts creates a Consul client and fetches the count of Redis primary
// and replica nodes from KV path: "service/destServiceName/scaling", and return
// them as a `*ScalingOpts` to the caller.
func GetScalingOpts(conf config.ConsulOpts, destServiceName string) (*ScalingOpts, error) {
	consulConfig, err := conf.MakeConsulConfig()
	if err != nil {
		return nil, err
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, err
	}
	kv := client.KV()

	scalingOptsKey := fmt.Sprintf("service/%s/scaling", destServiceName)
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
	return &ScalingOpts{opts.PrimaryCount, opts.ReplicaCount}, nil
}
