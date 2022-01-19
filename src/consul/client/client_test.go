package client

import (
	"testing"

	"github.com/letsencrypt/attache/src/consul/config"
)

func TestNew(t *testing.T) {
	config := config.ConsulOpts{
		DC:            "dev-general",
		Address:       "127.0.0.1:8501",
		EnableTLS:     true,
		TLSCACertFile: "../../../example/tls/consul/consul-agent-ca.pem",
		TLSCertFile:   "../../../example/tls/attache/consul/dev-general-client-consul-0.pem",
		TLSKeyFile:    "../../../example/tls/attache/consul/dev-general-client-consul-0-key.pem",
	}
	client, err := New(config, "test")
	if err != nil {
		t.Fatalf("failed to make client: %s", err)
	}

	if client == nil {
		t.Fatal("nil client")
	}
}
