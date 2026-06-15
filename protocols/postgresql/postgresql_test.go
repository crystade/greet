package postgresql

import (
	"encoding/binary"
	"testing"
)

func TestSSLRequestMagic(t *testing.T) {
	if SSLRequestMagic != 80877103 {
		t.Errorf("SSLRequestMagic = %d, want 80877103", SSLRequestMagic)
	}

	// Verify it matches (1234 << 16) | 5679
	expected := (1234 << 16) | 5679
	if SSLRequestMagic != uint32(expected) {
		t.Errorf("SSLRequestMagic = %d, want %d (1234<<16 | 5679)", SSLRequestMagic, expected)
	}
}

func TestDefaultPostgreSQLPort(t *testing.T) {
	if DefaultPostgreSQLPort != 5432 {
		t.Errorf("DefaultPostgreSQLPort = %d, want 5432", DefaultPostgreSQLPort)
	}
}

func TestSSLRequestPacketFormat(t *testing.T) {
	// Build the SSLRequest message as the protocol does
	sslReq := make([]byte, 8)
	binary.BigEndian.PutUint32(sslReq[0:4], 8)
	binary.BigEndian.PutUint32(sslReq[4:8], SSLRequestMagic)

	// Verify length field
	length := binary.BigEndian.Uint32(sslReq[0:4])
	if length != 8 {
		t.Errorf("SSLRequest length = %d, want 8", length)
	}

	// Verify magic field
	magic := binary.BigEndian.Uint32(sslReq[4:8])
	if magic != 80877103 {
		t.Errorf("SSLRequest magic = %d, want 80877103", magic)
	}
}
