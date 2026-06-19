package greet

import "time"

// GreetResult holds the outcome of a protocol handshake.
// Protocols populate Data with a typed result struct (e.g. *MinecraftResult);
// generic TCP/UDP set Data to nil.
//
// All four timing metrics share the same starting anchor time (start := time.Now()
// at the beginning of the operation):
//
//	|------|         TTDR: from start to when DNS resolves
//	|------|--|      RTT:  from start to when first ACK at TCP transport or Receive at UDP
//	|------|--|-|    TTFB: from start to when first byte response at the correct protocol layer
//	|------|--|-------| TTLB: from start to when last byte response at the correct protocol layer
type GreetResult struct {
	Protocol  string
	Transport Transport
	TTDR      time.Duration // Time to DNS Resolved
	RTT       time.Duration // Round Trip Time
	TTFB      time.Duration // Time to First Byte
	TTLB      time.Duration // Time to Last Byte
	Success   bool

	// Data holds protocol-specific payload. Nil for generic TCP/UDP.
	// Cast to the protocol's result type for typed access, e.g.:
	//   mc, ok := result.Data.(*minecraft.MinecraftResult)
	Data any
}

// NewResult creates a GreetResult with the given fields.
func NewResult(protocol string, transport Transport, ttdr, rtt, ttfb, ttlb time.Duration, success bool, data any) *GreetResult {
	return &GreetResult{
		Protocol:  protocol,
		Transport: transport,
		TTDR:      ttdr,
		RTT:       rtt,
		TTFB:      ttfb,
		TTLB:      ttlb,
		Success:   success,
		Data:      data,
	}
}
