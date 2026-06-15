package minecraft

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/tcperror"
)

const (
	ErrMinecraftHandshakeTimeout  greet.ErrorCode = "handshake_timeout"
	ErrMinecraftHandshakeFailed   greet.ErrorCode = "handshake_failed"
	ErrMinecraftMalformedResponse greet.ErrorCode = "malformed_response"
)

// classifyTCPError maps OS / net errors to typed GreetErrors for Minecraft.
func classifyTCPError(err error, host string, port int) *greet.GreetError {
	return tcperror.Classify(err, ProtocolName, host, port)
}
