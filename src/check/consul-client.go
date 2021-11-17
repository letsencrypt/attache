package check

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

type ConsulServiceInfo struct {
	client      *api.Client
	serviceName string
	tagName     string
	onlyHealthy bool
}

func NewConsulServiceClient(client *api.Client, serviceName, tagName string, onlyHealthy bool) *ConsulServiceInfo {
	return &ConsulServiceInfo{client, serviceName, tagName, onlyHealthy}
}

func (s *ConsulServiceInfo) GetNodeAddresses() ([]string, error) {
	nodes, _, err := s.client.Health().Service(s.serviceName, s.tagName, s.onlyHealthy, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot query consul for service %q: %w", s.serviceName, err)
	}

	var addresses []string
	for _, entry := range nodes {
		addresses = append(addresses, fmt.Sprintf("%s:%d", entry.Service.Address, entry.Service.Port))
	}
	return addresses, nil
}
