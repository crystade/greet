package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
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
	VersionString string `json:"version_string"` // e.g. "SSH-2.0-OpenSSH_8.9"
}

// SSH implements an SSH version banner probe.
// It connects, reads the server's version string, and closes without
// sending its own banner (anonymous probe).
type SSH struct{}

func (s *SSH) Name() string               { return ProtocolName }
func (s *SSH) Description() string        { return "SSH version banner exchange" }
func (s *SSH) DefaultPort() int           { return DefaultSSHPort }
func (s *SSH) Transport() greet.Transport { return greet.TransportTCP }

func (s *SSH) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	defer conn.Close()

	// Set read deadline from context
	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetReadDeadline(deadline)
	}

	// Limit read to 8KB to prevent unbounded allocation from
	// malicious servers. RFC 4253 §4.2 allows pre-auth banners before
	// the version string, so we loop until we find "SSH-".
	reader := bufio.NewReader(io.LimitReader(conn, 8192))
	var banner string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return nil, &greet.GreetError{
					Code:     ErrSSHBannerTimeout,
					Message:  fmt.Sprintf("timed out reading SSH banner from %s", addr),
					Protocol: ProtocolName,
					Cause:    err,
				}
			}
			return nil, &greet.GreetError{
				Code:     ErrSSHInvalidBanner,
				Message:  fmt.Sprintf("failed to read SSH banner from %s: %v", addr, err),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SSH-") {
			banner = line
			break
		}
		// Skip non-SSH pre-auth banner lines (RFC 4253 §4.2)
	}
	latency := time.Since(start)

	return greet.NewResult(ProtocolName, greet.TransportTCP, latency, true, &SSHResult{
		VersionString: banner,
	}), nil
}

func init() {
	greet.Register(&SSH{})
}
