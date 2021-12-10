package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
)

// RedisOpts contains the configuration for interacting with the node this
// serves as a sidecar to and, if one exists, the Redis Cluster.
type RedisOpts struct {
	// NodeAddr is the <address>:<port> that Redis expects connections on. This
	// field is required.
	NodeAddr string

	// Username is used for authentication with Redis nodes. This field is
	// required.
	Username string

	// PasswordConfig contains a path to a file containing a password used for
	// authentication with Redis nodes. This field is required.
	PasswordConfig

	// TLSConfig contains the paths to certificates and a key used by the
	// redis-go client and redis-cli to interact with Redis nodes using mutual
	// TLS. This field is required.
	TLSConfig
}

// Validate checks that the required opts for interacting with Redis nodes via
// go-redis client and redis-cli were provided. User friendly errors are
// returned when this is not the case.
func (c RedisOpts) Validate() error {
	if c.NodeAddr == "" {
		return errors.New("missing required opt: 'redis-node-addr'")
	}

	if c.Username == "" {
		return errors.New("missing required opt: 'redis-auth-username'")
	}

	if c.PasswordFile == "" {
		return errors.New("missing required opt: 'redis-auth-password-file'")
	}

	if c.CACertFile == "" {
		return errors.New("missing required opt: 'redis-tls-ca-cert'")
	}

	if c.CertFile == "" {
		return errors.New("missing required opt: 'redis-tls-cert-file'")
	}

	if c.KeyFile == "" {
		return errors.New("missing required opt: 'redis-tls-key-file'")
	}
	return nil
}

// PasswordConfig contains a path to a file containing a password used for
// authentication with Redis nodes.
type PasswordConfig struct {
	PasswordFile string
}

// LoadPassword returns the password loaded from the inner `File`.
func (c PasswordConfig) LoadPassword() (string, error) {
	contents, err := ioutil.ReadFile(c.PasswordFile)
	if err != nil {
		return "", fmt.Errorf("cannot load password: %w", err)
	}
	return strings.TrimRight(string(contents), "\n"), nil
}

// TLSConfig contains the paths to certificates and a key used by the redis-go
// client and redis-cli to interact with Redis nodes using mutual TLS.
type TLSConfig struct {
	// CertFile is the path to a PEM formatted Certificate.
	CertFile string
	// KeyFile is the path to a PEM formatted Private Key.
	KeyFile string
	// CACertFile is the path to a PEM formatted CA Certificate.
	CACertFile string
}

// LoadTLS reads and parses the certificates and key provided by the TLSConfig
// and returns a *tls.Config suitable for redis-go client use.
func (c TLSConfig) LoadTLS() (*tls.Config, error) {
	caCertBytes, err := ioutil.ReadFile(c.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert from %q: %s", c.CACertFile, err)
	}

	rootCAs := x509.NewCertPool()
	ok := rootCAs.AppendCertsFromPEM(caCertBytes)
	if !ok {
		return nil, fmt.Errorf("parsing CA cert from %q failed", c.CACertFile)
	}

	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf(
			"loading key pair from %q and %q: %s",
			c.CertFile,
			c.KeyFile,
			err,
		)
	}
	return &tls.Config{
		RootCAs:      rootCAs,
		Certificates: []tls.Certificate{cert},
	}, nil
}
