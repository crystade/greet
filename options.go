package greet

import "time"

// GreetConfig holds resolved options for a Greet call.
type GreetConfig struct {
	layerConfig  map[string]LayerConfig // keyed by layer name; set via WithLayerConfig
	layerTimeout map[string]time.Duration
}

// LayerConfig returns the per-layer config registered under name, or nil.
// Each layer type-asserts the result to its own config type.
func (c *GreetConfig) LayerConfig(name string) LayerConfig {
	if c.layerConfig == nil {
		return nil
	}
	return c.layerConfig[name]
}

// LayerConfig is the interface that all protocol-specific config types must implement.
type LayerConfig interface {
	LayerConfigName() string
}

// GreetOption is a functional option for configuring a Greet call.
type GreetOption func(*GreetConfig)

// WithLayerTimeout sets an independent timeout for a specific layer's handshake.
// This timeout is NOT capped by any global deadline — the caller controls global
// timeout via context.WithTimeout on the context passed to Greet/GreetStack.
func WithLayerTimeout(name string, d time.Duration) GreetOption {
	return func(c *GreetConfig) {
		if c.layerTimeout == nil {
			c.layerTimeout = make(map[string]time.Duration)
		}
		c.layerTimeout[name] = d
	}
}

// LayerTimeout returns the per-layer timeout for the named layer, or 0 if none.
func (c *GreetConfig) LayerTimeout(name string) time.Duration {
	if c.layerTimeout == nil {
		return 0
	}
	return c.layerTimeout[name]
}

// WithLayerConfig attaches a typed, per-layer config value.
// name must match the layer's Name() return value exactly.
// Each layer retrieves its config via cfg.LayerConfig(name) and type-asserts it.
//
// Example:
//
//	greet.WithLayerConfig("minecraft", &minecraft.Config{ProtocolVersion: 775})
func WithLayerConfig(name string, cfg LayerConfig) GreetOption {
	return func(c *GreetConfig) {
		if c.layerConfig == nil {
			c.layerConfig = make(map[string]LayerConfig)
		}
		c.layerConfig[name] = cfg
	}
}

// ResolveOptions applies functional options and fills defaults.
func ResolveOptions(opts ...GreetOption) GreetConfig {
	cfg := GreetConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
