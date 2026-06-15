package greet

import (
	"encoding/json"
	"time"
)

// customDuration marshals time.Duration as a human-readable string (e.g. "45.123ms").
type customDuration time.Duration

func (d customDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *customDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = customDuration(parsed)
	return nil
}

// GreetResult holds the outcome of a protocol handshake.
// Protocols populate Data with a typed result struct (e.g. *MinecraftResult);
// generic TCP/UDP set Data to nil.
type GreetResult struct {
	Protocol   string         `json:"protocol"`
	Transport  Transport      `json:"transport"`
	Latency    time.Duration  `json:"-"`
	LatencyStr customDuration `json:"latency"`
	LatencyMs  float64        `json:"latency_ms"`
	Success    bool           `json:"success"`

	// Data holds protocol-specific payload. Nil for generic TCP/UDP.
	// Cast to the protocol's result type for typed access, e.g.:
	//   mc, ok := result.Data.(*minecraft.MinecraftResult)
	Data any `json:"data,omitempty"`
}

// NewResult creates a GreetResult and eagerly populates derived latency fields
// (LatencyStr, LatencyMs) so they are available for programmatic access.
func NewResult(protocol string, transport Transport, latency time.Duration, success bool, data any) *GreetResult {
	r := &GreetResult{
		Protocol:  protocol,
		Transport: transport,
		Latency:   latency,
		Success:   success,
		Data:      data,
	}
	r.populateDerived()
	return r
}

// populateDerived fills computed JSON fields from Latency.
func (r *GreetResult) populateDerived() {
	r.LatencyStr = customDuration(r.Latency)
	r.LatencyMs = float64(r.Latency.Nanoseconds()) / 1e6
}

// MarshalJSON implements custom JSON serialization so that latency is
// human-readable and latency_ms is included.
func (r GreetResult) MarshalJSON() ([]byte, error) {
	r.populateDerived()
	// Use an alias to avoid infinite recursion
	type alias GreetResult
	return json.Marshal(alias(r))
}
