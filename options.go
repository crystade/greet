package greet

import "time"

// DefaultTimeout is the deadline for the entire handshake operation.
const DefaultTimeout = 5 * time.Second

// GreetConfig holds resolved options for a Greet call.
type GreetConfig struct {
	Timeout        time.Duration
	ProtocolConfig any // protocol-specific config set via WithProtocolConfig
}

// GreetOption is a functional option for configuring a Greet call.
type GreetOption func(*GreetConfig)

// WithTimeout overrides the default handshake timeout.
func WithTimeout(d time.Duration) GreetOption {
	return func(c *GreetConfig) { c.Timeout = d }
}

// WithProtocolConfig carries protocol-specific configuration from
// FlaggedProtocol.ParseFlags into the Greet call.
func WithProtocolConfig(cfg any) GreetOption {
	return func(c *GreetConfig) { c.ProtocolConfig = cfg }
}

// ResolveOptions applies functional options and fills defaults.
func ResolveOptions(opts ...GreetOption) GreetConfig {
	cfg := GreetConfig{
		Timeout: DefaultTimeout,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
