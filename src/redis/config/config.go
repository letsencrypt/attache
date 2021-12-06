package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
)

type RedisConfig struct {
	NodeAddr  string
	Username  string
	EnableTLS bool
	PasswordConfig
	TLSConfig
}

func (c RedisConfig) Validate() error {
	if c.NodeAddr == "" {
		return errors.New("missing required opt: 'redis-node-addr'")
	}

	if c.Username == "" && c.PasswordFile != "" {
		return errors.New("missing required opt: 'redis-username'")
	}

	if c.Username != "" && c.PasswordFile == "" {
		return errors.New("missing required opt: 'redis-password-file'")
	}

	if c.EnableTLS {
		if c.CACertFile == "" {
			return errors.New("missing required opt: 'redis-tls-ca-cert'")
		}

		if c.CertFile == "" {
			return errors.New("missing required opt: 'redis-tls-cert-file'")
		}

		if c.KeyFile == "" {
			return errors.New("missing required opt: 'redis-tls-key-file'")
		}
	}
	return nil
}

// PasswordConfig contains a path to a file containing a password.
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

// TLSConfig contains certificates and a key used for redis-go client
// connections or passed as paths the the redis-cli.
type TLSConfig struct {
	CertFile   string
	KeyFile    string
	CACertFile string
}

// LoadTLS reads and parses the certificates and key provided by the TLSConfig and
// returns a *tls.Config suitable for redis-go client use.
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
		ClientCAs:    rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},

		// Set the only acceptable TLS version to 1.2 and the only acceptable
		// cipher suite to ECDHE-RSA-CHACHA20-POLY1305.
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305},
	}, nil
}
