package client

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
)

type ServiceInfo struct {
	client      *consul.Client
	serviceName string
	tagName     string
	onlyHealthy bool
}

func New(client *consul.Client, serviceName, tagName string, onlyHealthy bool) *ServiceInfo {
	return &ServiceInfo{client, serviceName, tagName, onlyHealthy}
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
