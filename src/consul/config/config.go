package config

import (
	"errors"
	"fmt"
	"net/http"

	consul "github.com/hashicorp/consul/api"
)

// ConsulOpts is exported for use with flag.Parse().
type ConsulOpts struct {
	DC        string
	Address   string
	ACLToken  string
	EnableTLS bool
	TLSCACert string
	TLSCert   string
	TLSKey    string
}

func (c *ConsulOpts) Validate() error {
	if c.EnableTLS {
		if c.TLSCACert == "" {
			return errors.New("missing required opt: 'consul-tls-ca-cert")
		}

		if c.TLSCert == "" {
			return errors.New("missing required opt: 'consul-tls-cert")
		}

		if c.TLSKey == "" {
			return errors.New("missing required opt: 'consul-tls-key")
		}
	}
	return nil
}

func (c *ConsulOpts) MakeConsulConfig() (*consul.Config, error) {
	config := consul.DefaultConfig()
	config.Datacenter = c.DC
	config.Address = c.Address
	config.Token = c.ACLToken
	if c.EnableTLS {
		config.Scheme = "https"
		tlsConfig := consul.TLSConfig{
			Address:  c.Address,
			CAFile:   c.TLSCACert,
			CertFile: c.TLSCert,
			KeyFile:  c.TLSKey,
		}
		tlsClientConf, err := consul.SetupTLSConfig(&tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating TLS client config for consul: %w", err)
		}
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsClientConf,
		}
	}
	return config, nil
}
