package tls

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/tcperror"
)

const (
	ErrTLSHandshakeFailed greet.ErrorCode = "tls_handshake_failed"
)

// classifyTCPError maps OS / net errors to typed GreetErrors for TLS.
func classifyTCPError(err error, host string, port int) *greet.GreetError {
	return tcperror.Classify(err, ProtocolName, host, port)
}
