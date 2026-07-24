package main

import (
	"flag"

	"github.com/crystade/greet"
	greetmcp "github.com/crystade/greet/protocols/mcp"
)

func init() {
	registerFlagHandler(greetmcp.ProtocolName, flagHandler{
		register: registerMCPFlags,
		parse:    parseMCPFlags,
	})
}

func registerMCPFlags(fs *flag.FlagSet) {
	fs.String("path", "/", "Request path for MCP streamable HTTP (e.g. /mcp)")
	fs.String("bearer-token", "", "Bearer token for MCP Authorization header")
	fs.Bool("list-resources", false, "List server resources after the handshake")
	fs.Bool("list-tools", false, "List server tools after the handshake")
	fs.Bool("list-prompts", false, "List server prompts after the handshake")
	fs.Bool("list-resource-templates", false, "List resource templates after the handshake")
}

func parseMCPFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	cfg := &greetmcp.MCPConfig{}

	cfg.Path = fs.Lookup("path").Value.String()

	cfg.BearerToken = fs.Lookup("bearer-token").Value.String()

	if f := fs.Lookup("list-resources"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				cfg.ListResources = v
			}
		}
	}
	if f := fs.Lookup("list-tools"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				cfg.ListTools = v
			}
		}
	}
	if f := fs.Lookup("list-prompts"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				cfg.ListPrompts = v
			}
		}
	}
	if f := fs.Lookup("list-resource-templates"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				cfg.ListResourceTemplates = v
			}
		}
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(greetmcp.ProtocolName, cfg),
	}, nil
}
