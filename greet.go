package greet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// DNSResult holds the outcome of DNS resolution.
type DNSResult struct {
	Address string        // resolved IP address
	TTDR    time.Duration // time to DNS resolved
}

// ResolveHost resolves a hostname to an IP address and measures TTDR.
// If host is already an IP address, it returns immediately with TTDR ≈ 0.
func ResolveHost(ctx context.Context, host string) (*DNSResult, error) {
	start := time.Now()

	if ip := net.ParseIP(host); ip != nil {
		return &DNSResult{Address: host, TTDR: time.Since(start)}, nil
	}

	resolver := &net.Resolver{}
	addrs, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return nil, &GreetError{
			Code:    ErrResolveHostFailed,
			Message: fmt.Sprintf("failed to resolve host %q: %v", host, err),
			Cause:   err,
		}
	}
	if len(addrs) == 0 {
		return nil, &GreetError{
			Code:    ErrResolveHostFailed,
			Message: fmt.Sprintf("no addresses found for host %q", host),
		}
	}

	return &DNSResult{Address: addrs[0], TTDR: time.Since(start)}, nil
}

// Greet resolves the named protocol to its layer stack, parses the target,
// and executes the layered handshake engine. It is the primary entry point.
//
// protocol is a registered name such as "tcp", "tls", "minecraft", "ssh",
// "postgresql", or "udp". The name implies its full stack:
//   - "tls"        → [tcp, tls]
//   - "minecraft"  → [tcp, minecraft]
//   - "tcp"        → [tcp]
//   - "udp"        → [udp]
func (e *Engine) Greet(ctx context.Context, protocol, target string, opts ...GreetOption) (*GreetResult, error) {
	stack, err := e.Resolve(protocol)
	if err != nil {
		return nil, err
	}

	host, port, err := ParseTarget(target, stack.DefaultPort())
	if err != nil {
		return nil, err
	}

	return e.GreetStack(ctx, stack, host, port, opts...), nil
}

// GreetStack executes an already-resolved Stack against host:port.
// Use this when you have built a Stack directly via Resolve or ResolveStack.
//
// Execution:
//  1. Dial the transport layer (DNS + TCP/UDP + RTT).
//  2. For each app layer in order, run Handshake over the shared connection.
//  3. On first error: mark that layer failed, abort remaining layers,
//     set result.Error, and return.
//  4. Close the connection when done.
func (e *Engine) GreetStack(ctx context.Context, stack *Stack, host string, port int, opts ...GreetOption) *GreetResult {
	cfg := ResolveOptions(opts...)

	// NO context.WithTimeout wrapping — caller controls global deadline
	// start is the shared timing anchor for the whole stack. Every layer
	// measures TTDR/RTT/TTFB/TTLB as time.Since(start) so the aggregated
	// result illustrates the protocol stack from a single origin.
	start := time.Now()

	// Step 1: dial the transport layer — apply per-layer timeout if configured
	dialCtx := ctx
	var dialCancel context.CancelFunc
	if lt := cfg.LayerTimeout(stack.Transport.Name()); lt > 0 {
		dialCtx, dialCancel = context.WithTimeout(ctx, lt)
	}
	conn, ttdr, rtt, err := stack.Transport.Dial(dialCtx, start, host, port)
	if dialCancel != nil {
		dialCancel()
	}

	result := &GreetResult{
		Protocol:  stack.Name(),
		Transport: stack.Transport.Transport(),
		TTDR:      ttdr,
		RTT:       rtt,
	}

	// The transport layer is the first entry in Layers.
	// For a transport, TTFB = TTLB = RTT (no application data exchanged).
	result.Layers = append(result.Layers, LayerResult{
		Name:    stack.Transport.Name(),
		TTFB:    rtt,
		TTLB:    rtt,
		Success: err == nil,
	})

	if err != nil {
		result.Success = false
		result.Error = err
		return result
	}
	defer conn.Close()

	// Bare transport (tcp/udp only) — no app layers to run.
	if len(stack.Apps) == 0 {
		result.Success = true
		return result
	}

	// Step 2: run each app layer in order over the shared connection.
	// Each layer may upgrade conn (e.g. TLS wraps it); pass the upgraded conn
	// to the next layer. The engine's defer conn.Close() closes the original
	// transport conn — layers must not close it themselves.
	current := conn
	for _, app := range stack.Apps {
		layerCtx := ctx
		var cancel context.CancelFunc
		if lt := cfg.LayerTimeout(app.Name()); lt > 0 {
			layerCtx, cancel = context.WithTimeout(ctx, lt)
		}

		next, data, timing, handshakeErr := app.Handshake(layerCtx, start, current, host, port, &cfg)

		if cancel != nil {
			cancel()
		}

		lr := LayerResult{
			Name:    app.Name(),
			TTFB:    timing.TTFB,
			TTLB:    timing.TTLB,
			Success: handshakeErr == nil,
			Data:    data,
		}
		result.Layers = append(result.Layers, lr)

		if handshakeErr != nil {
			result.Success = false
			result.Error = handshakeErr
			return result
		}
		if next != nil {
			current = next
		}
	}

	result.Success = true
	return result
}

// ListProtocols returns all registered protocol names (transports + app layers), sorted.
func (e *Engine) ListProtocols() []string {
	all := e.ListAll()
	names := make([]string, len(all))
	for i, l := range all {
		names[i] = l.Name
	}
	return names
}

// ParseTarget splits a target string into host and port.
// Accepted formats: "host", "host:port", "[::1]:port".
// If no port is specified, defaultPort is used.
// Port is validated to be in the range 1–65535.
func ParseTarget(target string, defaultPort int) (string, int, error) {
	if strings.Contains(target, "[") {
		// IPv6 bracket notation: [::1]:port or [::1]
		host, portStr, err := net.SplitHostPort(target)
		if err != nil {
			// Try without port — maybe it's just [::1]
			host = strings.Trim(target, "[]")
			if defaultPort < 1 || defaultPort > 65535 {
				return "", 0, &GreetError{
					Code:    ErrInvalidAddress,
					Message: fmt.Sprintf("invalid default port %d", defaultPort),
				}
			}
			return host, defaultPort, nil
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, &GreetError{
				Code:    ErrInvalidAddress,
				Message: fmt.Sprintf("invalid port %q in target %q", portStr, target),
				Cause:   err,
			}
		}
		if port < 1 || port > 65535 {
			return "", 0, &GreetError{
				Code:    ErrInvalidAddress,
				Message: fmt.Sprintf("port %d out of range (1-65535) in target %q", port, target),
			}
		}
		return host, port, nil
	}

	// IPv4 or hostname — try host:port split
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		// Distinguish "missing port" (valid, use default) from other errors
		// like "too many colons" (invalid input, e.g. bare IPv6).
		addrErr := &net.AddrError{}
		if errors.As(err, &addrErr) && addrErr.Err == "too many colons in address" {
			return "", 0, &GreetError{
				Code:    ErrInvalidAddress,
				Message: fmt.Sprintf("invalid target %q: use [host]:port for IPv6 addresses", target),
				Cause:   err,
			}
		}
		// No port specified — use default
		if defaultPort < 1 || defaultPort > 65535 {
			return "", 0, &GreetError{
				Code:    ErrInvalidAddress,
				Message: fmt.Sprintf("invalid default port %d", defaultPort),
			}
		}
		return target, defaultPort, nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, &GreetError{
			Code:    ErrInvalidAddress,
			Message: fmt.Sprintf("invalid port %q in target %q", portStr, target),
			Cause:   err,
		}
	}
	if port < 1 || port > 65535 {
		return "", 0, &GreetError{
			Code:    ErrInvalidAddress,
			Message: fmt.Sprintf("port %d out of range (1-65535) in target %q", port, target),
		}
	}
	return host, port, nil
}
