package greet

import "time"

// GreetResult holds the outcome of a protocol handshake.
// Protocols populate Data with a typed result struct (e.g. *MinecraftResult);
// generic TCP/UDP set Data to nil.
type GreetResult struct {
	Protocol  string
	Transport Transport
	Latency   time.Duration
	Success   bool

	// Data holds protocol-specific payload. Nil for generic TCP/UDP.
	// Cast to the protocol's result type for typed access, e.g.:
	//   mc, ok := result.Data.(*minecraft.MinecraftResult)
	Data any
}

// NewResult creates a GreetResult with the given fields.
func NewResult(protocol string, transport Transport, latency time.Duration, success bool, data any) *GreetResult {
	return &GreetResult{
		Protocol:  protocol,
		Transport: transport,
		Latency:   latency,
		Success:   success,
		Data:      data,
	}
}
