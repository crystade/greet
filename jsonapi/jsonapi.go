// Package jsonapi provides JSON-serializable request and response models for
// the greet engine, together with mappers that convert to and from the domain
// models. The domain models themselves carry no jsonapi tags; all JSON shaping
// lives here so the wire format can evolve independently.
//
// Structure mirrors the protocols/ tree: this root package holds the core
// Request/Response types plus a mapper registry, and each protocol has its own
// subpackage (jsonapi/tls, jsonapi/ssh, ...) that registers its data mapper and
// option decoder via Register(c *Codec). Import jsonapi/all to opt into every protocol.
//
// All JSON field names use camelCase.
package jsonapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/crystade/greet"
)

// Request is the JSON-serializable input for a greet operation.
type Request struct {
	// Protocol is the registered protocol name, e.g. "tls", "minecraft".
	Protocol string `json:"protocol"`

	// Target is the host or host:port to probe.
	Target string `json:"target"`

	// Options holds per-protocol options keyed by protocol name. Each value is
	// decoded by the option decoder registered for that protocol.
	Options map[string]json.RawMessage `json:"options,omitempty"`
}

// Response is the JSON-serializable outcome of a greet operation.
type Response struct {
	// Protocol is the name of the outermost application layer.
	Protocol string `json:"protocol"`

	// Transport is the base transport used ("tcp" or "udp").
	Transport string `json:"transport"`

	// TTDRMs is the time to DNS resolution, in milliseconds.
	TTDRMs float64 `json:"ttdrMs"`

	// RTTMs is the time to establish the transport connection, in milliseconds.
	RTTMs float64 `json:"rttMs"`

	// Success is true only when every layer in the stack succeeded.
	Success bool `json:"success"`

	// Layers contains per-layer results in stack order.
	Layers []Layer `json:"layers"`
}

// Layer is the JSON-serializable result of a single layer.
type Layer struct {
	// Name is the layer slug (e.g. "tcp", "tls", "minecraft").
	Name string `json:"name"`

	// TTFBMs is the time to first byte from the shared start anchor, in milliseconds.
	TTFBMs float64 `json:"ttfbMs"`

	// TTLBMs is the time to last byte from the shared start anchor, in milliseconds.
	TTLBMs float64 `json:"ttlbMs"`

	// Success indicates whether this layer completed without error.
	Success bool `json:"success"`

	// Data holds the protocol-specific payload for this layer. Nil for
	// transport-only layers (tcp, udp).
	Data any `json:"data,omitempty"`
}

// DataMapper converts a protocol-specific domain payload (as stored in
// greet.LayerResult.Data) into its JSON-serializable form.
type DataMapper func(data greet.LayerData) any

// OptionDecoder decodes a protocol's raw JSON options into engine GreetOptions.
type OptionDecoder func(raw json.RawMessage) ([]greet.GreetOption, error)

// Codec holds data mappers and option decoders for JSON serialization.
type Codec struct {
	mu             sync.RWMutex
	dataMappers    map[string]DataMapper
	optionDecoders map[string]OptionDecoder
}

// NewCodec creates a new empty Codec.
func NewCodec() *Codec {
	return &Codec{
		dataMappers:    make(map[string]DataMapper),
		optionDecoders: make(map[string]OptionDecoder),
	}
}

// RegisterDataMapper registers a DataMapper for the named protocol layer.
func (c *Codec) RegisterDataMapper(name string, m DataMapper) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := c.dataMappers[key]; dup {
		panic(fmt.Sprintf("jsonapi: data mapper %q already registered", key))
	}
	c.dataMappers[key] = m
}

// RegisterOptionDecoder registers an OptionDecoder for the named protocol.
func (c *Codec) RegisterOptionDecoder(name string, d OptionDecoder) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := c.optionDecoders[key]; dup {
		panic(fmt.Sprintf("jsonapi: option decoder %q already registered", key))
	}
	c.optionDecoders[key] = d
}

// FromGreetResult maps a domain *greet.GreetResult to a JSON-serializable
// *Response. It returns nil when result is nil.
func (c *Codec) FromGreetResult(result *greet.GreetResult) *Response {
	if result == nil {
		return nil
	}

	resp := &Response{
		Protocol:  result.Protocol,
		Transport: result.Transport.String(),
		TTDRMs:    DurationToMs(result.TTDR),
		RTTMs:     DurationToMs(result.RTT),
		Success:   result.Success,
	}

	if len(result.Layers) > 0 {
		resp.Layers = make([]Layer, len(result.Layers))
		for i, l := range result.Layers {
			resp.Layers[i] = Layer{
				Name:    l.Name,
				TTFBMs:  DurationToMs(l.TTFB),
				TTLBMs:  DurationToMs(l.TTLB),
				Success: l.Success,
				Data:    c.mapLayerData(l.Name, l.Data),
			}
		}
	}

	return resp
}

// mapLayerData looks up the registered DataMapper for the layer and applies it.
// Layers with no registered mapper (or nil data) pass their data through as-is.
func (c *Codec) mapLayerData(name string, data greet.LayerData) any {
	if data == nil {
		return nil
	}
	c.mu.RLock()
	m, ok := c.dataMappers[strings.ToLower(name)]
	c.mu.RUnlock()
	if !ok {
		return data
	}
	return m(data)
}

// GreetOptions builds engine GreetOptions from the request.
// It dispatches each per-protocol option blob to its registered decoder.
// An unknown protocol key in Options is an error.
func (r *Request) GreetOptions(c *Codec) ([]greet.GreetOption, error) {
	var opts []greet.GreetOption

	for name, raw := range r.Options {
		c.mu.RLock()
		d, ok := c.optionDecoders[strings.ToLower(name)]
		c.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("jsonapi: no option decoder registered for %q", name)
		}
		decoded, err := d(raw)
		if err != nil {
			return nil, fmt.Errorf("jsonapi: decoding %q options: %w", name, err)
		}
		opts = append(opts, decoded...)
	}

	return opts, nil
}

// DurationToMs converts a time.Duration to fractional milliseconds.
func DurationToMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
