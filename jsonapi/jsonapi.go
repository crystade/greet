// Package jsonapi provides JSON-serializable request and response models for
// the greet engine, together with mappers that convert to and from the domain
// models. The domain models themselves carry no jsonapi tags; all JSON shaping
// lives here so the wire format can evolve independently.
//
// Structure mirrors the protocols/ tree: this root package holds the core
// Request/Response types plus a mapper registry, and each protocol has its own
// subpackage (jsonapi/tls, jsonapi/ssh, ...) that registers its data mapper and
// option decoder via init(). Import jsonapi/all to opt into every protocol.
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

	// TimeoutMs overrides the default handshake timeout, in milliseconds.
	// Zero or negative means use the engine default.
	TimeoutMs int64 `json:"timeoutMs,omitempty"`

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
type DataMapper func(data any) any

// OptionDecoder decodes a protocol's raw JSON options into engine GreetOptions.
type OptionDecoder func(raw json.RawMessage) ([]greet.GreetOption, error)

var (
	mu             sync.RWMutex
	dataMappers    = map[string]DataMapper{}
	optionDecoders = map[string]OptionDecoder{}
)

// RegisterDataMapper registers a DataMapper for the named protocol layer.
// Call this from init() in each jsonapi protocol subpackage.
func RegisterDataMapper(name string, m DataMapper) {
	mu.Lock()
	defer mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := dataMappers[key]; dup {
		panic(fmt.Sprintf("jsonapi: data mapper %q already registered", key))
	}
	dataMappers[key] = m
}

// RegisterOptionDecoder registers an OptionDecoder for the named protocol.
// Call this from init() in each jsonapi protocol subpackage that has options.
func RegisterOptionDecoder(name string, d OptionDecoder) {
	mu.Lock()
	defer mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := optionDecoders[key]; dup {
		panic(fmt.Sprintf("jsonapi: option decoder %q already registered", key))
	}
	optionDecoders[key] = d
}

// FromGreetResult maps a domain *greet.GreetResult to a JSON-serializable
// *Response. It returns nil when result is nil.
func FromGreetResult(result *greet.GreetResult) *Response {
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
				Data:    mapLayerData(l.Name, l.Data),
			}
		}
	}

	return resp
}

// mapLayerData looks up the registered DataMapper for the layer and applies it.
// Layers with no registered mapper (or nil data) pass their data through as-is.
func mapLayerData(name string, data any) any {
	if data == nil {
		return nil
	}
	mu.RLock()
	m, ok := dataMappers[strings.ToLower(name)]
	mu.RUnlock()
	if !ok {
		return data
	}
	return m(data)
}

// GreetOptions builds engine GreetOptions from the request. It applies the
// timeout and dispatches each per-protocol option blob to its registered
// decoder. An unknown protocol key in Options is an error.
func (r *Request) GreetOptions() ([]greet.GreetOption, error) {
	var opts []greet.GreetOption

	if r.TimeoutMs > 0 {
		opts = append(opts, greet.WithTimeout(time.Duration(r.TimeoutMs)*time.Millisecond))
	}

	for name, raw := range r.Options {
		mu.RLock()
		d, ok := optionDecoders[strings.ToLower(name)]
		mu.RUnlock()
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
