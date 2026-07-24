package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/all"
	greetmcp "github.com/crystade/greet/protocols/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      int     `json:"id"`
	Result  any     `json:"result,omitempty"`
	Error   *rpcErr `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(handler)
}

func runMockServer(t *testing.T, fn func(method string, req rpcRequest) (int, any)) *httptest.Server {
	t.Helper()
	return newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		_ = r.Body.Close()

		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		status, result := fn(req.Method, req)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "initialize" && status == http.StatusOK {
			w.Header().Set("Mcp-Session-Id", "test-session")
		}
		w.WriteHeader(status)
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
		if status != http.StatusOK && status != http.StatusAccepted && result == nil {
			resp.Error = &rpcErr{Code: -32603, Message: "internal error"}
			resp.Result = nil
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
}

func TestHandshake(t *testing.T) {
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		switch method {
		case "initialize":
			return http.StatusOK, sdkmcp.InitializeResult{
				ProtocolVersion: "2026-07-28",
				ServerInfo:      &sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"},
				Instructions:    "test instructions",
				Capabilities: &sdkmcp.ServerCapabilities{
					Tools: &sdkmcp.ToolCapabilities{},
				},
			}
		case "notifications/initialized":
			return http.StatusAccepted, nil
		default:
			return http.StatusNotFound, nil
		}
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("greet failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success")
	}

	lr, ok := result.Layer("mcp")
	if !ok {
		t.Fatalf("mcp layer not found")
	}
	if !lr.Success {
		t.Fatalf("mcp layer failed")
	}

	data, ok := lr.Data.(*greetmcp.MCPResult)
	if !ok {
		t.Fatalf("unexpected data type %T", lr.Data)
	}
	if data.ProtocolVersion != "2026-07-28" {
		t.Errorf("protocol version = %q, want %q", data.ProtocolVersion, "2026-07-28")
	}
	if data.ServerInfo == nil || data.ServerInfo.Name != "test-server" {
		t.Errorf("server info = %+v, want test-server", data.ServerInfo)
	}
	if data.Capabilities == nil {
		t.Errorf("capabilities = nil")
	}
	if data.Resources != nil {
		t.Errorf("resources = %+v, want nil", data.Resources)
	}
	if data.Tools != nil {
		t.Errorf("tools = %+v, want nil", data.Tools)
	}
}

func TestHandshakeWithLists(t *testing.T) {
	calls := []string{}
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		calls = append(calls, method)
		switch method {
		case "initialize":
			return http.StatusOK, sdkmcp.InitializeResult{
				ProtocolVersion: "2026-07-28",
				ServerInfo:      &sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"},
				Capabilities: &sdkmcp.ServerCapabilities{
					Resources: &sdkmcp.ResourceCapabilities{},
					Tools:     &sdkmcp.ToolCapabilities{},
					Prompts:   &sdkmcp.PromptCapabilities{},
				},
			}
		case "notifications/initialized":
			return http.StatusAccepted, nil
		case "resources/list":
			return http.StatusOK, sdkmcp.ListResourcesResult{Resources: []*sdkmcp.Resource{{Name: "r1", URI: "test://r1"}}}
		case "tools/list":
			return http.StatusOK, sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: "t1"}}}
		case "prompts/list":
			return http.StatusOK, sdkmcp.ListPromptsResult{Prompts: []*sdkmcp.Prompt{{Name: "p1"}}}
		case "resources/templates/list":
			return http.StatusOK, sdkmcp.ListResourceTemplatesResult{ResourceTemplates: []*sdkmcp.ResourceTemplate{{Name: "rt1", URITemplate: "test://{x}"}}}
		default:
			return http.StatusNotFound, nil
		}
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String(),
		greet.WithLayerConfig("mcp", &greetmcp.MCPConfig{
			ListResources:         true,
			ListTools:             true,
			ListPrompts:           true,
			ListResourceTemplates: true,
		}),
	)
	if err != nil {
		t.Fatalf("greet failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success")
	}

	data := result.Layers[len(result.Layers)-1].Data.(*greetmcp.MCPResult)
	if len(data.Resources) != 1 || data.Resources[0].Name != "r1" {
		t.Errorf("resources = %+v", data.Resources)
	}
	if len(data.Tools) != 1 || data.Tools[0].Name != "t1" {
		t.Errorf("tools = %+v", data.Tools)
	}
	if len(data.Prompts) != 1 || data.Prompts[0].Name != "p1" {
		t.Errorf("prompts = %+v", data.Prompts)
	}
	if len(data.ResourceTemplates) != 1 || data.ResourceTemplates[0].Name != "rt1" {
		t.Errorf("resource templates = %+v", data.ResourceTemplates)
	}

	for _, method := range []string{"resources/list", "tools/list", "prompts/list", "resources/templates/list"} {
		found := false
		for _, call := range calls {
			if call == method {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be called", method)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	var gotParams rpcRequest
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		if method == "initialize" {
			gotParams = req
		}
		return http.StatusOK, sdkmcp.InitializeResult{ProtocolVersion: "2026-07-28"}
	})
	defer server.Close()

	engine := all.NewEngine()
	if _, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String()); err != nil {
		t.Fatalf("greet failed: %v", err)
	}

	var initParams sdkmcp.InitializeParams
	if err := json.Unmarshal(gotParams.Params, &initParams); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if initParams.ClientInfo == nil {
		t.Fatalf("client info nil")
	}
	if initParams.ClientInfo.Name != "greet" {
		t.Errorf("client info name = %q, want greet", initParams.ClientInfo.Name)
	}
	if initParams.ClientInfo.Version != "0.0.0" {
		t.Errorf("client info version = %q, want 0.0.0", initParams.ClientInfo.Version)
	}
	if initParams.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocol version = %q, want 2025-06-18", initParams.ProtocolVersion)
	}
}

func TestInitializeBadStatus(t *testing.T) {
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		return http.StatusInternalServerError, nil
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	lr, ok := result.Layer("mcp")
	if !ok || lr.Success {
		t.Fatalf("mcp layer should be failed")
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "mcp_unexpected_status") {
		t.Errorf("expected mcp_unexpected_status error, got %v", result.Error)
	}
}

func TestSessionlessServer(t *testing.T) {
	// A server that omits Mcp-Session-Id is sessionless — greet should
	// complete the handshake successfully and simply omit the header on
	// the initialized notification.
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1, Result: sdkmcp.InitializeResult{ProtocolVersion: "2026-07-28"}})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success for sessionless server, got error: %v", result.Error)
	}

	lr, ok := result.Layer("mcp")
	if !ok || !lr.Success {
		t.Fatalf("mcp layer should be successful")
	}
	data := lr.Data.(*greetmcp.MCPResult)
	if data.ProtocolVersion != "2026-07-28" {
		t.Errorf("protocol version = %q, want %q", data.ProtocolVersion, "2026-07-28")
	}
}

func TestInitializeBadJSON(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "test-session")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "mcp_initialize_failed") {
		t.Errorf("expected mcp_initialize_failed error, got %v", result.Error)
	}
}

func TestListResourcesFailure(t *testing.T) {
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		switch method {
		case "initialize":
			return http.StatusOK, sdkmcp.InitializeResult{
				ProtocolVersion: "2026-07-28",
				ServerInfo:      &sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"},
				Capabilities:    &sdkmcp.ServerCapabilities{Resources: &sdkmcp.ResourceCapabilities{}},
			}
		case "notifications/initialized":
			return http.StatusAccepted, nil
		case "resources/list":
			return http.StatusInternalServerError, nil
		default:
			return http.StatusNotFound, nil
		}
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String(),
		greet.WithLayerConfig("mcp", &greetmcp.MCPConfig{ListResources: true}),
	)
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	lr, ok := result.Layer("mcp")
	if !ok || lr.Success {
		t.Fatalf("mcp layer should be failed")
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "mcp_list_resources_failed") {
		t.Errorf("expected mcp_list_resources_failed error, got %v", result.Error)
	}

	data := lr.Data.(*greetmcp.MCPResult)
	if data.ServerInfo == nil || data.ServerInfo.Name != "test-server" {
		t.Errorf("server info not populated in partial result")
	}
	if data.Capabilities == nil {
		t.Errorf("capabilities not populated in partial result")
	}
	if data.Resources != nil {
		t.Errorf("resources should be nil in partial result")
	}
}

func TestListProbesNotCalled(t *testing.T) {
	calls := []string{}
	server := runMockServer(t, func(method string, req rpcRequest) (int, any) {
		calls = append(calls, method)
		switch method {
		case "initialize":
			return http.StatusOK, sdkmcp.InitializeResult{ProtocolVersion: "2026-07-28"}
		case "notifications/initialized":
			return http.StatusAccepted, nil
		default:
			return http.StatusNotFound, nil
		}
	})
	defer server.Close()

	engine := all.NewEngine()
	if _, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String()); err != nil {
		t.Fatalf("greet failed: %v", err)
	}

	for _, method := range []string{"resources/list", "tools/list", "prompts/list", "resources/templates/list"} {
		for _, call := range calls {
			if call == method {
				t.Errorf("did not expect %s to be called", method)
			}
		}
	}
}

// --- Auth / Bearer token tests ---

func TestAuthBearerSuccess(t *testing.T) {
	const goodToken = "secret-token"

	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		_ = r.Body.Close()

		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+goodToken {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="The access token is invalid"`)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "test-session")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: sdkmcp.InitializeResult{
				ProtocolVersion: "2026-07-28",
				ServerInfo:      &sdkmcp.Implementation{Name: "auth-server", Version: "1.0.0"},
			},
		})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String(),
		greet.WithLayerConfig("mcp", &greetmcp.MCPConfig{
			BearerToken: goodToken,
		}),
	)
	if err != nil {
		t.Fatalf("greet failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	lr, ok := result.Layer("mcp")
	if !ok || !lr.Success {
		t.Fatalf("mcp layer should be successful")
	}
	data := lr.Data.(*greetmcp.MCPResult)
	if data.ServerInfo == nil || data.ServerInfo.Name != "auth-server" {
		t.Errorf("server info = %+v, want auth-server", data.ServerInfo)
	}
}

func TestAuthBearer401WrongToken(t *testing.T) {
	const goodToken = "secret-token"

	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		_ = r.Body.Close()

		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+goodToken {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="The access token is invalid"`)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "test-session")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  sdkmcp.InitializeResult{ProtocolVersion: "2026-07-28"},
		})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String(),
		greet.WithLayerConfig("mcp", &greetmcp.MCPConfig{
			BearerToken: "wrong-token",
		}),
	)
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	lr, ok := result.Layer("mcp")
	if !ok || lr.Success {
		t.Fatalf("mcp layer should be failed")
	}
	if result.Error == nil {
		t.Fatalf("expected mcp_auth_failed error")
	}

	ge, ok := result.Error.(*greet.GreetError)
	if !ok {
		t.Fatalf("error should be *greet.GreetError, got %T", result.Error)
	}
	if ge.Code != greetmcp.ErrMCPAuthFailed {
		t.Errorf("error code = %q, want %q", ge.Code, greetmcp.ErrMCPAuthFailed)
	}
	if !strings.Contains(ge.Message, "The access token is invalid") {
		t.Errorf("expected error_description in message, got: %s", ge.Message)
	}
}

func TestAuthBearer401WithoutToken(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request", error_description="Missing access token"`)
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	lr, ok := result.Layer("mcp")
	if !ok || lr.Success {
		t.Fatalf("mcp layer should be failed")
	}

	ge, ok := result.Error.(*greet.GreetError)
	if !ok {
		t.Fatalf("error should be *greet.GreetError, got %T", result.Error)
	}
	if ge.Code != greetmcp.ErrMCPAuthFailed {
		t.Errorf("error code = %q, want %q", ge.Code, greetmcp.ErrMCPAuthFailed)
	}
	if !strings.Contains(ge.Message, "Missing access token") {
		t.Errorf("expected error_description in message, got: %s", ge.Message)
	}
}

func TestAuthBearer401WithWellKnown(t *testing.T) {
	// Create the TLS server first so we know its address.
	tlsServer := httptest.NewTLSServer(nil) // placeholder
	tlsAddr := tlsServer.Listener.Addr().String()
	resourceURL := "https://" + tlsAddr + "/"

	// Non-TLS server for the well-known endpoint. The probe uses a fresh
	// http.Client which validates TLS certs, so we point resource_metadata
	// at a plain-HTTP loopback server.
	wellKnown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-protected-resource" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":                 resourceURL,
			"authorization_servers":    []string{"https://auth.example.com"},
			"scopes_supported":         []string{"read", "write"},
			"bearer_methods_supported": []string{"header"},
		})
	}))
	defer wellKnown.Close()

	// Reconfigure the TLS server with the MCP handler.
	tlsServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer error="invalid_request", error_description="Missing access token", resource_metadata="%s/.well-known/oauth-protected-resource"`, wellKnown.URL))
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1})
	})
	defer tlsServer.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", tlsAddr)
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	ge, ok := result.Error.(*greet.GreetError)
	if !ok {
		t.Fatalf("error should be *greet.GreetError, got %T", result.Error)
	}
	if ge.Code != greetmcp.ErrMCPAuthFailed {
		t.Errorf("error code = %q, want %q", ge.Code, greetmcp.ErrMCPAuthFailed)
	}
	if !strings.Contains(ge.Message, "Missing access token") {
		t.Errorf("expected error_description in message, got: %s", ge.Message)
	}
	// The well-known probe should enrich the error with scopes and auth servers.
	if !strings.Contains(ge.Message, "scopes: read, write") {
		t.Errorf("expected enriched scopes in message, got: %s", ge.Message)
	}
	if !strings.Contains(ge.Message, "auth servers: https://auth.example.com") {
		t.Errorf("expected auth servers in message, got: %s", ge.Message)
	}
}

func TestAuthBearer401NoWWWAuthenticate(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	ge, ok := result.Error.(*greet.GreetError)
	if !ok {
		t.Fatalf("error should be *greet.GreetError, got %T", result.Error)
	}
	if ge.Code != greetmcp.ErrMCPAuthFailed {
		t.Errorf("error code = %q, want %q", ge.Code, greetmcp.ErrMCPAuthFailed)
	}
	if !strings.Contains(ge.Message, "no parseable WWW-Authenticate") {
		t.Errorf("expected no-parseable message, got: %s", ge.Message)
	}
}

func TestAuthUnsupportedScheme(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: 1})
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure")
	}

	ge, ok := result.Error.(*greet.GreetError)
	if !ok {
		t.Fatalf("error should be *greet.GreetError, got %T", result.Error)
	}
	if ge.Code != greetmcp.ErrMCPUnsupportedOAuth {
		t.Errorf("error code = %q, want %q", ge.Code, greetmcp.ErrMCPUnsupportedOAuth)
	}
	if !strings.Contains(ge.Message, "unsupported auth scheme") {
		t.Errorf("expected unsupported auth scheme message, got: %s", ge.Message)
	}
	if !strings.Contains(ge.Message, "basic") {
		t.Errorf("expected 'basic' in scheme list, got: %s", ge.Message)
	}
}

func TestHandshakeSSEResponse(t *testing.T) {
	// A server that responds with SSE (text/event-stream) — valid per the
	// MCP Streamable HTTP spec. Greet must extract the JSON-RPC payload
	// from the first non-empty "data:" line.
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Mcp-Session-Id", "sse-session")
		w.WriteHeader(http.StatusOK)

		initResult, _ := json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  sdkmcp.InitializeResult{ProtocolVersion: "2026-07-28"},
		})
		// Real SSE stream with an id: line, an empty data: line, and
		// an event: message line before the actual payload — greet must
		// skip them and find the first non-empty data: line.
		_, _ = w.Write([]byte("id: abc-123\n"))
		_, _ = w.Write([]byte("data:\n"))
		_, _ = w.Write([]byte("event: message\n"))
		_, _ = w.Write([]byte("data: " + string(initResult) + "\n"))
		_, _ = w.Write([]byte("\n"))
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success for SSE response, got error: %v", result.Error)
	}

	lr, ok := result.Layer("mcp")
	if !ok || !lr.Success {
		t.Fatalf("mcp layer should be successful")
	}
	data := lr.Data.(*greetmcp.MCPResult)
	if data.ProtocolVersion != "2026-07-28" {
		t.Errorf("protocol version = %q, want %q", data.ProtocolVersion, "2026-07-28")
	}
}

func TestHandshakeSSEWithLists(t *testing.T) {
	// SSE server with list probes — ensures post() extracts SSE payloads
	// on all requests, not just the initialize.
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var req rpcRequest
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Mcp-Session-Id", "sse-session")
		w.WriteHeader(http.StatusOK)

		var resp any
		switch req.Method {
		case "initialize":
			resp = sdkmcp.InitializeResult{
				ProtocolVersion: "2026-07-28",
				ServerInfo:      &sdkmcp.Implementation{Name: "sse-server", Version: "1.0.0"},
				Capabilities: &sdkmcp.ServerCapabilities{
					Resources: &sdkmcp.ResourceCapabilities{},
					Tools:     &sdkmcp.ToolCapabilities{},
				},
			}
		case "notifications/initialized":
			// No JSON-RPC response body for notifications, just SSE framing.
			_, _ = w.Write([]byte("data:\n"))
			return
		case "resources/list":
			resp = sdkmcp.ListResourcesResult{Resources: []*sdkmcp.Resource{{Name: "r1", URI: "test://r1"}}}
		case "tools/list":
			resp = sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: "t1"}}}
		default:
			http.NotFound(w, r)
			return
		}

		payload, _ := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: resp})
		_, _ = w.Write([]byte("data: " + string(payload) + "\n"))
		_, _ = w.Write([]byte("\n"))
	})
	defer server.Close()

	engine := all.NewEngine()
	result, err := engine.Greet(context.Background(), "mcp", server.Listener.Addr().String(),
		greet.WithLayerConfig("mcp", &greetmcp.MCPConfig{
			ListResources: true,
			ListTools:     true,
		}),
	)
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success for SSE response with lists, got error: %v", result.Error)
	}

	data := result.Layers[len(result.Layers)-1].Data.(*greetmcp.MCPResult)
	if data.ServerInfo == nil || data.ServerInfo.Name != "sse-server" {
		t.Errorf("server info = %+v, want sse-server", data.ServerInfo)
	}
	if len(data.Resources) != 1 || data.Resources[0].Name != "r1" {
		t.Errorf("resources = %+v, want r1", data.Resources)
	}
	if len(data.Tools) != 1 || data.Tools[0].Name != "t1" {
		t.Errorf("tools = %+v, want t1", data.Tools)
	}
}
