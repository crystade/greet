package ssh

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/tcperror"
)

const (
	ErrSSHBannerTimeout greet.ErrorCode = "banner_timeout"
	ErrSSHInvalidBanner greet.ErrorCode = "invalid_banner"
)

// classifyTCPError maps OS / net errors to typed GreetErrors for SSH.
func classifyTCPError(err error, host string, port int) *greet.GreetError {
	return tcperror.Classify(err, ProtocolName, host, port)
}
