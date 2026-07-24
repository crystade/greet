package mcp

import "github.com/modelcontextprotocol/go-sdk/mcp"

// MCPResult holds the outcome of an MCP handshake and optional list probes.
type MCPResult struct {
	ProtocolVersion   string                  `json:"protocolVersion"`
	ServerInfo        *mcp.Implementation     `json:"serverInfo,omitempty"`
	Capabilities      *mcp.ServerCapabilities `json:"capabilities,omitempty"`
	Resources         []*ResourceInfo         `json:"resources,omitempty"`
	ResourceTemplates []*ResourceTemplateInfo `json:"resourceTemplates,omitempty"`
	Tools             []*ToolInfo             `json:"tools,omitempty"`
	Prompts           []*PromptInfo           `json:"prompts,omitempty"`
}

// ResourceInfo is a compact view of an MCP resource.
type ResourceInfo struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// ResourceTemplateInfo is a compact view of an MCP resource template.
type ResourceTemplateInfo struct {
	Name        string `json:"name"`
	URITemplate string `json:"uriTemplate"`
}

// ToolInfo is a compact view of an MCP tool.
type ToolInfo struct {
	Name string `json:"name"`
}

// PromptInfo is a compact view of an MCP prompt.
type PromptInfo struct {
	Name string `json:"name"`
}

// LayerDataName returns the layer name for result aggregation.
func (r *MCPResult) LayerDataName() string { return ProtocolName }
