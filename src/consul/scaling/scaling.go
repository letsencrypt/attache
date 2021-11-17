package kv

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
	"gopkg.in/yaml.v3"
)

type scalingOpts struct {
	PrimaryCount int `yaml:"primary-count"`
	ReplicaCount int `yaml:"replica-count"`
}

func GetOpts(conf config.ConsulOpts, destServiceName string) (int, int, error) {
	consulConfig, err := conf.MakeConsulConfig()
	if err != nil {
		return 0, 0, err
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return 0, 0, err
	}
	kv := client.KV()

	scalingOptsKey := fmt.Sprintf("service/%s/scaling", destServiceName)
	scalingOptsKV, _, err := kv.Get(scalingOptsKey, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot get value for consul key %q: %w", scalingOptsKey, err)
	}

	if scalingOptsKV == nil {
		return 0, 0, fmt.Errorf("consul key %q is not defined", scalingOptsKey)
	}

	var opts scalingOpts
	err = yaml.Unmarshal(scalingOptsKV.Value, &opts)
	if err != nil {
		return 0, 0, err
	}
	return opts.PrimaryCount, opts.ReplicaCount, nil
}
