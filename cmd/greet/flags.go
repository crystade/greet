package main

import (
	"flag"

	"github.com/crystade/greet"
)

// flagHandler bundles register and parse functions for a single protocol's CLI flags.
type flagHandler struct {
	register func(fs *flag.FlagSet)
	parse    func(fs *flag.FlagSet) ([]greet.GreetOption, error)
}

// flagHandlers is the registry of protocol-specific CLI flag handlers,
// keyed by layer name (e.g. "tls", "http", "minecraft", "postgresql").
// Each protocol's flags_*.go file registers its handler via init().
var flagHandlers = map[string]flagHandler{}

// registerFlagHandler registers a flagHandler for the given layer name.
func registerFlagHandler(name string, h flagHandler) {
	flagHandlers[name] = h
}
