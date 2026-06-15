package greet_test

import (
	"testing"

	"github.com/crystade/greet"
)

func TestParseTargetHostOnly(t *testing.T) {
	host, port, err := greet.ParseTarget("example.com", 80)
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com" || port != 80 {
		t.Errorf("ParseTarget(example.com, 80) = (%q, %d), want (%q, 80)", host, port, "example.com")
	}
}

func TestParseTargetHostPort(t *testing.T) {
	host, port, err := greet.ParseTarget("example.com:443", 80)
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com" || port != 443 {
		t.Errorf("ParseTarget(example.com:443, 80) = (%q, %d), want (%q, 443)", host, port, "example.com")
	}
}

func TestParseTargetIPv6(t *testing.T) {
	host, port, err := greet.ParseTarget("[::1]:22", 80)
	if err != nil {
		t.Fatal(err)
	}
	if host != "::1" || port != 22 {
		t.Errorf("ParseTarget([::1]:22, 80) = (%q, %d), want (::1, 22)", host, port)
	}
}

func TestParseTargetIPv6NoPort(t *testing.T) {
	host, port, err := greet.ParseTarget("[::1]", 80)
	if err != nil {
		t.Fatal(err)
	}
	if host != "::1" || port != 80 {
		t.Errorf("ParseTarget([::1], 80) = (%q, %d), want (::1, 80)", host, port)
	}
}

func TestParseTargetInvalidPort(t *testing.T) {
	_, _, err := greet.ParseTarget("example.com:abc", 80)
	if err == nil {
		t.Fatal("ParseTarget with invalid port should return error")
	}
}

func TestParseTargetBareIPv6(t *testing.T) {
	// Bare IPv6 without brackets should return a clear error,
	// not silently use the whole string as hostname.
	_, _, err := greet.ParseTarget("::1:80", 22)
	if err == nil {
		t.Fatal("ParseTarget with bare IPv6 should return error")
	}
}
