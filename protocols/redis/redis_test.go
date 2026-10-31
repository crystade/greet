package redis

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDefaultRedisPort(t *testing.T) {
	if DefaultRedisPort != 6379 {
		t.Errorf("DefaultRedisPort = %d, want 6379", DefaultRedisPort)
	}
}

func TestParsePingSuccess(t *testing.T) {
	result := &RedisResult{}
	err := parsePing(bufio.NewReader(strings.NewReader("+PONG\r\n")), result)
	if err != nil {
		t.Fatalf("parsePing returned error: %v", err)
	}
	if !result.Pong {
		t.Error("expected Pong = true")
	}
}

func TestParsePingUnexpected(t *testing.T) {
	result := &RedisResult{}
	err := parsePing(bufio.NewReader(strings.NewReader("+OK\r\n")), result)
	if err == nil {
		t.Fatal("expected error for non-PONG response")
	}
	if result.Pong {
		t.Error("Pong should remain false on error")
	}
}

func TestParseHelloRESP2(t *testing.T) {
	// Minimal RESP2 HELLO reply
	raw := "*8\r\n" +
		"$6\r\nserver\r\n$5\r\nredis\r\n" +
		"$7\r\nversion\r\n$5\r\n7.2.4\r\n" +
		"$5\r\nproto\r\n:2\r\n" +
		"$4\r\nrole\r\n$6\r\nmaster\r\n"

	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader(raw)), result)
	if err != nil {
		t.Fatalf("parseHello returned error: %v", err)
	}

	if result.Info["server"] != "redis" {
		t.Errorf("server = %q, want redis", result.Info["server"])
	}
	if result.Info["version"] != "7.2.4" {
		t.Errorf("version = %q, want 7.2.4", result.Info["version"])
	}
	if result.Info["proto"] != "2" {
		t.Errorf("proto = %q, want 2", result.Info["proto"])
	}
	if result.Info["role"] != "master" {
		t.Errorf("role = %q, want master", result.Info["role"])
	}
}

func TestParseHelloRESP3(t *testing.T) {
	// Minimal RESP3 HELLO map reply
	raw := "%4\r\n" +
		"$6\r\nserver\r\n$5\r\nredis\r\n" +
		"$7\r\nversion\r\n$5\r\n7.2.4\r\n" +
		"$5\r\nproto\r\n:3\r\n" +
		"$4\r\nmode\r\n$10\r\nstandalone\r\n"

	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader(raw)), result)
	if err != nil {
		t.Fatalf("parseHello returned error: %v", err)
	}

	if result.Info["server"] != "redis" {
		t.Errorf("server = %q, want redis", result.Info["server"])
	}
	if result.Info["version"] != "7.2.4" {
		t.Errorf("version = %q, want 7.2.4", result.Info["version"])
	}
	if result.Info["proto"] != "3" {
		t.Errorf("proto = %q, want 3", result.Info["proto"])
	}
	if result.Info["mode"] != "standalone" {
		t.Errorf("mode = %q, want standalone", result.Info["mode"])
	}
}

func TestParseHelloError(t *testing.T) {
	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader("-ERR unknown command\r\n")), result)
	if err == nil {
		t.Fatal("expected error for server error reply")
	}
}

func TestParseHelloErrorIsServerError(t *testing.T) {
	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader("-ERR unknown command\r\n")), result)

	var se *serverError
	if !errors.As(err, &se) {
		t.Fatalf("error is %T, want *serverError", err)
	}
	if se.msg != "ERR unknown command" {
		t.Errorf("msg = %q, want %q", se.msg, "ERR unknown command")
	}
}

func TestParseHelloUnexpectedType(t *testing.T) {
	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader("+OK\r\n")), result)
	if err == nil {
		t.Fatal("expected error for unexpected reply type")
	}
	var se *serverError
	if errors.As(err, &se) {
		t.Error("non-error reply should not be a *serverError")
	}
}

func TestParseHelloTruncated(t *testing.T) {
	// Map declares 2 pairs but the reply ends after the first key.
	raw := "%2\r\n$6\r\nserver\r\n"

	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader(raw)), result)
	if err == nil {
		t.Fatal("expected error for truncated reply")
	}
}

func TestParseHelloInvalidLength(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"array odd length", "*3\r\n"},
		{"array empty", "*0\r\n"},
		{"array non-numeric", "*x\r\n"},
		{"map empty", "%0\r\n"},
		{"map non-numeric", "%x\r\n"},
	}
	for _, tt := range tests {
		result := &RedisResult{}
		err := parseHello(bufio.NewReader(strings.NewReader(tt.raw)), result)
		if err == nil {
			t.Errorf("%s: expected error", tt.name)
		}
	}
}

func TestParseHelloModulesArray(t *testing.T) {
	// HELLO replies carry modules as a nested array; it is joined by comma.
	raw := "%2\r\n" +
		"$5\r\nproto\r\n:3\r\n" +
		"$7\r\nmodules\r\n*2\r\n$6\r\nsearch\r\n$4\r\njson\r\n"

	result := &RedisResult{}
	err := parseHello(bufio.NewReader(strings.NewReader(raw)), result)
	if err != nil {
		t.Fatalf("parseHello returned error: %v", err)
	}
	if result.Info["modules"] != "search,json" {
		t.Errorf("modules = %q, want search,json", result.Info["modules"])
	}
}

func TestReadRESPValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"simple string", "+OK\r\n", "OK"},
		{"integer", ":42\r\n", "42"},
		{"bulk string", "$5\r\nhello\r\n", "hello"},
		{"empty bulk string", "$0\r\n\r\n", ""},
		{"array", "*2\r\n$1\r\na\r\n:2\r\n", "a,2"},
		{"empty array", "*0\r\n", ""},
	}
	for _, tt := range tests {
		got, err := readRESPValue(bufio.NewReader(strings.NewReader(tt.raw)))
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestReadRESPValueErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty input", ""},
		{"null bulk string", "$-1\r\n"},
		{"bulk non-numeric length", "$x\r\n"},
		{"bulk truncated", "$5\r\nhi"},
		{"array negative length", "*-1\r\n"},
		{"unsupported type", "!oops\r\n"},
	}
	for _, tt := range tests {
		if got, err := readRESPValue(bufio.NewReader(strings.NewReader(tt.raw))); err == nil {
			t.Errorf("%s: expected error, got %q", tt.name, got)
		}
	}
}

func TestReadRESPValueServerError(t *testing.T) {
	_, err := readRESPValue(bufio.NewReader(strings.NewReader("-ERR boom\r\n")))

	var se *serverError
	if !errors.As(err, &se) {
		t.Fatalf("error is %T, want *serverError", err)
	}
	if se.msg != "ERR boom" {
		t.Errorf("msg = %q, want %q", se.msg, "ERR boom")
	}
}

func TestServerErrorMessage(t *testing.T) {
	err := &serverError{msg: "ERR unknown command"}
	if got, want := err.Error(), "server error: ERR unknown command"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestRedisConfigLayerName(t *testing.T) {
	cfg := &RedisConfig{}
	if cfg.LayerConfigName() != ProtocolName {
		t.Errorf("LayerConfigName() = %q, want %q", cfg.LayerConfigName(), ProtocolName)
	}
}

func TestRedisResultLayerDataName(t *testing.T) {
	result := &RedisResult{}
	if result.LayerDataName() != ProtocolName {
		t.Errorf("LayerDataName() = %q, want %q", result.LayerDataName(), ProtocolName)
	}
}

func TestParseHelloModeValid(t *testing.T) {
	tests := []struct {
		in   string
		want HelloMode
	}{
		{"", HelloAuto},
		{"auto", HelloAuto},
		{"AUTO", HelloAuto},
		{"resp2", HelloRESP2},
		{"  RESP2 ", HelloRESP2},
		{"resp3", HelloRESP3},
		{"Resp3", HelloRESP3},
	}
	for _, tt := range tests {
		got, err := ParseHelloMode(tt.in)
		if err != nil {
			t.Errorf("ParseHelloMode(%q) returned error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseHelloMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseHelloModeInvalid(t *testing.T) {
	for _, in := range []string{"resp4", "2", "3", "hello", "resp"} {
		if got, err := ParseHelloMode(in); err == nil {
			t.Errorf("ParseHelloMode(%q) = %q, want error", in, got)
		}
	}
}

func TestHelloCmd(t *testing.T) {
	if got, want := helloCmd(2), "*2\r\n$5\r\nHELLO\r\n$1\r\n2\r\n"; got != want {
		t.Errorf("helloCmd(2) = %q, want %q", got, want)
	}
	if got, want := helloCmd(3), "*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"; got != want {
		t.Errorf("helloCmd(3) = %q, want %q", got, want)
	}
}

func TestShouldFallBack(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unknown command", &serverError{msg: "ERR unknown command 'HELLO'"}, true},
		{"unsupported protocol", &serverError{msg: "NOPROTO sorry, this protocol version is not supported"}, true},
		{"wrapped server error", fmt.Errorf("wrapped: %w", &serverError{msg: "ERR unknown command"}), true},
		{"auth required", &serverError{msg: "NOAUTH Authentication required."}, false},
		{"non-server error", errors.New("unexpected reply type '!'"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		if got := shouldFallBack(tt.err); got != tt.want {
			t.Errorf("%s: shouldFallBack = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestShouldFallBackOnParsedReplies(t *testing.T) {
	// End to end through parseHello: which real server replies trigger the
	// HELLO 3 -> HELLO 2 fallback in auto mode.
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"redis < 6", "-ERR unknown command 'HELLO'\r\n", true},
		{"unsupported version", "-NOPROTO sorry, this protocol version is not supported\r\n", true},
		{"auth required", "-NOAUTH HELLO must be called with the client already authenticated\r\n", false},
		{"success RESP3", "%1\r\n$5\r\nproto\r\n:3\r\n", false},
		{"unexpected type", "+OK\r\n", false},
	}
	for _, tt := range tests {
		err := parseHello(bufio.NewReader(strings.NewReader(tt.raw)), &RedisResult{})
		if got := shouldFallBack(err); got != tt.want {
			t.Errorf("%s: shouldFallBack = %v, want %v (err = %v)", tt.name, got, tt.want, err)
		}
	}
}
