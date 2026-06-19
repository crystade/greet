package postgresql

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
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

// PostgreSQLResult holds the outcome of a PostgreSQL SSL probe.
type PostgreSQLResult struct {
	SSLSupported bool
}

// PostgreSQL implements a PostgreSQL SSLRequest probe.
// It is stateless — all configuration is passed via GreetOption.
type PostgreSQL struct{}

func (p *PostgreSQL) Name() string               { return ProtocolName }
func (p *PostgreSQL) Description() string        { return "PostgreSQL startup packet exchange" }
func (p *PostgreSQL) DefaultPort() int           { return DefaultPostgreSQLPort }
func (p *PostgreSQL) Transport() greet.Transport { return greet.TransportTCP }

func (p *PostgreSQL) RegisterFlags(fs *flag.FlagSet) {
	// Register flags on the FlagSet; ParseFlags reads the values back.
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
	cfg := &PostgreSQLConfig{SSLMode: sslMode}
	return []greet.GreetOption{greet.WithProtocolConfig(cfg)}, nil
}

func (p *PostgreSQL) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	// Resolve protocol-specific config
	sslMode := "prefer"
	if cfg.ProtocolConfig != nil {
		pgCfg, ok := cfg.ProtocolConfig.(*PostgreSQLConfig)
		if !ok {
			return nil, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid protocol config: expected *PostgreSQLConfig, got %T", cfg.ProtocolConfig),
				Protocol: ProtocolName,
			}
		}
		sslMode = pgCfg.SSLMode
	}

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
	conn, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(start)

	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetDeadline(deadline)
	}

	if sslMode == "disable" {
		// Skip SSL probe, just confirm TCP connection
		ttfb := rtt // no app data — TTFB equals RTT
		return greet.NewResult(ProtocolName, greet.TransportTCP, ttdr, rtt, ttfb, ttfb, true, &PostgreSQLResult{SSLSupported: false}), nil
	}

	// Phase 3: Send SSLRequest and read response (measures TTFB/TTLB from start)
	sslReq := make([]byte, 8)
	binary.BigEndian.PutUint32(sslReq[0:4], 8)               // length = 8
	binary.BigEndian.PutUint32(sslReq[4:8], SSLRequestMagic) // code = 80877103

	_, err = conn.Write(sslReq)
	if err != nil {
		code := ErrPostgreSQLSendFailed
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			code = ErrPostgreSQLStartupTimeout
		}
		return nil, &greet.GreetError{
			Code:     code,
			Message:  fmt.Sprintf("failed to send SSLRequest to %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	// Read single-byte response
	resp := make([]byte, 1)
	_, err = io.ReadFull(conn, resp)
	ttfb := time.Since(start) // TTFB = first (and only) byte of response
	ttlb := ttfb              // single byte response — TTLB equals TTFB

	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, &greet.GreetError{
				Code:     ErrPostgreSQLStartupTimeout,
				Message:  fmt.Sprintf("timed out reading PostgreSQL response from %s", addr),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		return nil, &greet.GreetError{
			Code:     ErrPostgreSQLMalformedResponse,
			Message:  fmt.Sprintf("failed to read PostgreSQL response from %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	switch resp[0] {
	case 'S':
		// Server supports SSL
		return greet.NewResult(ProtocolName, greet.TransportTCP, ttdr, rtt, ttfb, ttlb, true, &PostgreSQLResult{SSLSupported: true}), nil
	case 'E':
		return nil, &greet.GreetError{
			Code:     ErrPostgreSQLSSLRejected,
			Message:  fmt.Sprintf("PostgreSQL server at %s rejected SSL request", addr),
			Protocol: ProtocolName,
		}
	case 'N':
		// Server does not support SSL — this is a valid response, not an error
		return greet.NewResult(ProtocolName, greet.TransportTCP, ttdr, rtt, ttfb, ttlb, true, &PostgreSQLResult{SSLSupported: false}), nil
	default:
		return nil, &greet.GreetError{
			Code:     ErrPostgreSQLMalformedResponse,
			Message:  fmt.Sprintf("unexpected PostgreSQL response byte: 0x%02x", resp[0]),
			Protocol: ProtocolName,
		}
	}
}

func init() {
	greet.Register(&PostgreSQL{})
}
