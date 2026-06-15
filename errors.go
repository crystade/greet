package greet

import "fmt"

// ErrorCode is a human-readable error category string.
// Code always compares against named constants; the string value is
// immediately readable in logs, JSON output, and debug prints.
type ErrorCode string

// String returns the human-readable error code.
func (c ErrorCode) String() string { return string(c) }

// GreetError is the structured error type returned by all protocol
// implementations. It carries a machine-readable code, a human message,
// the originating protocol name, and the underlying OS / network error.
type GreetError struct {
	Code     ErrorCode
	Message  string
	Protocol string // protocol name; empty for common errors
	Cause    error  // underlying OS or network error
}

// Error implements the error interface.
func (e *GreetError) Error() string {
	if e.Protocol != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Protocol, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause, enabling errors.Is / errors.As chains.
func (e *GreetError) Unwrap() error { return e.Cause }

// Common error codes shared by every protocol.
const (
	ErrResolveHostFailed ErrorCode = "resolve_host_failed"
	ErrInvalidAddress    ErrorCode = "invalid_address"
	ErrProtocolMismatch  ErrorCode = "protocol_mismatch"
	ErrUnknownProtocol   ErrorCode = "unknown_protocol"
	ErrInvalidConfig     ErrorCode = "invalid_config"
)
