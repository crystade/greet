package greet

import "time"

// DefaultTimeout is the deadline for the entire handshake operation.
const DefaultTimeout = 5 * time.Second

// GreetConfig holds resolved options for a Greet call.
type GreetConfig struct {
	Timeout     time.Duration
	layerConfig map[string]any // keyed by layer name; set via WithLayerConfig
}

// LayerConfig returns the per-layer config registered under name, or nil.
// Each layer type-asserts the result to its own config type.
func (c *GreetConfig) LayerConfig(name string) any {
	if c.layerConfig == nil {
		return nil
	}
	return c.layerConfig[name]
}

// GreetOption is a functional option for configuring a Greet call.
type GreetOption func(*GreetConfig)

// WithTimeout overrides the default handshake timeout.
func WithTimeout(d time.Duration) GreetOption {
	return func(c *GreetConfig) { c.Timeout = d }
}

// WithLayerConfig attaches a typed, per-layer config value.
// name must match the layer's Name() return value exactly.
// Each layer retrieves its config via cfg.LayerConfig(name) and type-asserts it.
//
// Example:
//
//	greet.WithLayerConfig("minecraft", &minecraft.Config{ProtocolVersion: 775})
func WithLayerConfig(name string, cfg any) GreetOption {
	return func(c *GreetConfig) {
		if c.layerConfig == nil {
			c.layerConfig = make(map[string]any)
		}
		c.layerConfig[name] = cfg
	}
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
