package udp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the UDP protocol.
const ProtocolName = "udp"

// DefaultUDPPort is the well-known port for generic UDP probes.
const DefaultUDPPort = 53

// UDP implements a generic UDP send/receive probe.
// It sends an empty datagram and waits for any response or ICMP error.
type UDP struct{}

func (u *UDP) Name() string               { return ProtocolName }
func (u *UDP) Description() string        { return "Generic UDP send/receive (latency only)" }
func (u *UDP) DefaultPort() int           { return DefaultUDPPort }
func (u *UDP) Transport() greet.Transport { return greet.TransportUDP }

func (u *UDP) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, &greet.GreetError{
			Code:     greet.ErrResolveHostFailed,
			Message:  fmt.Sprintf("failed to resolve UDP address %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, classifyUDPError(err, host, port)
	}
	defer conn.Close()

	// Set deadline on the connection
	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetDeadline(deadline)
	}

	start := time.Now()
	_, err = conn.Write([]byte{})
	if err != nil {
		return nil, &greet.GreetError{
			Code:     greet.ErrSendFailed,
			Message:  fmt.Sprintf("failed to send UDP datagram to %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	buf := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buf)
	latency := time.Since(start)

	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, &greet.GreetError{
				Code:     greet.ErrReceiveTimeout,
				Message:  fmt.Sprintf("UDP receive from %s:%d timed out", host, port),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		return nil, classifyUDPError(err, host, port)
	}

	return greet.NewResult(ProtocolName, greet.TransportUDP, latency, true, nil), nil
}

func init() {
	greet.Register(&UDP{})
}
