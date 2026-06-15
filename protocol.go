package greet

import (
	"context"
	"flag"
)

// Transport is the network transport layer a protocol operates over.
type Transport string

const (
	TransportTCP Transport = "tcp"
	TransportUDP Transport = "udp"
)

// String returns the human-readable transport name.
func (t Transport) String() string { return string(t) }

// Protocol is the interface every greeter implementation must satisfy.
// Implementations register themselves via greet.Register() in init().
type Protocol interface {
	// Name returns the protocol slug used by the CLI and registry (e.g. "minecraft").
	Name() string

	// Description is a one-line human-readable summary.
	Description() string

	// DefaultPort is the well-known port (e.g. 25565 for Minecraft).
	DefaultPort() int

	// Transport returns TransportTCP or TransportUDP.
	Transport() Transport

	// Greet performs the handshake and returns a result or error.
	Greet(ctx context.Context, host string, port int, opts ...GreetOption) (*GreetResult, error)
}

// FlaggedProtocol is an optional interface for protocols that need
// CLI-specific flags. The CLI auto-discovers this via type assertion.
type FlaggedProtocol interface {
	Protocol

	// RegisterFlags adds protocol-specific flags to a flag.FlagSet.
	RegisterFlags(fs *flag.FlagSet)

	// ParseFlags returns protocol-specific options from parsed flags.
	ParseFlags(fs *flag.FlagSet) ([]GreetOption, error)
}
