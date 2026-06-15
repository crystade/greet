package udp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/crystade/greet"
)

// classifyUDPError maps OS / net errors to typed GreetErrors.
func classifyUDPError(err error, host string, port int) *greet.GreetError {
	base := &greet.GreetError{
		Protocol: ProtocolName,
		Cause:    err,
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			base.Code = greet.ErrReceiveTimeout
			base.Message = fmt.Sprintf("UDP receive from %s:%d timed out", host, port)
			return base
		}
		if sysErr, ok := opErr.Err.(*os.SyscallError); ok {
			return classifySyscallError(sysErr.Err, base, host, port)
		}
	}

	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		return classifySyscallError(sysErr, base, host, port)
	}

	base.Code = greet.ErrSendFailed
	base.Message = fmt.Sprintf("UDP operation on %s:%d failed: %v", host, port, err)
	return base
}

func classifySyscallError(err error, base *greet.GreetError, host string, port int) *greet.GreetError {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		base.Code = greet.ErrSendFailed
		base.Message = fmt.Sprintf("UDP operation on %s:%d failed: %v", host, port, err)
		return base
	}

	switch errno {
	case syscall.ECONNREFUSED:
		base.Code = greet.ErrPortUnreachable
		base.Message = fmt.Sprintf("port %d on %s unreachable (ICMP)", port, host)
	case syscall.ENETUNREACH:
		base.Code = greet.ErrNetworkUnreachable
		base.Message = fmt.Sprintf("network unreachable for %s:%d", host, port)
	default:
		base.Code = greet.ErrSendFailed
		base.Message = fmt.Sprintf("UDP operation on %s:%d failed: %v", host, port, err)
	}
	return base
}
