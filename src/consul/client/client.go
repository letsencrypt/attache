package client

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
)

type ServiceInfo struct {
	client      *consul.Client
	serviceName string
	tagName     string
	onlyHealthy bool
}

func New(conf config.ConsulOpts, serviceName, tagName string, onlyHealthy bool) (*ServiceInfo, error) {
	consulConfig, err := conf.MakeConsulConfig()
	if err != nil {
		return nil, err
	}

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, err
	}
	return &ServiceInfo{client, serviceName, tagName, onlyHealthy}, nil
}

func (s *ServiceInfo) GetNodeAddresses() ([]string, error) {
	nodes, _, err := s.client.Health().Service(
		s.serviceName,
		s.tagName,
		s.onlyHealthy,
		&consul.QueryOptions{
			RequireConsistent: true,
			AllowStale:        false,
			UseCache:          false,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("cannot query consul for service %q: %w", s.serviceName, err)
	}

	var addresses []string
	for _, entry := range nodes {
		addresses = append(addresses, fmt.Sprintf("%s:%d", entry.Service.Address, entry.Service.Port))
	}
	return addresses, nil
}
