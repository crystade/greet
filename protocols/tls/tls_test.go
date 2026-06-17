package tls

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/crystade/greet"
)

func TestTLS_Greet_Expired(t *testing.T) {
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "expired.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success (connection+leaf cert), got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	if !hasStatus(res.Status, "CERT_DATE_INVALID(expired)") {
		t.Errorf("Expected CERT_DATE_INVALID(expired), got statuses: %v", res.Status)
	}
}

func TestTLS_Greet_WrongHost(t *testing.T) {
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "wrong.host.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success (connection+leaf cert), got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	if !hasStatus(res.Status, "CERT_COMMON_NAME_INVALID") {
		t.Errorf("Expected CERT_COMMON_NAME_INVALID, got statuses: %v", res.Status)
	}
}

func TestTLS_Greet_SelfSigned(t *testing.T) {
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "self-signed.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success (connection+leaf cert), got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	if !hasStatus(res.Status, "CERT_AUTHORITY_INVALID(self-signed)") {
		t.Errorf("Expected CERT_AUTHORITY_INVALID(self-signed), got statuses: %v", res.Status)
	}
}

func TestTLS_Greet_UntrustedRoot(t *testing.T) {
	// InsecureSkipVerify=true should succeed even with untrusted root
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "untrusted-root.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success with InsecureSkipVerify, got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	if len(res.CertChain) == 0 || res.CertChain[0].Subject == "" {
		t.Error("Expected non-empty subject in leaf cert")
	}
}

func TestTLS_Greet_Revoked(t *testing.T) {
	// InsecureSkipVerify=true should succeed; cert may lack OCSP/CRL endpoints
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "revoked.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success with InsecureSkipVerify, got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	hasOK := hasStatus(res.Status, "OK")
	hasNoRev := hasStatus(res.Status, "CERT_NO_REVOCATION_MECHANISM")
	if !hasOK && !hasNoRev {
		t.Errorf("Expected OK or CERT_NO_REVOCATION_MECHANISM, got statuses: %v", res.Status)
	}
}

func TestTLS_Greet_PinningTest(t *testing.T) {
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "pinning-test.badssl.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success with InsecureSkipVerify, got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}
	if !hasStatus(res.Status, "OK") {
		t.Errorf("Expected OK, got statuses: %v", res.Status)
	}
}

func TestTLS_Greet_CertChain(t *testing.T) {
	tlsProto := &TLS{}
	result, err := tlsProto.Greet(context.Background(), "www.google.com", 443, defaultTimeout())
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	res, ok := result.Data.(*TLSResult)
	if !ok {
		t.Fatalf("Expected *TLSResult, got %T", result.Data)
	}

	if len(res.CertChain) == 0 {
		t.Fatal("Expected non-empty cert chain")
	}

	// Each entry should have its own status.
	for i, entry := range res.CertChain {
		if len(entry.Status) == 0 {
			t.Errorf("CertChain[%d] has empty status", i)
		}
	}

	// Leaf should not be a CA.
	if res.CertChain[0].IsCA {
		t.Error("Expected leaf cert IsCA=false")
	}

	// Last cert in chain is typically the root (self-signed, IsCA=true).
	last := res.CertChain[len(res.CertChain)-1]
	if !last.IsCA {
		t.Errorf("Expected last cert in chain to be a CA, got IsCA=%v", last.IsCA)
	}

	t.Logf("Chain has %d cert(s); chain-level status: %v", len(res.CertChain), res.Status)
	for i, entry := range res.CertChain {
		t.Logf("  [%d] Subject=%s Status=%v", i, entry.Subject, entry.Status)
	}
}

func TestTLS_Greet_ConnectionRefused(t *testing.T) {
	tlsProto := &TLS{}
	_, err := tlsProto.Greet(context.Background(), "localhost", 19999, defaultTimeout())
	if err == nil {
		t.Fatal("Expected error for refused connection, got nil")
	}
	ge, ok := err.(*greet.GreetError)
	if !ok {
		t.Fatalf("Expected *greet.GreetError, got %T", err)
	}
	// On different platforms the OS error code may vary between
	// connection_refused and connection_failed.
	if ge.Code != greet.ErrConnectionRefused && ge.Code != greet.ErrConnectionFailed {
		t.Errorf("Expected connection_refused or connection_failed, got %s", ge.Code)
	}
}

// helpers

func defaultTimeout() greet.GreetOption {
	return greet.WithTimeout(10 * time.Second)
}

func hasStatus(statuses []string, target string) bool {
	for _, s := range statuses {
		if s == target {
			return true
		}
	}
	return false
}

func containsString(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
