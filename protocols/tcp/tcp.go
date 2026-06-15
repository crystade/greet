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
// It measures connection latency without sending any application data.
type TCP struct{}

func (t *TCP) Name() string               { return ProtocolName }
func (t *TCP) Description() string        { return "Generic TCP dial + close (latency only)" }
func (t *TCP) DefaultPort() int           { return DefaultTCPPort }
func (t *TCP) Transport() greet.Transport { return greet.TransportTCP }

func (t *TCP) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	latency := time.Since(start)

	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	conn.Close()

	return greet.NewResult(ProtocolName, greet.TransportTCP, latency, true, nil), nil
}

func init() {
	greet.Register(&TCP{})
}
