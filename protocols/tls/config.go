package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
)

// TLSConfig holds per-layer configuration for the TLS layer.
// All PEM fields accept raw bytes (loaded from file, stream, embed, etc.).
type TLSConfig struct {
	// ServerName overrides the SNI hostname sent in the ClientHello.
	// When empty, the target host is used.
	ServerName string

	// CertPEM is the PEM-encoded client certificate for mTLS.
	// When non-empty, the certificate is presented to the server
	// during the handshake. KeyPEM must also be set.
	CertPEM []byte

	// KeyPEM is the PEM-encoded private key for the client certificate.
	// When empty and CertPEM is non-empty, the key is expected to be
	// bundled in the same PEM block as CertPEM.
	KeyPEM []byte

	// CAPEM is the PEM-encoded CA certificate pool. When non-empty,
	// the pool is used as the root of trust (RootCAs) instead of
	// skipping verification entirely.
	CAPEM []byte

	// InsecureSkipVerify controls whether the server's certificate chain
	// and hostname are verified. Defaults to true.
	InsecureSkipVerify bool
}

func (c *TLSConfig) LayerConfigName() string { return ProtocolName }

// DefaultTLSConfig returns a TLSConfig with safe defaults.
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		InsecureSkipVerify: true,
	}
}

// BuildTLSConfig constructs a crypto/tls.Config from the TLSConfig settings.
// When cfg is nil, the default (InsecureSkipVerify=true, ServerName=host) is used.
func BuildTLSConfig(cfg *TLSConfig, host string) (*tls.Config, error) {
	if cfg == nil {
		cfg = DefaultTLSConfig()
	}

	serverName := host
	if cfg.ServerName != "" {
		serverName = cfg.ServerName
	}

	result := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	// Load custom CA pool if specified.
	if len(cfg.CAPEM) > 0 {
		pool, err := buildCertPool(cfg.CAPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: failed to load CA certificate: %w", err)
		}
		result.RootCAs = pool
		// When a CA pool is provided explicitly, disable InsecureSkipVerify
		// so verification actually uses the pool.
		result.InsecureSkipVerify = false
	}

	// Load client certificate for mTLS if specified.
	if len(cfg.CertPEM) > 0 {
		cert, err := buildClientCert(cfg.CertPEM, cfg.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: failed to load client certificate: %w", err)
		}
		result.Certificates = []tls.Certificate{cert}
	}

	return result, nil
}

// buildCertPool parses PEM-encoded CA certificates and returns a CertPool.
func buildCertPool(pem []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no valid CA certificates found in PEM data")
	}
	return pool, nil
}

// buildClientCert parses a PEM-encoded X509 key pair. When keyPEM is empty,
// certPEM is expected to contain both the certificate and the private key.
func buildClientCert(certPEM, keyPEM []byte) (tls.Certificate, error) {
	if len(keyPEM) == 0 {
		keyPEM = certPEM
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}
