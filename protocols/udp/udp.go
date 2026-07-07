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

// ProtocolName is the registered name for the UDP transport.
const ProtocolName = "udp"

// DefaultUDPPort is the well-known port for generic UDP probes.
const DefaultUDPPort = 53

// New creates a new UDP transport layer.
func New() *UDP {
	return &UDP{}
}

// UDP implements greet.TransportLayer for UDP probes.
// Because UDP is connectionless, Dial sends an empty datagram and waits for
// any response to capture RTT. The returned conn is the live UDPConn; since
// no app layers run over UDP today the engine closes it immediately after Dial.
type UDP struct{}

func (u *UDP) Name() string               { return ProtocolName }
func (u *UDP) DefaultPort() int           { return DefaultUDPPort }
func (u *UDP) Transport() greet.Transport { return greet.TransportUDP }

// Dial resolves host, sends an empty datagram, waits for a response, and
// returns the UDPConn together with TTDR and RTT. RTT is the send→receive
// round trip. The caller is responsible for closing the returned conn.
func (u *UDP) Dial(ctx context.Context, start time.Time, host string, port int) (net.Conn, time.Duration, time.Duration, error) {
	dns, err := greet.ResolveHost(ctx, host)
	if err != nil {
		return nil, 0, 0, err
	}
	ttdr := time.Since(start) // anchored to the shared start

	addr := net.JoinHostPort(dns.Address, strconv.Itoa(port))
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, ttdr, 0, &greet.GreetError{
			Code:     greet.ErrResolveHostFailed,
			Message:  fmt.Sprintf("failed to resolve UDP address %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, ttdr, 0, classifyUDPError(err, host, port)
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	_, err = conn.Write([]byte{})
	if err != nil {
		conn.Close()
		return nil, ttdr, 0, &greet.GreetError{
			Code:     greet.ErrSendFailed,
			Message:  fmt.Sprintf("failed to send UDP datagram to %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	buf := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buf)
	rtt := time.Since(start)

	if err != nil {
		conn.Close()
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, ttdr, rtt, &greet.GreetError{
				Code:     greet.ErrReceiveTimeout,
				Message:  fmt.Sprintf("UDP receive from %s:%d timed out", host, port),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		return nil, ttdr, rtt, classifyUDPError(err, host, port)
	}

	return conn, ttdr, rtt, nil
}
