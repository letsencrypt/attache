package config

import (
	"fmt"
	"net/http"

	consul "github.com/hashicorp/consul/api"
)

// ConsulOpts contains the configuration for interacting with the Consul cluster
// that Attaché uses for leader lock and to retrieve the scaling options in the
// Consul KV store.
type ConsulOpts struct {
	// DC is the Consul datacenter used for API calls. This field is required.
	DC string

	// Address is the <address>:<port> that your Consul servers expect to
	// recieve API calls on. This field is required.
	Address string

	// ACLToken is not required but if present will be passed as the token for
	// API calls.
	ACLToken string

	// EnableTLS is not required but if set to true will enable mutual TLS for
	// API calls.
	EnableTLS bool

	// TLSCACertFile is the path to a PEM formatted CA Certificate. Required
	// when `EnableTLS` is true.
	TLSCACertFile string

	// TLSCertFile is the path to a PEM formatted Certificate. Required when
	// `EnableTLS` is true.
	TLSCertFile string

	// TLSKeyFile is the path to a PEM formatted Private Key. Required when
	// `EnableTLS` is true.
	TLSKeyFile string
}

// MakeConsulConfig constructs a `*consul.Config`.
func (c *ConsulOpts) MakeConsulConfig() (*consul.Config, error) {
	config := consul.DefaultConfig()
	config.Datacenter = c.DC
	config.Address = c.Address
	config.Token = c.ACLToken
	if c.EnableTLS {
		config.Scheme = "https"
		tlsConfig := consul.TLSConfig{
			Address:            c.Address,
			CAFile:             c.TLSCACertFile,
			CertFile:           c.TLSCertFile,
			KeyFile:            c.TLSKeyFile,
			InsecureSkipVerify: true,
		}
		tlsClientConf, err := consul.SetupTLSConfig(&tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating TLS client config for consul: %w", err)
		}

		config.HttpClient = &http.Client{
			Transport: http.DefaultTransport,
		}
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsClientConf,
		}
	}
	return config, nil
}
