package service

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

type ServiceClient struct {
	client      *api.Client
	serviceName string
	tagName     string
	onlyHealthy bool
}

func NewServiceClient(client *api.Client, serviceName, tagName string, onlyHealthy bool) *ServiceClient {
	return &ServiceClient{client, serviceName, tagName, onlyHealthy}
}

func (s *ServiceClient) GetAddresses() ([]string, error) {
	nodes, _, err := s.client.Health().Service(s.serviceName, s.tagName, s.onlyHealthy, nil)
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, entry := range nodes {
		addresses = append(addresses, fmt.Sprintf("%s:%d", entry.Service.Address, entry.Service.Port))
	}
	return addresses, nil
}
