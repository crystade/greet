package tcp

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the TCP protocol.
const ProtocolName = "tcp"

// DefaultTCPPort is the well-known port for generic TCP probes.
const DefaultTCPPort = 80

// TCP implements a generic TCP dial-and-close probe.
// It measures connection timing without sending any application data.
type TCP struct{}

func (t *TCP) Name() string               { return ProtocolName }
func (t *TCP) Description() string        { return "Generic TCP dial + close" }
func (t *TCP) DefaultPort() int           { return DefaultTCPPort }
func (t *TCP) Transport() greet.Transport { return greet.TransportTCP }

func (t *TCP) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
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
	conn, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(start)

	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	conn.Close()

	// No application data — TTFB and TTLB equal RTT
	return greet.NewResult(ProtocolName, greet.TransportTCP, ttdr, rtt, rtt, rtt, true, nil), nil
}

func init() {
	greet.Register(&TCP{})
}
