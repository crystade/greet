package tcp

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/tcperror"
)

// classifyTCPError maps OS / net errors to typed GreetErrors.
func classifyTCPError(err error, host string, port int) *greet.GreetError {
	return tcperror.Classify(err, ProtocolName, host, port)
}
