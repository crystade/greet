package redis

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

// ProtocolName is the registered name for the Redis protocol.
const ProtocolName = "redis"

// DefaultRedisPort is the well-known port for Redis.
const DefaultRedisPort = 6379

// HelloMode selects which RESP version a HELLO probe requests.
type HelloMode string

const (
	// HelloAuto sends HELLO 3 and falls back to HELLO 2 when the server
	// rejects it (unknown command or unsupported protocol version).
	HelloAuto HelloMode = "auto"
	// HelloRESP2 sends HELLO 2 only.
	HelloRESP2 HelloMode = "resp2"
	// HelloRESP3 sends HELLO 3 only, with no fallback.
	HelloRESP3 HelloMode = "resp3"
)

// ParseHelloMode normalizes and validates a hello mode string
// (case-insensitive). An empty string means HelloAuto.
func ParseHelloMode(s string) (HelloMode, error) {
	switch m := HelloMode(strings.ToLower(strings.TrimSpace(s))); m {
	case "", HelloAuto:
		return HelloAuto, nil
	case HelloRESP2, HelloRESP3:
		return m, nil
	default:
		return "", fmt.Errorf("invalid hello mode %q: expected auto, resp2 or resp3", s)
	}
}

// RedisConfig holds protocol-specific configuration.
type RedisConfig struct {
	// UseHello sends HELLO instead of PING (returns server info).
	// Default (false) sends classic PING for maximum compatibility.
	UseHello bool

	// HelloMode selects the RESP version requested by HELLO:
	// "auto" (default), "resp2" or "resp3". An empty value means "auto".
	// Setting a non-empty HelloMode implies UseHello.
	HelloMode HelloMode
}

func (c *RedisConfig) LayerConfigName() string { return ProtocolName }

// RedisResult holds the outcome of a Redis probe.
type RedisResult struct {
	Pong bool              // true when PING succeeded
	Info map[string]string // all key-value pairs returned by HELLO (nil on PING); Info["proto"] is the negotiated RESP version
}

func (r *RedisResult) LayerDataName() string { return ProtocolName }

// serverError is an error reply (-ERR ...) sent by the Redis server, as
// opposed to an I/O or parse failure.
type serverError struct{ msg string }

func (e *serverError) Error() string { return "server error: " + e.msg }

// Redis implements a Redis PING/HELLO probe.
// It is stateless — configuration is passed via GreetOption.
type Redis struct{}

// New creates a new Redis application layer.
func New() *Redis {
	return &Redis{}
}

func (r *Redis) Name() string                   { return ProtocolName }
func (r *Redis) Description() string            { return "Redis PING/HELLO probe" }
func (r *Redis) DefaultPort() int               { return DefaultRedisPort }
func (r *Redis) BaseTransport() greet.Transport { return greet.TransportTCP }

const cmdPing = "*1\r\n$4\r\nPING\r\n"

// helloCmd builds a HELLO command for the given RESP version (2 or 3).
func helloCmd(version int) string {
	return fmt.Sprintf("*2\r\n$5\r\nHELLO\r\n$1\r\n%d\r\n", version)
}

// Handshake performs a Redis PING or HELLO probe over an already-dialed conn.
func (r *Redis) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, greet.LayerData, greet.LayerTiming, error) {
	useHello := false
	mode := HelloAuto
	if raw := cfg.LayerConfig(ProtocolName); raw != nil {
		redisCfg, ok := raw.(*RedisConfig)
		if !ok {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: expected *RedisConfig, got %T", raw),
				Protocol: ProtocolName,
			}
		}
		m, err := ParseHelloMode(string(redisCfg.HelloMode))
		if err != nil {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: %v", err),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		mode = m
		useHello = redisCfg.UseHello || redisCfg.HelloMode != ""
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	addr := conn.RemoteAddr().String()

	// Pick the first command to send.
	cmd := cmdPing
	if useHello {
		if mode == HelloRESP2 {
			cmd = helloCmd(2)
		} else {
			// auto and resp3 both start with HELLO 3.
			cmd = helloCmd(3)
		}
	}

	if err := sendCommand(conn, addr, cmd); err != nil {
		return nil, nil, greet.LayerTiming{}, err
	}

	reader := bufio.NewReader(io.LimitReader(conn, 8192))
	ttfb := time.Since(start)

	result := &RedisResult{}
	var err error

	if useHello {
		err = parseHello(reader, result)

		// Auto mode: the server rejected HELLO 3, retry once with HELLO 2.
		// Error replies leave the connection open, so reuse it.
		if err != nil && mode == HelloAuto && shouldFallBack(err) {
			result = &RedisResult{}
			if serr := sendCommand(conn, addr, helloCmd(2)); serr != nil {
				return nil, nil, greet.LayerTiming{}, serr
			}
			err = parseHello(reader, result)
		}
	} else {
		err = parsePing(reader, result)
	}

	ttlb := time.Since(start)

	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     ErrRedisTimeout,
				Message:  fmt.Sprintf("timed out reading Redis response from %s", addr),
				Protocol: ProtocolName,
				Cause:    err,
			}
		}
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrRedisUnexpectedResponse,
			Message:  fmt.Sprintf("unexpected Redis response from %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
}

// sendCommand writes a raw command and maps failures to a *greet.GreetError.
func sendCommand(conn net.Conn, addr, cmd string) error {
	if _, err := conn.Write([]byte(cmd)); err != nil {
		code := ErrRedisSendFailed
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			code = ErrRedisTimeout
		}
		return &greet.GreetError{
			Code:     code,
			Message:  fmt.Sprintf("failed to send Redis command to %s: %v", addr, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	return nil
}

// shouldFallBack reports whether a failed HELLO 3 should be retried as
// HELLO 2: the server answered with an unknown-command (-ERR) or
// unsupported-version (-NOPROTO) error. Other server errors such as -NOAUTH
// would fail identically on retry, so they are not retried.
func shouldFallBack(err error) bool {
	var se *serverError
	if !errors.As(err, &se) {
		return false
	}
	return strings.HasPrefix(se.msg, "ERR") || strings.HasPrefix(se.msg, "NOPROTO")
}

func parsePing(r *bufio.Reader, result *RedisResult) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "+PONG" {
		result.Pong = true
		return nil
	}
	return fmt.Errorf("expected +PONG, got %q", line)
}

// parseHello handles both RESP2 array and RESP3 map replies from HELLO.
func parseHello(r *bufio.Reader, result *RedisResult) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}

	switch b {
	case '*': // RESP2 array (reply to HELLO 2)
		return parseHelloArray(r, result)
	case '%': // RESP3 map (reply to HELLO 3)
		return parseHelloMap(r, result)
	case '-': // error
		line, _ := r.ReadString('\n')
		return &serverError{msg: strings.TrimSpace(line)}
	default:
		return fmt.Errorf("unexpected reply type %q", b)
	}
}

func parseHelloArray(r *bufio.Reader, result *RedisResult) error {
	lenStr, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(lenStr))
	if err != nil || n < 2 || n%2 != 0 {
		return fmt.Errorf("invalid HELLO array length: %q", lenStr)
	}

	result.Info = make(map[string]string, n/2)

	for i := 0; i < n; i += 2 {
		key, err := readRESPValue(r)
		if err != nil {
			return err
		}
		val, err := readRESPValue(r)
		if err != nil {
			return err
		}
		result.Info[key] = val
	}
	return nil
}

func parseHelloMap(r *bufio.Reader, result *RedisResult) error {
	lenStr, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(lenStr))
	if err != nil || n < 1 {
		return fmt.Errorf("invalid HELLO map length: %q", lenStr)
	}

	result.Info = make(map[string]string, n)

	for i := 0; i < n; i++ {
		key, err := readRESPValue(r)
		if err != nil {
			return err
		}
		val, err := readRESPValue(r)
		if err != nil {
			return err
		}
		result.Info[key] = val
	}
	return nil
}

// readRESPValue reads a single RESP value and returns it as a string.
// Supports simple strings, bulk strings, integers, and arrays (joined by comma).
func readRESPValue(r *bufio.Reader) (string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}

	switch b {
	case '+': // simple string
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil

	case ':': // integer
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil

	case '$': // bulk string
		lenStr, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		n, err := strconv.Atoi(strings.TrimSpace(lenStr))
		if err != nil || n < 0 {
			return "", fmt.Errorf("invalid bulk length: %q", lenStr)
		}
		buf := make([]byte, n+2) // +2 for \r\n
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil

	case '*': // array (e.g. modules)
		lenStr, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		n, err := strconv.Atoi(strings.TrimSpace(lenStr))
		if err != nil || n < 0 {
			return "", fmt.Errorf("invalid array length: %q", lenStr)
		}
		parts := make([]string, 0, n)
		for i := 0; i < n; i++ {
			v, err := readRESPValue(r)
			if err != nil {
				return "", err
			}
			parts = append(parts, v)
		}
		return strings.Join(parts, ","), nil

	case '-': // error
		line, _ := r.ReadString('\n')
		return "", &serverError{msg: strings.TrimSpace(line)}

	default:
		return "", fmt.Errorf("unsupported RESP type %q", b)
	}
}
