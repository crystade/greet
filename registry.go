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
	Transport    TransportLayer
	Apps         []AppLayer
	overrideName string // set by composite stacks (e.g. "https")
	overridePort int    // set by composite stacks to override outermost layer's port
}

// Name returns the name of the outermost (last) layer in the stack.
// For a bare transport (e.g. "tcp") this is the transport name.
// Composite stacks (e.g. "https") override this with their registered name.
func (s *Stack) Name() string {
	if s.overrideName != "" {
		return s.overrideName
	}
	if len(s.Apps) > 0 {
		return s.Apps[len(s.Apps)-1].Name()
	}
	return s.Transport.Name()
}

// DefaultPort returns the default port of the outermost layer.
// Composite stacks may override this (e.g. "https" uses 443 even though
// the outermost "http" layer defaults to 80).
func (s *Stack) DefaultPort() int {
	if s.overridePort > 0 {
		return s.overridePort
	}
	if len(s.Apps) > 0 {
		return s.Apps[len(s.Apps)-1].DefaultPort()
	}
	return s.Transport.DefaultPort()
}

// stackEntry is a pre-built composite stack registered by name (e.g. "https").
type stackEntry struct {
	stack       *Stack
	defaultPort int
	description string
}

// Engine is an isolated greet instance with its own protocol registry.
// Create one via NewEngine() and register protocols with RegisterTransport(),
// RegisterApp(), and RegisterStack().
type Engine struct {
	mu                sync.RWMutex
	transportRegistry map[string]TransportLayer
	appRegistry       map[string]AppLayer
	compositeRegistry map[string]stackEntry
}

// NewEngine creates a new empty Engine.
func NewEngine() *Engine {
	return &Engine{
		transportRegistry: make(map[string]TransportLayer),
		appRegistry:       make(map[string]AppLayer),
		compositeRegistry: make(map[string]stackEntry),
	}
}

// RegisterTransport adds a TransportLayer to the engine's registry.
func (e *Engine) RegisterTransport(t TransportLayer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	name := strings.ToLower(t.Name())
	if _, dup := e.transportRegistry[name]; dup {
		panic(fmt.Sprintf("greet: transport %q already registered", name))
	}
	e.transportRegistry[name] = t
}

// RegisterApp adds an AppLayer to the engine's registry.
func (e *Engine) RegisterApp(a AppLayer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	name := strings.ToLower(a.Name())
	if _, dup := e.appRegistry[name]; dup {
		panic(fmt.Sprintf("greet: app layer %q already registered", name))
	}
	e.appRegistry[name] = a
}

// RegisterStack registers a pre-built composite stack under a given name.
// layers must be an ordered slice starting with a transport name followed by
// app-layer names (e.g. ["tcp", "tls", "http"]). defaultPort overrides the
// outermost layer's default port (pass 0 to use the outermost layer's port).
func (e *Engine) RegisterStack(name string, layers []string, defaultPort int, description string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := e.compositeRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q already registered", key))
	}
	if _, dup := e.appRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q conflicts with app layer", key))
	}
	if _, dup := e.transportRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q conflicts with transport", key))
	}

	if len(layers) == 0 {
		panic(fmt.Sprintf("greet: composite stack %q has empty layer list", key))
	}
	transportName := strings.ToLower(layers[0])
	t, ok := e.transportRegistry[transportName]
	if !ok {
		panic(fmt.Sprintf("greet: composite stack %q: transport %q not registered", key, layers[0]))
	}
	apps := make([]AppLayer, 0, len(layers)-1)
	for _, n := range layers[1:] {
		a, ok := e.appRegistry[strings.ToLower(n)]
		if !ok {
			panic(fmt.Sprintf("greet: composite stack %q: app layer %q not registered", key, n))
		}
		apps = append(apps, a)
	}
	stack := &Stack{Transport: t, Apps: apps}
	e.compositeRegistry[key] = stackEntry{stack: stack, defaultPort: defaultPort, description: description}
}

// Resolve returns the execution Stack for the given protocol name.
// If name matches a transport (e.g. "tcp", "udp"), the stack has no app layers.
// If name matches an app layer (e.g. "tls", "minecraft"), the stack is
// [required transport] + [app layer].
// If name matches a composite stack (e.g. "https"), the pre-built stack is returned.
func (e *Engine) Resolve(name string) (*Stack, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	key := strings.ToLower(name)

	// Check composite registry first (e.g. "https" = tcp+tls+http)
	if entry, ok := e.compositeRegistry[key]; ok {
		s := *entry.stack // shallow copy so callers can't mutate the registry copy
		s.overrideName = key
		if entry.defaultPort > 0 {
			s.overridePort = entry.defaultPort
		}
		return &s, nil
	}

	// Check transport registry first (tcp, udp are standalone stacks)
	if t, ok := e.transportRegistry[key]; ok {
		return &Stack{Transport: t}, nil
	}

	// Check app layer registry
	if a, ok := e.appRegistry[key]; ok {
		base := strings.ToLower(string(a.BaseTransport()))
		t, ok := e.transportRegistry[base]
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
func (e *Engine) ResolveStack(names []string) (*Stack, error) {
	if len(names) == 0 {
		return nil, &GreetError{
			Code:    ErrUnknownProtocol,
			Message: "layer list is empty",
		}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	transportName := strings.ToLower(names[0])
	t, ok := e.transportRegistry[transportName]
	if !ok {
		return nil, &GreetError{
			Code:    ErrUnknownProtocol,
			Message: fmt.Sprintf("first layer %q is not a registered transport", names[0]),
		}
	}

	apps := make([]AppLayer, 0, len(names)-1)
	for _, n := range names[1:] {
		key := strings.ToLower(n)
		a, ok := e.appRegistry[key]
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

// LayerInfo describes a registered layer.
type LayerInfo struct {
	Name        string
	Description string
	DefaultPort int
	Transport   Transport
}

// ListAll returns every registered transport and app layer as a flat list of
// (name, description, defaultPort, transport) tuples, sorted by name.
func (e *Engine) ListAll() []LayerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []LayerInfo
	for _, t := range e.transportRegistry {
		out = append(out, LayerInfo{
			Name:        t.Name(),
			Description: fmt.Sprintf("%s transport", t.Transport()),
			DefaultPort: t.DefaultPort(),
			Transport:   t.Transport(),
		})
	}
	for _, a := range e.appRegistry {
		out = append(out, LayerInfo{
			Name:        a.Name(),
			Description: a.Description(),
			DefaultPort: a.DefaultPort(),
			Transport:   a.BaseTransport(),
		})
	}
	for name, entry := range e.compositeRegistry {
		port := entry.defaultPort
		if port == 0 {
			port = entry.stack.DefaultPort()
		}
		out = append(out, LayerInfo{
			Name:        name,
			Description: entry.description,
			DefaultPort: port,
			Transport:   entry.stack.Transport.Transport(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
