package redis

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strings"
)

// PasswordConfig contains a path to a file containing a password.
type PasswordConfig struct {
	PasswordFile string
}

// String returns a password, extracted from the inner `File`.
func (p *PasswordConfig) String() (string, error) {
	contents, err := ioutil.ReadFile(p.PasswordFile)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(contents), "\n"), nil
}

// TLSConfig represents certificates and a key for authenticated TLS.
type TLSConfig struct {
	CertFile   *string
	KeyFile    *string
	CACertFile *string
}

// Load reads and parses the certificates and key listed in the TLSConfig, and
// returns a *tls.Config suitable for redis-go client use.
func (t *TLSConfig) Load() (*tls.Config, error) {
	if t == nil {
		return nil, fmt.Errorf("nil TLS section in config")
	}

	if t.CertFile == nil {
		return nil, fmt.Errorf("nil CertFile in TLSConfig")
	}

	if t.KeyFile == nil {
		return nil, fmt.Errorf("nil KeyFile in TLSConfig")
	}

	if t.CACertFile == nil {
		return nil, fmt.Errorf("nil CACertFile in TLSConfig")
	}

	caCertBytes, err := ioutil.ReadFile(*t.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert from %q: %s", *t.CACertFile, err)
	}

	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCertBytes); !ok {
		return nil, fmt.Errorf("parsing CA certs from %s failed", *t.CACertFile)
	}

	cert, err := tls.LoadX509KeyPair(*t.CertFile, *t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading key pair from %q and %q: %s",
			*t.CertFile, *t.KeyFile, err)
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
