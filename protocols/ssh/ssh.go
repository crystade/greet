package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the SSH protocol.
const ProtocolName = "ssh"

// DefaultSSHPort is the well-known port for SSH.
const DefaultSSHPort = 22

// SSHResult holds the outcome of an SSH banner exchange.
type SSHResult struct {
	VersionString string // e.g. "SSH-2.0-OpenSSH_8.9"
}

// SSH implements an SSH version banner probe.
// It connects, reads the server's version string, and closes without
// sending its own banner (anonymous probe).
type SSH struct{}

func (s *SSH) Name() string                   { return ProtocolName }
func (s *SSH) Description() string            { return "SSH version banner exchange" }
func (s *SSH) DefaultPort() int               { return DefaultSSHPort }
func (s *SSH) BaseTransport() greet.Transport { return greet.TransportTCP }

// Handshake reads the SSH version banner over an already-dialed conn.
// start is the shared timing anchor from the transport dial.
func (s *SSH) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, any, greet.LayerTiming, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Set read deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetReadDeadline(deadline)
	}

	// Read SSH banner. Limit to 8KB to prevent unbounded allocation from
	// malicious servers. RFC 4253 §4.2 allows pre-auth banners before
	// the version string, so we loop until we find "SSH-".
	reader := bufio.NewReader(io.LimitReader(conn, 8192))
	var banner string
	var ttfb time.Duration
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return nil, nil, greet.LayerTiming{}, &greet.GreetError{
					Code:     ErrSSHBannerTimeout,
					Message:  fmt.Sprintf("timed out reading SSH banner from %s", addr),
					Protocol: ProtocolName,
					Cause:    err,
				}
			}
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     ErrSSHInvalidBanner,
				Message:  fmt.Sprintf("failed to read SSH banner from %s: %v", addr, err),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SSH-") {
			banner = line
			ttfb = time.Since(start) // TTFB = first byte of SSH banner
			break
		}
		// Skip non-SSH pre-auth banner lines (RFC 4253 §4.2)
	}
	ttlb := time.Since(start) // TTLB = banner fully read

	return conn, &SSHResult{VersionString: banner}, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
}

func init() {
	greet.RegisterApp(&SSH{})
}
