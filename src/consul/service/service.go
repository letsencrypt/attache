package service

import (
	"fmt"

	consul "github.com/hashicorp/consul/api"
	"github.com/letsencrypt/attache/src/consul/config"
)

// ServiceInfo is a convenience wrapper for an inner `*consul.Client` with
// fields used to filter queries to Consul's Service Catalog API.
type ServiceInfo struct {
	client      *consul.Client
	serviceName string
	tagName     string
	onlyHealthy bool
}

// New creates a new Consul client and returns a `*ServiceInfo` to the caller.
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

// GetNodeAddresses queries the Consul Service Catalog for members of the
// `s.ServiceName`, constructs a slice of addresses in the format <ip>:<port>
// which it returns to the caller.
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
