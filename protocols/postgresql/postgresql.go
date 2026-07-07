package postgresql

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/crystade/greet"
)

// https://www.postgresql.org/docs/current/protocol-message-formats.html#PROTOCOL-MESSAGE-FORMATS-SSLREQUEST
// https://github.com/transact-rs/sqlx/blob/main/sqlx-postgres/src/message/ssl_request.rs

// ProtocolName is the registered name for the PostgreSQL protocol.
const ProtocolName = "postgresql"

// DefaultPostgreSQLPort is the well-known port for PostgreSQL.
const DefaultPostgreSQLPort = 5432

// SSLRequest magic number: Int32(8) + Int32(80877103)
// 8 = message length (including self)
// 80877103 = (1234 << 16) | 5679
const SSLRequestMagic = 80877103

// PostgreSQLConfig holds protocol-specific configuration for PostgreSQL.
type PostgreSQLConfig struct {
	SSLMode string // "prefer" (default) or "disable"
}

func (c *PostgreSQLConfig) LayerConfigName() string { return ProtocolName }

// PostgreSQLResult holds the outcome of a PostgreSQL SSL probe.
type PostgreSQLResult struct {
	SSLSupported bool
}

func (r *PostgreSQLResult) LayerDataName() string { return ProtocolName }

// PostgreSQL implements a PostgreSQL SSLRequest probe.
// It is stateless — all configuration is passed via GreetOption.
type PostgreSQL struct{}

// New creates a new PostgreSQL application layer.
func New() *PostgreSQL {
	return &PostgreSQL{}
}

func (p *PostgreSQL) Name() string                   { return ProtocolName }
func (p *PostgreSQL) Description() string            { return "PostgreSQL startup packet exchange" }
func (p *PostgreSQL) DefaultPort() int               { return DefaultPostgreSQLPort }
func (p *PostgreSQL) BaseTransport() greet.Transport { return greet.TransportTCP }

func (p *PostgreSQL) RegisterFlags(fs *flag.FlagSet) {
	fs.String("sslmode", "prefer", "SSL mode: prefer (try SSL) or disable (skip SSL)")
}

func (p *PostgreSQL) ParseFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	sslMode := fs.Lookup("sslmode").Value.String()
	switch sslMode {
	case "prefer", "disable":
		// valid
	default:
		return nil, fmt.Errorf("invalid sslmode %q: must be 'prefer' or 'disable'", sslMode)
	}
	return []greet.GreetOption{greet.WithLayerConfig(ProtocolName, &PostgreSQLConfig{SSLMode: sslMode})}, nil
}

func (p *PostgreSQL) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, greet.LayerData, greet.LayerTiming, error) {

	// Resolve per-layer config
	sslMode := "prefer"
	if raw := cfg.LayerConfig(ProtocolName); raw != nil {
		pgCfg, ok := raw.(*PostgreSQLConfig)
		if !ok {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: expected *PostgreSQLConfig, got %T", raw),
				Protocol: ProtocolName,
			}
		}
		sslMode = pgCfg.SSLMode
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	addr := conn.RemoteAddr().String()

	if sslMode == "disable" {
		// Skip SSL probe — TCP connection alone is the result.
		now := time.Since(start)
		return conn, &PostgreSQLResult{SSLSupported: false}, greet.LayerTiming{TTFB: now, TTLB: now}, nil
	}

	// Phase 3: Send SSLRequest and read response (measures TTFB/TTLB from start)
	sslReq := make([]byte, 8)
	binary.BigEndian.PutUint32(sslReq[0:4], 8)               // length = 8
	binary.BigEndian.PutUint32(sslReq[4:8], SSLRequestMagic) // code = 80877103

	_, err := conn.Write(sslReq)
	if err != nil {
		code := ErrPostgreSQLSendFailed
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			code = ErrPostgreSQLStartupTimeout
		}
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     code,
			Message:  fmt.Sprintf("failed to send SSLRequest to %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	// Read single-byte response
	resp := make([]byte, 1)
	_, readErr := io.ReadFull(conn, resp)
	ttfb := time.Since(start) // TTFB = first (and only) byte of response
	ttlb := ttfb              // single byte response — TTLB equals TTFB

	if readErr != nil {
		var netErr net.Error
		if errors.As(readErr, &netErr) && netErr.Timeout() {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     ErrPostgreSQLStartupTimeout,
				Message:  fmt.Sprintf("timed out reading PostgreSQL response from %s", addr),
				Protocol: ProtocolName,
				Cause:    readErr,
			}
		}
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrPostgreSQLMalformedResponse,
			Message:  fmt.Sprintf("failed to read PostgreSQL response from %s: %v", addr, readErr),
			Protocol: ProtocolName,
			Cause:    readErr,
		}
	}

	switch resp[0] {
	case 'S':
		return conn, &PostgreSQLResult{SSLSupported: true}, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
	case 'E':
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrPostgreSQLSSLRejected,
			Message:  fmt.Sprintf("PostgreSQL server at %s rejected SSL request", addr),
			Protocol: ProtocolName,
		}
	case 'N':
		return conn, &PostgreSQLResult{SSLSupported: false}, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
	default:
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrPostgreSQLMalformedResponse,
			Message:  fmt.Sprintf("unexpected PostgreSQL response byte: 0x%02x", resp[0]),
			Protocol: ProtocolName,
		}
	}
}
