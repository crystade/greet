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
func Greet(ctx context.Context, protocol, target string, opts ...GreetOption) (*GreetResult, error) {
	stack, err := Resolve(protocol)
	if err != nil {
		return nil, err
	}

	host, port, err := ParseTarget(target, stack.DefaultPort())
	if err != nil {
		return nil, err
	}

	return GreetStack(ctx, stack, host, port, opts...)
}

// GreetStack executes an already-resolved Stack against host:port.
// Use this when you have built a Stack directly via Resolve or ResolveStack.
//
// Execution:
//  1. Dial the transport layer (DNS + TCP/UDP + RTT).
//  2. For each app layer in order, run Handshake over the shared connection.
//  3. On first error: mark that layer failed, abort remaining layers,
//     return the partial GreetResult alongside the error.
//  4. Close the connection when done.
func GreetStack(ctx context.Context, stack *Stack, host string, port int, opts ...GreetOption) (*GreetResult, error) {
	cfg := ResolveOptions(opts...)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// start is the shared timing anchor for the whole stack. Every layer
	// measures TTDR/RTT/TTFB/TTLB as time.Since(start) so the aggregated
	// result illustrates the protocol stack from a single origin.
	start := time.Now()

	// Step 1: dial the transport layer
	conn, ttdr, rtt, err := stack.Transport.Dial(ctx, start, host, port)

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
		return result, err
	}
	defer conn.Close()

	// Bare transport (tcp/udp only) — no app layers to run.
	if len(stack.Apps) == 0 {
		result.Success = true
		return result, nil
	}

	// Step 2: run each app layer in order over the shared connection.
	// Each layer may upgrade conn (e.g. TLS wraps it); pass the upgraded conn
	// to the next layer. The engine's defer conn.Close() closes the original
	// transport conn — layers must not close it themselves.
	current := conn
	for _, app := range stack.Apps {
		next, data, timing, handshakeErr := app.Handshake(ctx, start, current, host, port, &cfg)

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
			return result, handshakeErr
		}
		if next != nil {
			current = next
		}
	}

	result.Success = true
	return result, nil
}

// ListProtocols returns all registered protocol names (transports + app layers), sorted.
func ListProtocols() []string {
	all := ListAll()
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
