package greet

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	mu               sync.RWMutex
	protocolRegistry = map[string]Protocol{}
)

// Register adds a protocol implementation to the global registry.
// Call this from init() in each protocol package.
func Register(p Protocol) {
	mu.Lock()
	defer mu.Unlock()
	name := strings.ToLower(p.Name())
	if _, dup := protocolRegistry[name]; dup {
		panic(fmt.Sprintf("greet: protocol %q already registered", name))
	}
	protocolRegistry[name] = p
}

// Get looks up a protocol by name (case-insensitive).
func Get(name string) (Protocol, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := protocolRegistry[strings.ToLower(name)]
	if !ok {
		return nil, &GreetError{
			Code:    ErrUnknownProtocol,
			Message: fmt.Sprintf("unknown protocol %q", name),
		}
	}
	return p, nil
}

// List returns every registered protocol, sorted by name.
func List() []Protocol {
	mu.RLock()
	defer mu.RUnlock()
	protocols := make([]Protocol, 0, len(protocolRegistry))
	for _, p := range protocolRegistry {
		protocols = append(protocols, p)
	}
	sort.Slice(protocols, func(i, j int) bool {
		return protocols[i].Name() < protocols[j].Name()
	})
	return protocols
}
