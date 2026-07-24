package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/crystade/greet"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// MCP implements an MCP streamable HTTP application-layer probe.
type MCP struct{}

// New creates a new MCP application layer.
func New() *MCP { return &MCP{} }

func (m *MCP) Name() string                   { return ProtocolName }
func (m *MCP) Description() string            { return "MCP handshake + capability probe" }
func (m *MCP) DefaultPort() int               { return DefaultMCPPort }
func (m *MCP) BaseTransport() greet.Transport { return greet.TransportTCP }

// Handshake performs the MCP initialize handshake and optional list probes
// over the supplied TLS connection.
func (m *MCP) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, greet.LayerData, greet.LayerTiming, error) {
	mcpCfg := &MCPConfig{}
	if raw := cfg.LayerConfig(ProtocolName); raw != nil {
		c, ok := raw.(*MCPConfig)
		if !ok {
			return conn, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: expected *MCPConfig, got %T", raw),
				Protocol: ProtocolName,
			}
		}
		mcpCfg = c
	}
	if mcpCfg.ClientInfo.Name == "" {
		mcpCfg.ClientInfo.Name = "greet"
	}
	if mcpCfg.ClientInfo.Version == "" {
		mcpCfg.ClientInfo.Version = "0.0.0"
	}

	mcpPath := mcpCfg.Path
	if mcpPath == "" {
		mcpPath = "/"
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline) //nolint:errcheck
	}

	br := bufio.NewReader(conn)
	protocolVersion := "2025-06-18"
	sessionID := ""
	var ttfb time.Duration

	// Step 1: initialize request.
	initBody, err := buildRequest(1, "initialize", mcp.InitializeParams{
		ProtocolVersion: protocolVersion,
		ClientInfo:      &mcpCfg.ClientInfo,
		Capabilities:    &mcp.ClientCapabilities{},
	})
	if err != nil {
		return conn, nil, greet.LayerTiming{}, newError(ErrMCPInitializeFailed, fmt.Sprintf("failed to build initialize request: %v", err), err)
	}

	resp, body, err := m.post(ctx, conn, br, host, port, initBody, mcpPath, sessionID, protocolVersion, mcpCfg.BearerToken)
	if err != nil {
		return conn, nil, greet.LayerTiming{}, newError(ErrMCPInitializeFailed, fmt.Sprintf("failed to send initialize request: %v", err), err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return conn, nil, greet.LayerTiming{}, m.analyzeAuthError(ctx, resp, host, port, mcpPath)
	}
	if resp.StatusCode != http.StatusOK {
		return conn, nil, greet.LayerTiming{}, newError(ErrMCPUnexpectedStatus, fmt.Sprintf("initialize returned HTTP status %d: %s", resp.StatusCode, string(body)), nil)
	}

	sessionID = resp.Header.Get("Mcp-Session-Id")
	// Sessionless servers omit Mcp-Session-Id; greet operates in stateless
	// mode and simply omits the header on subsequent requests.
	ttfb = time.Since(start)

	rpcResp, err := parseJSONRPCResponse(body)
	if err != nil {
		return conn, nil, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPInitializeFailed,
			fmt.Sprintf("unexpected Content-Type %q (expected application/json)", resp.Header.Get("Content-Type")), err)
	}
	if rpcResp.Error != nil {
		return conn, nil, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPInitializeFailed, fmt.Sprintf("initialize returned JSON-RPC error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code), nil)
	}

	var initResult mcp.InitializeResult
	if err := json.Unmarshal(rpcResp.Result, &initResult); err != nil {
		return conn, nil, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPInitializeFailed, fmt.Sprintf("failed to parse initialize result: %v", err), err)
	}

	result := &MCPResult{
		ProtocolVersion: initResult.ProtocolVersion,
		ServerInfo:      initResult.ServerInfo,
		Capabilities:    initResult.Capabilities,
	}

	// Step 2: initialized notification.
	notifBody, err := buildNotification("notifications/initialized", struct{}{})
	if err != nil {
		return conn, result, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPInitializeFailed, fmt.Sprintf("failed to build initialized notification: %v", err), err)
	}

	resp, body, err = m.post(ctx, conn, br, host, port, notifBody, mcpPath, sessionID, initResult.ProtocolVersion, mcpCfg.BearerToken)
	if err != nil {
		return conn, result, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPInitializeFailed, fmt.Sprintf("failed to send initialized notification: %v", err), err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return conn, result, greet.LayerTiming{TTFB: ttfb}, newError(ErrMCPUnexpectedStatus, fmt.Sprintf("initialized notification returned HTTP status %d", resp.StatusCode), nil)
	}
	_ = body
	ttlb := time.Since(start)

	// Step 3: optional list probes — only run those the server advertises
	// in its capabilities to avoid -32601 "Method not found" errors.
	reqID := 2
	caps := initResult.Capabilities
	if mcpCfg.ListResources && caps != nil && caps.Resources != nil {
		resources, err := m.listResources(ctx, conn, br, host, port, mcpPath, &reqID, sessionID, initResult.ProtocolVersion, mcpCfg.BearerToken, start)
		ttlb = time.Since(start)
		if err != nil {
			return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, err
		}
		result.Resources = resources
	}

	if mcpCfg.ListTools && caps != nil && caps.Tools != nil {
		tools, err := m.listTools(ctx, conn, br, host, port, mcpPath, &reqID, sessionID, initResult.ProtocolVersion, mcpCfg.BearerToken, start)
		ttlb = time.Since(start)
		if err != nil {
			return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, err
		}
		result.Tools = tools
	}

	if mcpCfg.ListPrompts && caps != nil && caps.Prompts != nil {
		prompts, err := m.listPrompts(ctx, conn, br, host, port, mcpPath, &reqID, sessionID, initResult.ProtocolVersion, mcpCfg.BearerToken, start)
		ttlb = time.Since(start)
		if err != nil {
			return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, err
		}
		result.Prompts = prompts
	}

	if mcpCfg.ListResourceTemplates && caps != nil && caps.Resources != nil {
		templates, err := m.listResourceTemplates(ctx, conn, br, host, port, mcpPath, &reqID, sessionID, initResult.ProtocolVersion, mcpCfg.BearerToken, start)
		ttlb = time.Since(start)
		if err != nil {
			return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, err
		}
		result.ResourceTemplates = templates
	}

	return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
}

func (m *MCP) listResources(ctx context.Context, conn net.Conn, br *bufio.Reader, host string, port int, path string, reqID *int, sessionID, protocolVersion, bearerToken string, start time.Time) ([]*ResourceInfo, error) {
	body, err := buildRequest(*reqID, "resources/list", struct{}{})
	*reqID++
	if err != nil {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("failed to build resources/list request: %v", err), err)
	}
	resp, respBody, err := m.post(ctx, conn, br, host, port, body, path, sessionID, protocolVersion, bearerToken)
	if err != nil {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("resources/list request failed: %v", err), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("resources/list returned HTTP status %d", resp.StatusCode), nil)
	}
	rpcResp, err := parseJSONRPCResponse(respBody)
	if err != nil {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("failed to parse resources/list JSON-RPC response: %v", err), err)
	}
	if rpcResp.Error != nil {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("resources/list returned JSON-RPC error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code), nil)
	}
	var listResult mcp.ListResourcesResult
	if err := json.Unmarshal(rpcResp.Result, &listResult); err != nil {
		return nil, newError(ErrMCPListResourcesFailed, fmt.Sprintf("failed to parse resources/list result: %v", err), err)
	}
	out := make([]*ResourceInfo, len(listResult.Resources))
	for i, r := range listResult.Resources {
		out[i] = &ResourceInfo{Name: r.Name, URI: r.URI}
	}
	return out, nil
}

func (m *MCP) listTools(ctx context.Context, conn net.Conn, br *bufio.Reader, host string, port int, path string, reqID *int, sessionID, protocolVersion, bearerToken string, start time.Time) ([]*ToolInfo, error) {
	body, err := buildRequest(*reqID, "tools/list", struct{}{})
	*reqID++
	if err != nil {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("failed to build tools/list request: %v", err), err)
	}
	resp, respBody, err := m.post(ctx, conn, br, host, port, body, path, sessionID, protocolVersion, bearerToken)
	if err != nil {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("tools/list request failed: %v", err), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("tools/list returned HTTP status %d", resp.StatusCode), nil)
	}
	rpcResp, err := parseJSONRPCResponse(respBody)
	if err != nil {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("failed to parse tools/list JSON-RPC response: %v", err), err)
	}
	if rpcResp.Error != nil {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("tools/list returned JSON-RPC error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code), nil)
	}
	var listResult mcp.ListToolsResult
	if err := json.Unmarshal(rpcResp.Result, &listResult); err != nil {
		return nil, newError(ErrMCPListToolsFailed, fmt.Sprintf("failed to parse tools/list result: %v", err), err)
	}
	out := make([]*ToolInfo, len(listResult.Tools))
	for i, t := range listResult.Tools {
		out[i] = &ToolInfo{Name: t.Name}
	}
	return out, nil
}

func (m *MCP) listPrompts(ctx context.Context, conn net.Conn, br *bufio.Reader, host string, port int, path string, reqID *int, sessionID, protocolVersion, bearerToken string, start time.Time) ([]*PromptInfo, error) {
	body, err := buildRequest(*reqID, "prompts/list", struct{}{})
	*reqID++
	if err != nil {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("failed to build prompts/list request: %v", err), err)
	}
	resp, respBody, err := m.post(ctx, conn, br, host, port, body, path, sessionID, protocolVersion, bearerToken)
	if err != nil {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("prompts/list request failed: %v", err), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("prompts/list returned HTTP status %d", resp.StatusCode), nil)
	}
	rpcResp, err := parseJSONRPCResponse(respBody)
	if err != nil {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("failed to parse prompts/list JSON-RPC response: %v", err), err)
	}
	if rpcResp.Error != nil {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("prompts/list returned JSON-RPC error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code), nil)
	}
	var listResult mcp.ListPromptsResult
	if err := json.Unmarshal(rpcResp.Result, &listResult); err != nil {
		return nil, newError(ErrMCPListPromptsFailed, fmt.Sprintf("failed to parse prompts/list result: %v", err), err)
	}
	out := make([]*PromptInfo, len(listResult.Prompts))
	for i, p := range listResult.Prompts {
		out[i] = &PromptInfo{Name: p.Name}
	}
	return out, nil
}

func (m *MCP) listResourceTemplates(ctx context.Context, conn net.Conn, br *bufio.Reader, host string, port int, path string, reqID *int, sessionID, protocolVersion, bearerToken string, start time.Time) ([]*ResourceTemplateInfo, error) {
	body, err := buildRequest(*reqID, "resources/templates/list", struct{}{})
	*reqID++
	if err != nil {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("failed to build resources/templates/list request: %v", err), err)
	}
	resp, respBody, err := m.post(ctx, conn, br, host, port, body, path, sessionID, protocolVersion, bearerToken)
	if err != nil {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("resources/templates/list request failed: %v", err), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("resources/templates/list returned HTTP status %d", resp.StatusCode), nil)
	}
	rpcResp, err := parseJSONRPCResponse(respBody)
	if err != nil {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("failed to parse resources/templates/list JSON-RPC response: %v", err), err)
	}
	if rpcResp.Error != nil {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("resources/templates/list returned JSON-RPC error: %s (code %d)", rpcResp.Error.Message, rpcResp.Error.Code), nil)
	}
	var listResult mcp.ListResourceTemplatesResult
	if err := json.Unmarshal(rpcResp.Result, &listResult); err != nil {
		return nil, newError(ErrMCPListTemplatesFailed, fmt.Sprintf("failed to parse resources/templates/list result: %v", err), err)
	}
	out := make([]*ResourceTemplateInfo, len(listResult.ResourceTemplates))
	for i, t := range listResult.ResourceTemplates {
		out[i] = &ResourceTemplateInfo{Name: t.Name, URITemplate: t.URITemplate}
	}
	return out, nil
}

// post sends a JSON-RPC body as an HTTP POST over conn and returns the parsed response.
// It uses http.NewRequest + req.Write(conn) for clean HTTP framing, matching the
// pattern in the HTTP protocol layer.
func (m *MCP) post(ctx context.Context, conn net.Conn, br *bufio.Reader, host string, port int, body []byte, path string, sessionID, protocolVersion, bearerToken string) (*http.Response, []byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline) //nolint:errcheck
	}

	urlStr := "https://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)) + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", protocolVersion)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	// Strip default port from the Host header so that requests to
	// e.g. api.githubcopilot.com:443 carry Host: api.githubcopilot.com.
	// Many CDNs and load balancers (including Azure Front Door) reject
	// requests whose Host header includes the default port.
	if port == 443 {
		req.Host = host
	} else {
		req.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}

	if err := req.Write(conn); err != nil {
		return nil, nil, err
	}

	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	// If the server responded with SSE (text/event-stream), extract the
	// JSON-RPC message from the first non-empty "data:" line.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/event-stream") {
		respBody = extractFirstSSEData(respBody)
	}

	return resp, respBody, nil
}

func buildRequest(id int, method string, params any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
}

func buildNotification(method string, params any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func parseJSONRPCResponse(body []byte) (*jsonRPCResponse, error) {
	var resp jsonRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// extractFirstSSEData scans an SSE body for the first non-empty "data:" line
// and returns its payload. Returns the input unchanged if no data line is found.
func extractFirstSSEData(body []byte) []byte {
	lines := strings.SplitSeq(string(body), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			data := strings.TrimSpace(after)
			if data != "" {
				return []byte(data)
			}
		}
	}
	return body
}

// analyzeAuthError inspects a 401 response's WWW-Authenticate header to
// determine whether the server expects an OAuth 2.0 Bearer token or a
// different (unsupported) authentication scheme.
func (m *MCP) analyzeAuthError(ctx context.Context, resp *http.Response, host string, port int, mcpPath string) *greet.GreetError {
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("Www-Authenticate"))
	if err != nil || len(challenges) == 0 {
		return newError(ErrMCPAuthFailed,
			"server returned HTTP 401 with no parseable WWW-Authenticate header", err)
	}

	bearer := findBearerChallenge(challenges)
	if bearer == nil {
		schemes := challengeSchemes(challenges)
		return newError(ErrMCPUnsupportedOAuth,
			fmt.Sprintf("server returned HTTP 401 with unsupported auth scheme(s): %s", strings.Join(schemes, ", ")), nil)
	}

	msg := "server returned HTTP 401; MCP endpoint requires Bearer authentication"
	if errDesc := bearer.Params["error_description"]; errDesc != "" {
		msg += ": " + errDesc
	}

	// Best-effort probe of protected resource metadata.
	resourceURL := fmt.Sprintf("https://%s%s", net.JoinHostPort(host, fmt.Sprintf("%d", port)), mcpPath)
	resourceMetadataURL := bearer.Params["resource_metadata"]
	prm, probeErr := probeProtectedResourceMetadata(ctx, host, port, resourceURL, resourceMetadataURL)
	if probeErr == nil && prm != nil {
		if len(prm.ScopesSupported) > 0 {
			msg += fmt.Sprintf("; scopes: %s", strings.Join(prm.ScopesSupported, ", "))
		}
		if len(prm.AuthorizationServers) > 0 {
			msg += fmt.Sprintf("; auth servers: %s", strings.Join(prm.AuthorizationServers, ", "))
		}
	}

	return newError(ErrMCPAuthFailed, msg, nil)
}

// probeProtectedResourceMetadata fetches OAuth 2.0 protected resource metadata
// from the well-known endpoint. It returns nil on any failure so callers can
// gracefully degrade (the auth error is still reported).
func probeProtectedResourceMetadata(ctx context.Context, host string, port int, resourceURL, resourceMetadataURL string) (*oauthex.ProtectedResourceMetadata, error) {
	wellKnownURL := resourceMetadataURL
	if wellKnownURL == "" {
		// Per RFC 9728 §3, the default well-known path is at the server root.
		wellKnownURL = fmt.Sprintf("https://%s/.well-known/oauth-protected-resource",
			net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	}
	// Validate the well-known URL is safe.
	u, err := url.Parse(wellKnownURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && !strings.HasPrefix(u.Hostname(), "127.") {
		return nil, fmt.Errorf("well-known URL must be HTTPS or loopback, got %q", wellKnownURL)
	}

	client := &http.Client{}
	return oauthex.GetProtectedResourceMetadata(ctx, wellKnownURL, resourceURL, client)
}

// findBearerChallenge returns the first Bearer challenge from the slice.
func findBearerChallenge(cs []oauthex.Challenge) *oauthex.Challenge {
	for i := range cs {
		if strings.EqualFold(cs[i].Scheme, "bearer") {
			return &cs[i]
		}
	}
	return nil
}

// challengeSchemes returns a deduplicated list of challenge scheme names.
func challengeSchemes(cs []oauthex.Challenge) []string {
	seen := make(map[string]bool)
	var schemes []string
	for _, c := range cs {
		if !seen[c.Scheme] {
			seen[c.Scheme] = true
			schemes = append(schemes, c.Scheme)
		}
	}
	return schemes
}
