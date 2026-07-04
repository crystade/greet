package greet

import "time"

// LayerResult holds the outcome of a single layer within a stack. This
// includes the base transport layer (tcp/udp) as well as each application
// layer. Timing is anchored from the same start as the parent GreetResult.
type LayerResult struct {
	// Name is the layer slug (e.g. "tcp", "tls", "minecraft").
	Name string

	// TTFB is the time from the shared start anchor to the first byte of the
	// layer's response. For the transport layer, TTFB equals RTT.
	TTFB time.Duration

	// TTLB is the time from the shared start anchor to the last byte of the
	// layer's response. For the transport layer, TTLB equals RTT.
	TTLB time.Duration

	// Success indicates whether this layer completed without error.
	Success bool

	// Data holds the protocol-specific payload for this layer (e.g. *tls.TLSResult).
	// Nil for transport-only layers (tcp, udp).
	Data any
}

// GreetResult holds the aggregated outcome of a layered protocol stack execution.
//
// Timing anchors (all share the same start time):
//
//	|------|           TTDR: DNS resolved
//	|------|--|        RTT:  transport connection established
//	        per-layer TTFB/TTLB tracked in Layers[i]
type GreetResult struct {
	// Protocol is the name of the outermost (application) layer, e.g. "tls".
	// For a bare TCP probe this is "tcp".
	Protocol string

	// Transport is the base transport used (tcp or udp).
	Transport Transport

	// TTDR is the time to DNS resolution, shared across all layers.
	TTDR time.Duration

	// RTT is the time to establish the transport connection.
	RTT time.Duration

	// Success is true only when every layer in the stack succeeded.
	Success bool

	// Layers contains per-layer results in stack order. Layers[0] is always the
	// base transport (tcp/udp) with TTFB = TTLB = RTT, followed by each
	// application layer in execution order.
	Layers []LayerResult
}

// Layer returns the LayerResult for the named layer, and whether it was found.
func (r *GreetResult) Layer(name string) (LayerResult, bool) {
	for _, l := range r.Layers {
		if l.Name == name {
			return l, true
		}
	}
	return LayerResult{}, false
}
