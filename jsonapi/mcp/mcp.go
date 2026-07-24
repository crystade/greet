// Package mcp provides the JSON-serializable model, mapper, and option
// decoder for the MCP layer.
package mcp

import (
	"encoding/json"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	greetmcp "github.com/crystade/greet/protocols/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Data is the JSON-serializable payload for an MCP layer.
type Data struct {
	ProtocolVersion   string                           `json:"protocolVersion"`
	ServerInfo        *sdkmcp.Implementation           `json:"serverInfo,omitempty"`
	Capabilities      *sdkmcp.ServerCapabilities       `json:"capabilities,omitempty"`
	Resources         []*greetmcp.ResourceInfo         `json:"resources,omitempty"`
	ResourceTemplates []*greetmcp.ResourceTemplateInfo `json:"resourceTemplates,omitempty"`
	Tools             []*greetmcp.ToolInfo             `json:"tools,omitempty"`
	Prompts           []*greetmcp.PromptInfo           `json:"prompts,omitempty"`
}

// Options holds JSON-serializable MCP layer options.
type Options struct {
	ClientInfo            *sdkmcp.Implementation `json:"clientInfo,omitempty"`
	Path                  string                 `json:"path,omitempty"`
	BearerToken           string                 `json:"bearerToken,omitempty"`
	ListResources         bool                   `json:"listResources,omitempty"`
	ListTools             bool                   `json:"listTools,omitempty"`
	ListPrompts           bool                   `json:"listPrompts,omitempty"`
	ListResourceTemplates bool                   `json:"listResourceTemplates,omitempty"`
}

// FromResult maps a *greetmcp.MCPResult to *Data. It returns the input
// unchanged when data is not a *greetmcp.MCPResult.
func FromResult(data greet.LayerData) any {
	r, ok := data.(*greetmcp.MCPResult)
	if !ok || r == nil {
		return data
	}
	return &Data{
		ProtocolVersion:   r.ProtocolVersion,
		ServerInfo:        r.ServerInfo,
		Capabilities:      r.Capabilities,
		Resources:         r.Resources,
		ResourceTemplates: r.ResourceTemplates,
		Tools:             r.Tools,
		Prompts:           r.Prompts,
	}
}

// DecodeOptions decodes raw MCP options into engine GreetOptions.
func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
	var o Options
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	cfg := &greetmcp.MCPConfig{
		Path:                  o.Path,
		BearerToken:           o.BearerToken,
		ListResources:         o.ListResources,
		ListTools:             o.ListTools,
		ListPrompts:           o.ListPrompts,
		ListResourceTemplates: o.ListResourceTemplates,
	}
	if o.ClientInfo != nil {
		cfg.ClientInfo = *o.ClientInfo
	}
	return []greet.GreetOption{
		greet.WithLayerConfig(greetmcp.ProtocolName, cfg),
	}, nil
}

// Register registers the MCP data mapper and option decoder on the given Codec.
func Register(c *jsonapi.Codec) {
	c.RegisterDataMapper(greetmcp.ProtocolName, FromResult)
	c.RegisterOptionDecoder(greetmcp.ProtocolName, DecodeOptions)
}
