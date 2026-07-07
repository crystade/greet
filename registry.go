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

var (
	mu                sync.RWMutex
	transportRegistry = map[string]TransportLayer{}
	appRegistry       = map[string]AppLayer{}
	compositeRegistry = map[string]stackEntry{}
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

// RegisterStack registers a pre-built composite stack under a given name.
// layers must be an ordered slice starting with a transport name followed by
// app-layer names (e.g. ["tcp", "tls", "http"]). defaultPort overrides the
// outermost layer's default port (pass 0 to use the outermost layer's port).
// Call this from init() in a protocol package — all referenced layers must
// already be registered before RegisterStack is called.
func RegisterStack(name string, layers []string, defaultPort int, description string) {
	mu.Lock()
	defer mu.Unlock()
	key := strings.ToLower(name)
	if _, dup := compositeRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q already registered", key))
	}
	if _, dup := appRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q conflicts with app layer", key))
	}
	if _, dup := transportRegistry[key]; dup {
		panic(fmt.Sprintf("greet: composite stack %q conflicts with transport", key))
	}

	if len(layers) == 0 {
		panic(fmt.Sprintf("greet: composite stack %q has empty layer list", key))
	}
	transportName := strings.ToLower(layers[0])
	t, ok := transportRegistry[transportName]
	if !ok {
		panic(fmt.Sprintf("greet: composite stack %q: transport %q not registered", key, layers[0]))
	}
	apps := make([]AppLayer, 0, len(layers)-1)
	for _, n := range layers[1:] {
		a, ok := appRegistry[strings.ToLower(n)]
		if !ok {
			panic(fmt.Sprintf("greet: composite stack %q: app layer %q not registered", key, n))
		}
		apps = append(apps, a)
	}
	stack := &Stack{Transport: t, Apps: apps}
	compositeRegistry[key] = stackEntry{stack: stack, defaultPort: defaultPort, description: description}
}

// Resolve returns the execution Stack for the given protocol name.
// If name matches a transport (e.g. "tcp", "udp"), the stack has no app layers.
// If name matches an app layer (e.g. "tls", "minecraft"), the stack is
// [required transport] + [app layer].
// If name matches a composite stack (e.g. "https"), the pre-built stack is returned.
func Resolve(name string) (*Stack, error) {
	mu.RLock()
	defer mu.RUnlock()
	key := strings.ToLower(name)

	// Check composite registry first (e.g. "https" = tcp+tls+http)
	if entry, ok := compositeRegistry[key]; ok {
		s := *entry.stack // shallow copy so callers can't mutate the registry copy
		s.overrideName = key
		if entry.defaultPort > 0 {
			s.overridePort = entry.defaultPort
		}
		return &s, nil
	}

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
	for name, entry := range compositeRegistry {
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
