package greet

import (
	"context"
	"flag"
	"net"
	"time"
)

// LayerTiming holds TTFB and TTLB for a single application layer's handshake.
// Both are measured from the same start anchor as the top-level GreetResult timings.
type LayerTiming struct {
	TTFB time.Duration
	TTLB time.Duration
}

// TransportLayer establishes a network connection. It owns DNS resolution,
// dialing, RTT measurement, and connection-level error classification.
// tcp and udp implement this interface.
type TransportLayer interface {
	// Name returns the transport slug (e.g. "tcp", "udp").
	Name() string

	// DefaultPort is the fallback port when none is specified.
	DefaultPort() int

	// Transport returns TransportTCP or TransportUDP.
	Transport() Transport

	// Dial resolves host, establishes the connection, and returns it together
	// with DNS and RTT timings. The caller is responsible for closing conn.
	//
	// start is the shared timing anchor owned by the engine; RTT (and TTDR
	// where applicable) must be measured as time.Since(start) so every layer
	// in the stack reports timings relative to the same origin.
	Dial(ctx context.Context, start time.Time, host string, port int) (conn net.Conn, ttdr time.Duration, rtt time.Duration, err error)
}

// AppLayer runs an application-level handshake over an existing connection.
// tls, minecraft, postgresql, ssh, http, and https implement this interface.
type AppLayer interface {
	// Name returns the protocol slug (e.g. "tls", "minecraft").
	Name() string

	// Description is a one-line human-readable summary.
	Description() string

	// DefaultPort is the well-known port for this application protocol.
	DefaultPort() int

	// BaseTransport declares which transport this layer requires (tcp or udp).
	BaseTransport() Transport

	// Handshake performs the application-level exchange over conn.
	// start is the shared timing anchor owned by the engine; TTFB and TTLB
	// must be measured as time.Since(start) so timings are relative to the
	// same origin as the transport RTT and any lower app layers.
	// host and port are the original target (not the resolved IP).
	// cfg is the per-layer config retrieved via GreetConfig.LayerConfig(name).
	// Returns the (possibly upgraded) connection to pass to the next layer,
	// protocol-specific data, timing anchored from the shared start, and any error.
	// The caller (engine) owns closing the original transport conn; layers that
	// upgrade conn (e.g. TLS) must not close it — the engine's defer handles it.
	Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *GreetConfig) (next net.Conn, data any, timing LayerTiming, err error)
}

// FlaggedLayer is an optional interface for layers that need CLI-specific flags.
// The CLI auto-discovers this via type assertion on both TransportLayer and AppLayer.
type FlaggedLayer interface {
	// RegisterFlags adds layer-specific flags to a flag.FlagSet.
	RegisterFlags(fs *flag.FlagSet)

	// ParseFlags returns layer-specific options derived from parsed flags.
	// Called after fs.Parse(); implementations should read values via fs.Lookup.
	ParseFlags(fs *flag.FlagSet) ([]GreetOption, error)
}
