package mcp

import "github.com/modelcontextprotocol/go-sdk/mcp"

// ProtocolName is the registered name for the MCP application layer.
const ProtocolName = "mcp"

// DefaultMCPPort is the well-known HTTPS port for the MCP streamable transport.
const DefaultMCPPort = 443

// MCPConfig holds protocol-specific configuration for an MCP probe.
type MCPConfig struct {
	// ClientInfo is sent in the initialize request.
	ClientInfo mcp.Implementation
	// Path is the request path for MCP streamable HTTP requests (e.g. "/mcp").
	// Defaults to "/" when empty.
	Path string
	// BearerToken is an optional OAuth 2.0 Bearer token sent in the
	// Authorization header on every request. When a server returns
	// 401 with a Bearer WWW-Authenticate challenge, greet probes the
	// resource metadata well-known endpoint for diagnostic information.
	BearerToken string
	// ListResources also runs resources/list after the handshake.
	ListResources bool
	// ListTools also runs tools/list after the handshake.
	ListTools bool
	// ListPrompts also runs prompts/list after the handshake.
	ListPrompts bool
	// ListResourceTemplates also runs resources/templates/list after the handshake.
	ListResourceTemplates bool
}

// LayerConfigName returns the layer name for config lookup.
func (c *MCPConfig) LayerConfigName() string { return ProtocolName }
