package tcperror

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/crystade/greet"
)

// Classify maps OS / net errors to typed GreetErrors for a TCP connection attempt.
// protocolName is the name of the protocol (e.g. "tcp", "ssh", "postgresql", "minecraft").
func Classify(err error, protocolName, host string, port int) *greet.GreetError {
	base := &greet.GreetError{
		Protocol: protocolName,
		Cause:    err,
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			base.Code = greet.ErrConnectionTimeout
			base.Message = fmt.Sprintf("connection to %s:%d timed out", host, port)
			return base
		}
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return classifySyscallError(sysErr.Err, base, host, port)
		}
	}

	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		return classifySyscallError(sysErr, base, host, port)
	}

	base.Code = greet.ErrConnectionFailed
	base.Message = fmt.Sprintf("failed to connect to %s:%d: %v", host, port, err)
	return base
}

func classifySyscallError(err error, base *greet.GreetError, host string, port int) *greet.GreetError {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		base.Code = greet.ErrConnectionFailed
		base.Message = fmt.Sprintf("failed to connect to %s:%d: %v", host, port, err)
		return base
	}

	switch errno {
	case syscall.ECONNREFUSED:
		base.Code = greet.ErrConnectionRefused
		base.Message = fmt.Sprintf("connection to %s:%d refused", host, port)
	case syscall.ECONNRESET:
		base.Code = greet.ErrConnectionReset
		base.Message = fmt.Sprintf("connection to %s:%d reset", host, port)
	case syscall.ENETUNREACH:
		base.Code = greet.ErrNetworkUnreachable
		base.Message = fmt.Sprintf("network unreachable for %s:%d", host, port)
	default:
		base.Code = greet.ErrConnectionFailed
		base.Message = fmt.Sprintf("connection to %s:%d failed: %v", host, port, err)
	}
	return base
}
