package greet_test

import (
	"context"
	"testing"
	"time"

	"github.com/crystade/greet"
)

func TestTransportString(t *testing.T) {
	tests := []struct {
		input greet.Transport
		want  string
	}{
		{greet.TransportTCP, "tcp"},
		{greet.TransportUDP, "udp"},
	}
	for _, tt := range tests {
		if got := tt.input.String(); got != tt.want {
			t.Errorf("Transport.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestErrorCodeString(t *testing.T) {
	code := greet.ErrConnectionRefused
	if got := code.String(); got != "connection_refused" {
		t.Errorf("ErrorCode.String() = %q, want %q", got, "connection_refused")
	}
}

func TestGreetError(t *testing.T) {
	cause := context.DeadlineExceeded
	ge := &greet.GreetError{
		Code:     greet.ErrConnectionTimeout,
		Message:  "connection timed out",
		Protocol: "tcp",
		Cause:    cause,
	}

	errStr := ge.Error()
	if errStr == "" {
		t.Fatal("GreetError.Error() returned empty string")
	}
	if ge.Unwrap() != cause {
		t.Errorf("GreetError.Unwrap() = %v, want %v", ge.Unwrap(), cause)
	}

	// Test without protocol
	ge2 := &greet.GreetError{
		Code:    greet.ErrInvalidAddress,
		Message: "bad address",
	}
	if errStr2 := ge2.Error(); errStr2 == "" {
		t.Fatal("GreetError.Error() without protocol returned empty")
	}
}

func TestGreetOptionTimeout(t *testing.T) {
	customTimeout := 10 * time.Second
	opt := greet.WithTimeout(customTimeout)

	cfg := &greet.GreetConfig{}
	opt(cfg)

	if cfg.Timeout != customTimeout {
		t.Errorf("WithTimeout: got %v, want %v", cfg.Timeout, customTimeout)
	}
}

func TestDefaultTimeout(t *testing.T) {
	if greet.DefaultTimeout != 5*time.Second {
		t.Errorf("DefaultTimeout = %v, want %v", greet.DefaultTimeout, 5*time.Second)
	}
}

func TestResolveOptions(t *testing.T) {
	// No options — should use defaults
	cfg := greet.ResolveOptions()
	if cfg.Timeout != greet.DefaultTimeout {
		t.Errorf("ResolveOptions(): Timeout = %v, want %v", cfg.Timeout, greet.DefaultTimeout)
	}

	// Custom timeout
	custom := 3 * time.Second
	cfg = greet.ResolveOptions(greet.WithTimeout(custom))
	if cfg.Timeout != custom {
		t.Errorf("ResolveOptions(WithTimeout): Timeout = %v, want %v", cfg.Timeout, custom)
	}
}
