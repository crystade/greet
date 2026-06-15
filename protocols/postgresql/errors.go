package postgresql

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/tcperror"
)

const (
	ErrPostgreSQLStartupTimeout    greet.ErrorCode = "startup_timeout"
	ErrPostgreSQLSendFailed        greet.ErrorCode = "send_failed"
	ErrPostgreSQLMalformedResponse greet.ErrorCode = "malformed_response"
	ErrPostgreSQLSSLRejected       greet.ErrorCode = "ssl_rejected"
)

// classifyTCPError maps OS / net errors to typed GreetErrors for PostgreSQL.
func classifyTCPError(err error, host string, port int) *greet.GreetError {
	return tcperror.Classify(err, ProtocolName, host, port)
}
