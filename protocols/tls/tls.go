package tls

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
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
	Subject           string   `json:"subject"`
	Issuer            string   `json:"issuer"`
	Serial            string   `json:"serial"`
	NotBefore         string   `json:"not_before"`
	NotAfter          string   `json:"not_after"`
	Version           int      `json:"version"`
	DNSNames          []string `json:"dns_names,omitempty"`
	IPAddresses       []string `json:"ip_addresses,omitempty"`
	IsCA              bool     `json:"is_ca"`
	SignatureAlgo     string   `json:"signature_algo"`
	PublicKeyAlgo     string   `json:"public_key_algo"`
	SHA256Fingerprint string   `json:"sha256_fingerprint"`

	// Status holds Chrome-style CERT_ checks for this individual certificate.
	// ref: https://github.com/chromium/chromium/blob/main/net/cert/cert_status_flags.cc
	Status []string `json:"status"`
}

// TLSResult holds the full certificate chain and the concluded chain status from a TLS handshake.
type TLSResult struct {
	// CertChain contains all presented certificates sorted from leaf to root.
	// The leaf certificate is always CertChain[0].
	CertChain []CertChainEntry `json:"cert_chain"`

	// Status is the final concluded chain status. It holds the statuses from
	// the first certificate (walking leaf→root) that has a non-OK check, or
	// ["OK"] if every certificate in the chain passes all checks.
	Status []string `json:"status"`
}

// TLS implements a TLS connectivity and certificate probe.
// It connects via TLS with InsecureSkipVerify, extracts the leaf
// certificate, and reports Chrome-style CERT_ status checks.
type TLS struct{}

func (t *TLS) Name() string               { return ProtocolName }
func (t *TLS) Description() string        { return "TLS leaf certificate check" }
func (t *TLS) DefaultPort() int           { return DefaultTLSPort }
func (t *TLS) Transport() greet.Transport { return greet.TransportTCP }

func (t *TLS) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	start := time.Now()

	// Phase 1: DNS resolution
	dns, err := greet.ResolveHost(ctx, host)
	if err != nil {
		return nil, err
	}
	ttdr := dns.TTDR

	// Phase 2: TCP connection (measures RTT from start)
	addr := net.JoinHostPort(dns.Address, strconv.Itoa(port))
	var d net.Dialer
	tcpConn, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(start)

	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	defer tcpConn.Close()

	// Phase 3: TLS handshake (measures TTFB/TTLB from start)
	tlsConn := tls.Client(tcpConn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, &greet.GreetError{
			Code:     ErrTLSHandshakeFailed,
			Message:  fmt.Sprintf("TLS handshake with %s:%d failed: %v", host, port, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	ttfb := time.Since(start) // TTFB = TLS handshake complete (certificate chain available)
	ttlb := ttfb              // TLS handshake is atomic — TTLB equals TTFB

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, &greet.GreetError{
			Code:     ErrTLSHandshakeFailed,
			Message:  fmt.Sprintf("TLS handshake with %s:%d completed but no certificates presented", host, port),
			Protocol: ProtocolName,
		}
	}

	// Build the certificate chain (leaf → root) with per-cert status.
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

		// The chain status is determined by the first cert (leaf→root)
		// that has a non-OK status.
		if chainStatus == nil && slices.ContainsFunc(certStatus, func(s string) bool { return s != "OK" }) {
			chainStatus = certStatus
		}
	}
	if chainStatus == nil {
		chainStatus = []string{"OK"}
	}

	result := &TLSResult{
		CertChain: chain,
		Status:    chainStatus,
	}

	return greet.NewResult(ProtocolName, greet.TransportTCP, ttdr, rtt, ttfb, ttlb, true, result), nil
}

// isTCPError returns true if the error comes from a TCP dial attempt
// (as opposed to a TLS handshake failure after connecting).
// It checks for net.OpError where Op is "dial".
func isTCPError(err error) bool {
	var opErr *net.OpError
	if err == nil {
		return false
	}
	// A TCP dial error is a *net.OpError with Op="dial"
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	// Unwrap once — tls.DialWithDialer may wrap the dial error
	if ue, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range ue.Unwrap() {
			if errors.As(e, &opErr) && opErr.Op == "dial" {
				return true
			}
		}
	}
	if ue, ok := err.(interface{ Unwrap() error }); ok {
		if inner := ue.Unwrap(); inner != nil {
			if errors.As(inner, &opErr) && opErr.Op == "dial" {
				return true
			}
		}
	}
	return false
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

func init() {
	greet.Register(&TLS{})
}
