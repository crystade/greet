package greet_test

import (
	"context"
	"testing"

	"github.com/crystade/greet"
	_ "github.com/crystade/greet/protocols/tcp"
)

// mockProtocol is a minimal Protocol for registry tests.
type mockProtocol struct {
	name string
}

func (m *mockProtocol) Name() string               { return m.name }
func (m *mockProtocol) Description() string        { return "mock" }
func (m *mockProtocol) DefaultPort() int           { return 9999 }
func (m *mockProtocol) Transport() greet.Transport { return greet.TransportTCP }
func (m *mockProtocol) Greet(_ context.Context, _ string, _ int, _ ...greet.GreetOption) (*greet.GreetResult, error) {
	return &greet.GreetResult{Protocol: m.name, Success: true}, nil
}

func TestRegistryGet(t *testing.T) {
	// TCP is registered via init() from the blank import
	p, err := greet.Get("tcp")
	if err != nil {
		t.Fatalf("Get(tcp) error: %v", err)
	}
	if p.Name() != "tcp" {
		t.Errorf("Get(tcp).Name() = %q, want %q", p.Name(), "tcp")
	}
}

func TestRegistryGetCaseInsensitive(t *testing.T) {
	p, err := greet.Get("TCP")
	if err != nil {
		t.Fatalf("Get(TCP) error: %v", err)
	}
	if p.Name() != "tcp" {
		t.Errorf("Get(TCP).Name() = %q, want %q", p.Name(), "tcp")
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	_, err := greet.Get("nonexistent")
	if err == nil {
		t.Fatal("Get(nonexistent) should return error")
	}
}

func TestRegistryList(t *testing.T) {
	list := greet.List()
	if len(list) == 0 {
		t.Fatal("List() returned empty; at least tcp should be registered")
	}

	// Verify sorted
	for i := 1; i < len(list); i++ {
		if list[i-1].Name() > list[i].Name() {
			t.Errorf("List() not sorted: %q > %q", list[i-1].Name(), list[i].Name())
		}
	}
}

func TestListProtocols(t *testing.T) {
	names := greet.ListProtocols()
	if len(names) == 0 {
		t.Fatal("ListProtocols() returned empty")
	}
	for _, n := range names {
		if n == "" {
			t.Error("ListProtocols() contains empty name")
		}
	}
}
