package tls

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	cryptotls "crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the TLS protocol.
const ProtocolName = "tls"

// DefaultTLSPort is the well-known port for TLS (HTTPS).
const DefaultTLSPort = 443

// CertChainEntry holds the details of a single certificate in the chain.
type CertChainEntry struct {
	Subject           string
	Issuer            string
	Serial            string
	NotBefore         string
	NotAfter          string
	Version           int
	DNSNames          []string
	IPAddresses       []string
	IsCA              bool
	SignatureAlgo     string
	PublicKeyAlgo     string
	SHA256Fingerprint string

	// Status holds Chrome-style CERT_ checks for this individual certificate.
	// ref: https://github.com/chromium/chromium/blob/main/net/cert/cert_status_flags.cc
	Status []string
}

// TLSResult holds the full certificate chain and the concluded chain status from a TLS handshake.
type TLSResult struct {
	// CertChain contains all presented certificates sorted from leaf to root.
	// The leaf certificate is always CertChain[0].
	CertChain []CertChainEntry

	// Status is the final concluded chain status. It holds the statuses from
	// the first certificate (walking leaf→root) that has a non-OK check, or
	// ["OK"] if every certificate in the chain passes all checks.
	Status []string
}

func (r *TLSResult) LayerDataName() string { return ProtocolName }

// New creates a new TLS application layer.
func New() *TLS {
	return &TLS{}
}

// TLS implements a TLS connectivity and certificate probe as an AppLayer.
// It performs a TLS handshake over an already-dialed net.Conn, with
// InsecureSkipVerify, and reports Chrome-style CERT_ status checks.
type TLS struct{}

func (t *TLS) Name() string                   { return ProtocolName }
func (t *TLS) Description() string            { return "TLS certificate check" }
func (t *TLS) DefaultPort() int               { return DefaultTLSPort }
func (t *TLS) BaseTransport() greet.Transport { return greet.TransportTCP }

// RegisterFlags adds TLS-specific flags to the FlagSet.
func (t *TLS) RegisterFlags(fs *flag.FlagSet) {
	fs.String("server-name", "", "TLS SNI hostname override (default: target host)")
	fs.String("client-cert", "", "Path to PEM client certificate for mTLS (key may be bundled)")
	fs.String("client-key", "", "Path to PEM client private key (default: bundled in --client-cert)")
	fs.String("ca-cert", "", "Path to PEM CA certificate for trust verification")
	fs.Bool("insecure", true, "Skip certificate verification (default true)")
}

// ParseFlags reads TLS-specific flag values and returns GreetOptions.
func (t *TLS) ParseFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	serverName := ""
	if f := fs.Lookup("server-name"); f != nil {
		serverName = f.Value.String()
	}

	certFile := ""
	if f := fs.Lookup("client-cert"); f != nil {
		certFile = f.Value.String()
	}

	keyFile := ""
	if f := fs.Lookup("client-key"); f != nil {
		keyFile = f.Value.String()
	}

	caFile := ""
	if f := fs.Lookup("ca-cert"); f != nil {
		caFile = f.Value.String()
	}

	insecure := true
	if f := fs.Lookup("insecure"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				insecure = v
			}
		}
	}

	// Read files into bytes (CLI handles files; library callers pass bytes directly).
	var certPEM, keyPEM, caPEM []byte
	if certFile != "" {
		data, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading client cert %q: %w", certFile, err)
		}
		certPEM = data
	}
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading client key %q: %w", keyFile, err)
		}
		keyPEM = data
	}
	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading CA cert %q: %w", caFile, err)
		}
		caPEM = data
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(ProtocolName, &TLSConfig{
			ServerName:         serverName,
			CertPEM:            certPEM,
			KeyPEM:             keyPEM,
			CAPEM:              caPEM,
			InsecureSkipVerify: insecure,
		}),
	}, nil
}

// Handshake upgrades conn to TLS, performs the handshake, and returns the
// certificate chain with Chrome-style status checks. It returns the *tls.Conn
// so subsequent layers (e.g. http) can read/write over the encrypted channel.
func (t *TLS) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, greet.LayerData, greet.LayerTiming, error) {

	// Resolve TLS-layer config, defaulting to InsecureSkipVerify + target host.
	var tlsCfg *TLSConfig
	if raw := cfg.LayerConfig(ProtocolName); raw != nil {
		c, ok := raw.(*TLSConfig)
		if !ok {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: expected *TLSConfig, got %T", raw),
				Protocol: ProtocolName,
			}
		}
		tlsCfg = c
	}

	cryptoCfg, err := BuildTLSConfig(tlsCfg, host)
	if err != nil {
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrTLSConfigFailed,
			Message:  fmt.Sprintf("TLS config for %s:%d: %v", host, port, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	tlsConn := cryptotls.Client(conn, cryptoCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrTLSHandshakeFailed,
			Message:  fmt.Sprintf("TLS handshake with %s:%d failed: %v", host, port, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	ttfb := time.Since(start) // TLS handshake is atomic — TTFB = TTLB
	ttlb := ttfb

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrTLSHandshakeFailed,
			Message:  fmt.Sprintf("TLS handshake with %s:%d completed but no certificates presented", host, port),
			Protocol: ProtocolName,
		}
	}

	chain := make([]CertChainEntry, len(state.PeerCertificates))
	var chainStatus []string
	for i, cert := range state.PeerCertificates {
		isLeaf := i == 0
		certStatus := checkCert(cert, host, isLeaf)

		var ipAddrs []string
		for _, ip := range cert.IPAddresses {
			ipAddrs = append(ipAddrs, ip.String())
		}

		chain[i] = CertChainEntry{
			Subject:           cert.Subject.String(),
			Issuer:            cert.Issuer.String(),
			Serial:            cert.SerialNumber.String(),
			NotBefore:         cert.NotBefore.Format(time.RFC3339),
			NotAfter:          cert.NotAfter.Format(time.RFC3339),
			Version:           cert.Version,
			DNSNames:          cert.DNSNames,
			IPAddresses:       ipAddrs,
			IsCA:              cert.IsCA,
			SignatureAlgo:     cert.SignatureAlgorithm.String(),
			PublicKeyAlgo:     cert.PublicKeyAlgorithm.String(),
			SHA256Fingerprint: fmt.Sprintf("%x", sha256.Sum256(cert.Raw)),
			Status:            certStatus,
		}

		if chainStatus == nil && slices.ContainsFunc(certStatus, func(s string) bool { return s != "OK" }) {
			chainStatus = certStatus
		}
	}
	if chainStatus == nil {
		chainStatus = []string{"OK"}
	}

	return tlsConn, &TLSResult{CertChain: chain, Status: chainStatus}, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
}

// checkCert runs Chrome-style CERT_ error checks on a certificate.
func checkCert(cert *x509.Certificate, serverName string, isLeaf bool) []string {
	var statuses []string
	now := time.Now()

	// CERT_DATE_INVALID
	if now.Before(cert.NotBefore) {
		statuses = append(statuses, "CERT_DATE_INVALID(not-yet-valid)")
	} else if now.After(cert.NotAfter) {
		statuses = append(statuses, "CERT_DATE_INVALID(expired)")
	}

	// CERT_COMMON_NAME_INVALID (leaf only)
	if isLeaf && serverName != "" {
		if err := cert.VerifyHostname(serverName); err != nil {
			statuses = append(statuses, "CERT_COMMON_NAME_INVALID")
		}
	}

	// CERT_AUTHORITY_INVALID (self-signed) / CERT_SELF_SIGNED_LOCAL_NETWORK
	if isLeaf && isSelfSigned(cert) {
		if hasLocalNetworkNames(cert) {
			statuses = append(statuses, "CERT_SELF_SIGNED_LOCAL_NETWORK")
		} else {
			statuses = append(statuses, "CERT_AUTHORITY_INVALID(self-signed)")
		}
	}

	// CERT_NO_REVOCATION_MECHANISM
	if !isSelfSigned(cert) && len(cert.OCSPServer) == 0 && len(cert.CRLDistributionPoints) == 0 {
		statuses = append(statuses, "CERT_NO_REVOCATION_MECHANISM")
	}

	// CERT_WEAK_SIGNATURE_ALGORITHM
	if isWeakSignature(cert.SignatureAlgorithm) {
		statuses = append(statuses, "CERT_WEAK_SIGNATURE_ALGORITHM")
	}

	// CERT_WEAK_KEY
	if isWeakKey(cert) {
		statuses = append(statuses, "CERT_WEAK_KEY")
	}

	// CERT_VALIDITY_TOO_LONG
	// Apple announced (2020-02-19) that Safari would reject leaf certificates with validity
	// longer than 398 days, effective for certs issued on or after 2020-09-01. Chrome and
	// Firefox followed. The 398-day limit (not 397) gives a one-day grace for timezone
	// edge cases during renewal. Certs issued before 2020-09-01 were allowed up to 825
	// days under the older CA/B Forum Ballot 193 rules, so we only enforce this on newer certs.
	policyEnforceDate := time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
	if isLeaf && !cert.IsCA && !cert.NotBefore.Before(policyEnforceDate) {
		if cert.NotAfter.Sub(cert.NotBefore) > 398*24*time.Hour {
			statuses = append(statuses, "CERT_VALIDITY_TOO_LONG")
		}
	}

	if len(statuses) == 0 {
		statuses = append(statuses, "OK")
	}

	return statuses
}

func isSelfSigned(cert *x509.Certificate) bool {
	if !bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return false
	}
	// Also verify the signature to avoid false positives on cross-signed certs.
	return cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

func isWeakSignature(algo x509.SignatureAlgorithm) bool {
	switch algo {
	case x509.MD2WithRSA, x509.MD5WithRSA,
		x509.SHA1WithRSA, x509.DSAWithSHA1, x509.ECDSAWithSHA1:
		return true
	default:
		return false
	}
}

func isWeakKey(cert *x509.Certificate) bool {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return pub.N.BitLen() < 2048
	case *ecdsa.PublicKey:
		return pub.Curve.Params().BitSize < 256
	case ed25519.PublicKey:
		return false
	default:
		return false
	}
}

func hasLocalNetworkNames(cert *x509.Certificate) bool {
	for _, name := range cert.DNSNames {
		if strings.HasSuffix(strings.ToLower(name), ".local") {
			return true
		}
	}
	for _, ip := range cert.IPAddresses {
		addr, ok := netip.AddrFromSlice(ip)
		if ok && (addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()) {
			return true
		}
	}
	return false
}
