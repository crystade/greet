package greet

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Stack is the resolved execution plan for a named protocol: the transport
// layer to dial, followed by zero or more application layers to run in order.
type Stack struct {
	Transport TransportLayer
	Apps      []AppLayer
}

// Name returns the name of the outermost (last) layer in the stack.
// For a bare transport (e.g. "tcp") this is the transport name.
func (s *Stack) Name() string {
	if len(s.Apps) > 0 {
		return s.Apps[len(s.Apps)-1].Name()
	}
	return s.Transport.Name()
}

// DefaultPort returns the default port of the outermost layer.
func (s *Stack) DefaultPort() int {
	if len(s.Apps) > 0 {
		return s.Apps[len(s.Apps)-1].DefaultPort()
	}
	return s.Transport.DefaultPort()
}

var (
	mu                sync.RWMutex
	transportRegistry = map[string]TransportLayer{}
	appRegistry       = map[string]AppLayer{}
)

// RegisterTransport adds a TransportLayer to the global registry.
// Call this from init() in each transport package (tcp, udp).
func RegisterTransport(t TransportLayer) {
	mu.Lock()
	defer mu.Unlock()
	name := strings.ToLower(t.Name())
	if _, dup := transportRegistry[name]; dup {
		panic(fmt.Sprintf("greet: transport %q already registered", name))
	}
	transportRegistry[name] = t
}

// RegisterApp adds an AppLayer to the global registry.
// Call this from init() in each application protocol package.
func RegisterApp(a AppLayer) {
	mu.Lock()
	defer mu.Unlock()
	name := strings.ToLower(a.Name())
	if _, dup := appRegistry[name]; dup {
		panic(fmt.Sprintf("greet: app layer %q already registered", name))
	}
	appRegistry[name] = a
}

// Resolve returns the execution Stack for the given protocol name.
// If name matches a transport (e.g. "tcp", "udp"), the stack has no app layers.
// If name matches an app layer (e.g. "tls", "minecraft"), the stack is
// [required transport] + [app layer].
func Resolve(name string) (*Stack, error) {
	mu.RLock()
	defer mu.RUnlock()
	key := strings.ToLower(name)

	// Check transport registry first (tcp, udp are standalone stacks)
	if t, ok := transportRegistry[key]; ok {
		return &Stack{Transport: t}, nil
	}

	// Check app layer registry
	if a, ok := appRegistry[key]; ok {
		base := strings.ToLower(string(a.BaseTransport()))
		t, ok := transportRegistry[base]
		if !ok {
			return nil, &GreetError{
				Code:    ErrUnknownProtocol,
				Message: fmt.Sprintf("app layer %q requires transport %q which is not registered", name, base),
			}
		}
		return &Stack{Transport: t, Apps: []AppLayer{a}}, nil
	}

	return nil, &GreetError{
		Code:    ErrUnknownProtocol,
		Message: fmt.Sprintf("unknown protocol %q", name),
	}
}

// ResolveStack builds a Stack from an explicit ordered list of layer names.
// The first name must be a transport; subsequent names must be app layers
// compatible with that transport.
func ResolveStack(names []string) (*Stack, error) {
	if len(names) == 0 {
		return nil, &GreetError{
			Code:    ErrUnknownProtocol,
			Message: "layer list is empty",
		}
	}
	mu.RLock()
	defer mu.RUnlock()

	transportName := strings.ToLower(names[0])
	t, ok := transportRegistry[transportName]
	if !ok {
		return nil, &GreetError{
			Code:    ErrUnknownProtocol,
			Message: fmt.Sprintf("first layer %q is not a registered transport", names[0]),
		}
	}

	apps := make([]AppLayer, 0, len(names)-1)
	for _, n := range names[1:] {
		key := strings.ToLower(n)
		a, ok := appRegistry[key]
		if !ok {
			return nil, &GreetError{
				Code:    ErrUnknownProtocol,
				Message: fmt.Sprintf("layer %q is not a registered app layer", n),
			}
		}
		if strings.ToLower(string(a.BaseTransport())) != transportName {
			return nil, &GreetError{
				Code:    ErrUnknownProtocol,
				Message: fmt.Sprintf("app layer %q requires transport %q but stack uses %q", n, a.BaseTransport(), names[0]),
			}
		}
		apps = append(apps, a)
	}
	return &Stack{Transport: t, Apps: apps}, nil
}

// ListAll returns every registered transport and app layer as a flat list of
// (name, description, defaultPort, transport) tuples, sorted by name.
// Used by the CLI "list" subcommand.
type LayerInfo struct {
	Name        string
	Description string
	DefaultPort int
	Transport   Transport
}

func ListAll() []LayerInfo {
	mu.RLock()
	defer mu.RUnlock()
	var out []LayerInfo
	for _, t := range transportRegistry {
		out = append(out, LayerInfo{
			Name:        t.Name(),
			Description: fmt.Sprintf("%s transport", t.Transport()),
			DefaultPort: t.DefaultPort(),
			Transport:   t.Transport(),
		})
	}
	for _, a := range appRegistry {
		out = append(out, LayerInfo{
			Name:        a.Name(),
			Description: a.Description(),
			DefaultPort: a.DefaultPort(),
			Transport:   a.BaseTransport(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
