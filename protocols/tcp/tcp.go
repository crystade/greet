package tcp

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the TCP transport.
const ProtocolName = "tcp"

// DefaultTCPPort is the fallback port for generic TCP probes.
const DefaultTCPPort = 80

// TCP implements greet.TransportLayer for plain TCP connections.
// It resolves DNS, dials, and returns the open conn together with TTDR and RTT.
// It sends no application data.
type TCP struct{}

func (t *TCP) Name() string               { return ProtocolName }
func (t *TCP) DefaultPort() int           { return DefaultTCPPort }
func (t *TCP) Transport() greet.Transport { return greet.TransportTCP }

// Dial resolves host, establishes a TCP connection, and returns it open.
// start is the shared timing anchor owned by the engine; TTDR and RTT are
// measured relative to it. The caller is responsible for closing conn.
func (t *TCP) Dial(ctx context.Context, start time.Time, host string, port int) (net.Conn, time.Duration, time.Duration, error) {
	dns, err := greet.ResolveHost(ctx, host)
	if err != nil {
		return nil, 0, 0, err
	}
	ttdr := time.Since(start) // anchored to the shared start

	addr := net.JoinHostPort(dns.Address, strconv.Itoa(port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(start)

	if err != nil {
		return nil, ttdr, rtt, classifyTCPError(err, host, port)
	}
	return conn, ttdr, rtt, nil
}

func init() {
	greet.RegisterTransport(&TCP{})
}
