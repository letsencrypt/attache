package service

import (
	"github.com/hashicorp/consul/api"
)

func Query(client *api.Client, serviceName, tagName string) ([]*api.ServiceEntry, error) {
	entries, _, err := client.Health().Service(serviceName, tagName, true, nil)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
