package greet_test

import (
	"testing"

	"github.com/crystade/greet"
	_ "github.com/crystade/greet/protocols/tcp"
)

func TestRegistryResolve(t *testing.T) {
	// TCP is registered via init() from the blank import
	stack, err := greet.Resolve("tcp")
	if err != nil {
		t.Fatalf("Resolve(tcp) error: %v", err)
	}
	if stack.Name() != "tcp" {
		t.Errorf("Resolve(tcp).Name() = %q, want %q", stack.Name(), "tcp")
	}
}

func TestRegistryResolveCaseInsensitive(t *testing.T) {
	stack, err := greet.Resolve("TCP")
	if err != nil {
		t.Fatalf("Resolve(TCP) error: %v", err)
	}
	if stack.Name() != "tcp" {
		t.Errorf("Resolve(TCP).Name() = %q, want %q", stack.Name(), "tcp")
	}
}

func TestRegistryResolveUnknown(t *testing.T) {
	_, err := greet.Resolve("nonexistent")
	if err == nil {
		t.Fatal("Resolve(nonexistent) should return error")
	}
}

func TestRegistryListAll(t *testing.T) {
	list := greet.ListAll()
	if len(list) == 0 {
		t.Fatal("ListAll() returned empty; at least tcp should be registered")
	}

	// Verify sorted
	for i := 1; i < len(list); i++ {
		if list[i-1].Name > list[i].Name {
			t.Errorf("ListAll() not sorted: %q > %q", list[i-1].Name, list[i].Name)
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

func TestRegistryResolveStack(t *testing.T) {
	// Resolving a bare transport by explicit stack
	stack, err := greet.ResolveStack([]string{"tcp"})
	if err != nil {
		t.Fatalf("ResolveStack([tcp]) error: %v", err)
	}
	if stack.Name() != "tcp" {
		t.Errorf("ResolveStack([tcp]).Name() = %q, want %q", stack.Name(), "tcp")
	}
}

func TestRegistryResolveStackEmptyError(t *testing.T) {
	_, err := greet.ResolveStack([]string{})
	if err == nil {
		t.Fatal("ResolveStack([]) should return error")
	}
}

func TestRegistryResolveStackBadTransport(t *testing.T) {
	_, err := greet.ResolveStack([]string{"notransport"})
	if err == nil {
		t.Fatal("ResolveStack with unknown transport should return error")
	}
}
