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

	// Short-circuit if host is already an IP address
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

// Greet is the one-stop entry point. It looks up the protocol by name,
// resolves the target (host or host:port), and performs the handshake.
func Greet(ctx context.Context, protocol, target string, opts ...GreetOption) (*GreetResult, error) {
	p, err := Get(protocol)
	if err != nil {
		return nil, err
	}

	host, port, err := ParseTarget(target, p.DefaultPort())
	if err != nil {
		return nil, err
	}

	return GreetWith(ctx, p, host, port, opts...)
}

// GreetWith is like Greet but accepts a pre-resolved Protocol instance.
func GreetWith(ctx context.Context, p Protocol, host string, port int, opts ...GreetOption) (*GreetResult, error) {
	return p.Greet(ctx, host, port, opts...)
}

// ListProtocols returns all registered protocol names, sorted.
func ListProtocols() []string {
	protocols := List()
	names := make([]string, len(protocols))
	for i, p := range protocols {
		names[i] = p.Name()
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
