# Core Library

The `greet` package is a layered protocol handshake engine for Go. Given a protocol name and a target address, it dials the server, runs each protocol layer in sequence, and returns structured timing and data for every layer — without entering a full session or requiring authentication.

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/crystade/greet"
    protoall "github.com/crystade/greet/protocols/all"
)

func main() {
    engine := protoall.NewEngine()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    result, err := engine.Greet(ctx, "tls", "example.com:443")
    if err != nil {
        fmt.Println("greet failed:", err)
        // result may still be non-nil with partial layer data (see Error handling)
    }
    if result != nil {
        fmt.Printf("protocol=%s success=%v rtt=%.2fms\n",
            result.Protocol, result.Success, result.RTT.Seconds()*1000)
        for _, l := range result.Layers {
            fmt.Printf("  layer=%s success=%v ttfb=%.2fms ttlb=%.2fms\n",
                l.Name, l.Success, l.TTFB.Seconds()*1000, l.TTLB.Seconds()*1000)
        }
    }
}
```

## Concepts

### Engine

The `Engine` is the central object. It holds a registry of known protocols and is the entry point for all probes. Create one with `NewEngine()`, register the protocols you need, then call `Greet`.

`protoall.NewEngine()` is the recommended shortcut — it returns an engine with every built-in protocol already registered.

### Transport and application layers

Every protocol stack has exactly one **transport layer** at the base, followed by zero or more **application layers** on top.

- A **transport layer** (`TransportLayer`) owns the network connection. It resolves the host, dials the socket, and returns an open `net.Conn`. The two built-in transports are `tcp` and `udp`.
- An **application layer** (`AppLayer`) performs the protocol-specific handshake over the connection. It receives the `net.Conn` from the layer below, does its exchange, and may return an upgraded connection (for example, TLS wraps the raw TCP socket so subsequent layers like HTTP use the encrypted channel).

### Stacks

A **stack** is a named combination of one transport layer and zero or more application layers. For example:

| Protocol name | Stack |
|---------------|-------|
| `tcp` | TCP only |
| `tls` | TCP → TLS |
| `http` | TCP → HTTP |
| `https` | TCP → TLS → HTTP |
| `mcp` | TCP → TLS → MCP |
| `ssh` | TCP → SSH |
| `minecraft` | TCP → Minecraft |
| `postgresql` | TCP → PostgreSQL |

### Timing model

All timings share a **single start anchor** set just before the transport dial. This means every duration in the result is comparable without adjustment:

```
start
  |------|                    TTDR:  DNS lookup completed
  |------|--|                 RTT:   transport connection established
           |---|              TTFB:  first byte of layer response
           |-------|          TTLB:  last byte of layer response
```

- For transport layers (`tcp`, `udp`): `TTFB = TTLB = RTT`.
- For application layers: `TTFB` is when the first byte of the response arrived, `TTLB` is when the last byte arrived.
- For TLS, `TTFB = TTLB` because the handshake completes atomically.
- When the target is an IP address, `TTDR ≈ 0`.

All durations are `time.Duration`. Use `.Seconds()`, `.Milliseconds()`, or `jsonapi.DurationToMs()` as needed.

## Supported protocols

| Name | Stack | Default port | Description |
|------|-------|-------------|-------------|
| `tcp` | TCP only | 80 | Plain TCP connectivity check. |
| `udp` | UDP only | 53 | UDP probe — sends an empty datagram and waits for any response. |
| `tls` | TCP → TLS | 443 | TLS handshake + full certificate chain inspection. |
| `http` | TCP → HTTP | 80 | HTTP request/response probe with optional redirect following. |
| `https` | TCP → TLS → HTTP | 443 | HTTPS request/response probe. |
| `mcp` | TCP → TLS → MCP | 443 | MCP streamable HTTP initialize handshake + capability probe. |
| `ssh` | TCP → SSH | 22 | Reads the server's SSH version banner. |
| `minecraft` | TCP → Minecraft | 25565 | Minecraft Java Edition handshake + status (version, MOTD, player count). |
| `postgresql` | TCP → PostgreSQL | 5432 | PostgreSQL `SSLRequest` probe — detects SSL support. |

## Setup

### All built-in protocols

```go
import protoall "github.com/crystade/greet/protocols/all"

engine := protoall.NewEngine()
```

### Selected protocols only

Register only what you need for a smaller binary footprint:

```go
import (
    "github.com/crystade/greet"
    "github.com/crystade/greet/protocols/tcp"
    "github.com/crystade/greet/protocols/tls"
)

engine := greet.NewEngine()
engine.RegisterTransport(tcp.New())
engine.RegisterApp(tls.New())
engine.RegisterStack("tls", []string{"tcp", "tls"}, 443, "TLS certificate check")
```

`RegisterStack` names a stack, defines the ordered layer slice, sets a default port, and adds a human-readable description. The layer names in the slice must already be registered as transports or app layers.

To see what's currently registered:

```go
fmt.Println(engine.ListProtocols()) // ["http", "minecraft", "postgresql", "ssh", "tcp", "tls", "udp"]
```

## Greet

`Greet` is the primary API.

```go
func (e *Engine) Greet(ctx context.Context, protocol, target string, opts ...GreetOption) (*GreetResult, error)
```

It resolves the protocol name to a stack, parses the target, and runs the full probe. The `ctx` deadline applies to the entire operation.

**Target formats:** `"host"`, `"host:port"`, `"[::1]:port"`, `"192.168.1.1:443"`. When no port is given, the protocol's default is used.

```go
// Default ports
engine.Greet(ctx, "tls", "example.com")           // → example.com:443
engine.Greet(ctx, "minecraft", "mc.hypixel.net")  // → mc.hypixel.net:25565

// Explicit ports
engine.Greet(ctx, "tls", "example.com:8443")
engine.Greet(ctx, "ssh", "10.0.0.5:2222")

// IPv6
engine.Greet(ctx, "tcp", "[::1]:8080")
```

## Results

### GreetResult

| Field | Type | Description |
|-------|------|-------------|
| `Protocol` | `string` | Name of the outermost application layer (e.g. `"tls"`). For bare TCP this is `"tcp"`. |
| `Transport` | `Transport` | Base transport used — `TransportTCP` (`"tcp"`) or `TransportUDP` (`"udp"`). |
| `TTDR` | `time.Duration` | Time to DNS resolution. Near-zero when the target is already an IP. |
| `RTT` | `time.Duration` | Time to establish the transport connection. |
| `Success` | `bool` | `true` only when **every** layer succeeded. |
| `Layers` | `[]LayerResult` | Per-layer results in stack order (transport first, then each app layer). |
| `Error` | `error` | Set when the probe failed; see [Error handling](#error-handling). |

Use `result.Layer(name)` to look up a specific layer by name without ranging over the slice:

```go
if l, ok := result.Layer("tls"); ok {
    fmt.Println("TLS succeeded:", l.Success)
}
```

### LayerResult

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Layer slug (`"tcp"`, `"tls"`, `"minecraft"`, …). |
| `TTFB` | `time.Duration` | Time to first byte from the shared start anchor. |
| `TTLB` | `time.Duration` | Time to last byte from the shared start anchor. |
| `Success` | `bool` | Whether this layer completed without error. |
| `Data` | `LayerData` | Protocol-specific payload (`nil` for transport layers). |

### Protocol-specific data

Each application protocol provides a typed result that implements `LayerData`. Type-assert `LayerResult.Data` to access it:

| Protocol | Data type | Key fields |
|----------|-----------|------------|
| `tls` | `*tls.TLSResult` | `CertChain []CertChainEntry`, `Status []string` |
| `http` | `*http.HTTPResult` | `StatusCode`, `Status`, `ResponseHeaders`, `Redirects` |
| `mcp` | `*mcp.MCPResult` | `ProtocolVersion`, `ServerInfo`, `Instructions`, `Capabilities`, `Resources`, `ResourceTemplates`, `Tools`, `Prompts` |
| `ssh` | `*ssh.SSHResult` | `VersionString string` |
| `minecraft` | `*minecraft.MinecraftResult` | `Version`, `MOTD`, `Players`, `MaxPlayers` |
| `postgresql` | `*postgresql.PostgreSQLResult` | `SSLSupported bool` |

```go
result, _ := engine.Greet(ctx, "tls", "example.com:443")
for _, l := range result.Layers {
    if l.Name == "tls" {
        tlsResult := l.Data.(*greettls.TLSResult)
        fmt.Printf("leaf cert subject: %s\n", tlsResult.CertChain[0].Subject)
        fmt.Printf("chain status: %v\n", tlsResult.Status)
    }
}
```

## Options

Options are passed as variadic `GreetOption` values to `Greet` or `GreetStack`.

### Timeout per layer

```go
engine.Greet(ctx, "tls", "example.com:443",
    greet.WithLayerTimeout("tls", 2*time.Second),
)
```

The timeout applies to that layer's handshake only. To bound the entire probe, set a deadline on `ctx` with `context.WithTimeout`.

### Config per layer

Pass protocol-specific configuration with `WithLayerConfig`. The config type must implement `LayerConfig`.

```go
engine.Greet(ctx, "minecraft", "mc.example.com",
    greet.WithLayerConfig("minecraft", &minecraft.MinecraftConfig{ProtocolVersion: 775}),
)
```

```go
engine.Greet(ctx, "postgresql", "db.example.com",
    greet.WithLayerConfig("postgresql", &postgresql.PostgreSQLConfig{SSLMode: "disable"}),
)
```

```go
engine.Greet(ctx, "mcp", "mcp.example.com",
    greet.WithLayerConfig("mcp", &mcp.MCPConfig{
        ClientInfo:    mcp.Implementation{Name: "my-client", Version: "1.0.0"},
        ListResources: true,
        ListTools:     true,
    }),
)

// With a custom path (some MCP servers use /mcp instead of /)
engine.Greet(ctx, "mcp", "mcp.example.com",
    greet.WithLayerConfig("mcp", &mcp.MCPConfig{Path: "/mcp"}),
)

// With Bearer token authentication
engine.Greet(ctx, "mcp", "mcp.example.com",
    greet.WithLayerConfig("mcp", &mcp.MCPConfig{
        BearerToken: "sk-abc123",
        ListTools:   true,
    }),
)
```

Available config types:

| Protocol | Config type | Fields |
|----------|-------------|--------|
| `minecraft` | `*minecraft.MinecraftConfig` | `ProtocolVersion int` (default 775) |
| `postgresql` | `*postgresql.PostgreSQLConfig` | `SSLMode string` — `"prefer"` (default) or `"disable"` |
| `http` | `*http.HTTPConfig` | `Target`, `Method`, `Headers`, `MaxResponseBodyBytes`, `MaxRedirectFollow`, `BodyReader`, `RedirectValidator` |
| `mcp` | `*mcp.MCPConfig` | `ClientInfo`, `Path`, `BearerToken`, `ListResources`, `ListTools`, `ListPrompts`, `ListResourceTemplates` |

## Error handling

`Greet` returns a `*greet.GreetError` when the probe fails.

```go
type GreetError struct {
    Code     ErrorCode  // machine-readable category
    Message  string     // human-readable description
    Protocol string     // originating layer name, empty for common errors
    Cause    error      // underlying OS/network error, accessible via errors.Unwrap
}
```

Use `errors.As` to inspect the error:

```go
result, err := engine.Greet(ctx, "tls", "bad.example.com:443")
var ge *greet.GreetError
if errors.As(err, &ge) {
    fmt.Printf("code=%s protocol=%s message=%s\n", ge.Code, ge.Protocol, ge.Message)
}
```

### Error codes

**Common (all protocols):**

| Code | Meaning |
|------|---------|
| `resolve_host_failed` | DNS lookup failed or returned no addresses. |
| `invalid_address` | Malformed target or port out of range. |
| `unknown_protocol` | Protocol name not registered. |
| `invalid_config` | Layer config type assertion failed. |

**TCP (all TCP-based protocols):**

| Code | Meaning |
|------|---------|
| `connection_refused` | Server actively rejected the connection (RST). |
| `connection_timeout` | TCP SYN timed out. |
| `connection_reset` | Connection reset by peer. |
| `network_unreachable` | No route to host. |
| `connection_failed` | Unclassified dial failure. |

**UDP:**

| Code | Meaning |
|------|---------|
| `send_failed` | Datagram could not be sent. |
| `receive_timeout` | No response received within the timeout. |
| `port_unreachable` | ICMP port unreachable. |

**HTTP:**

| Code | Meaning |
|------|---------|
| `http_request_failed` | Invalid target or request build failure. |
| `http_response_failed` | Response or body read/parse failure. |
| `http_timeout` | Read or write timeout during HTTP exchange. |
| `http_redirect_ssrf` | SSRF validator rejection. |
| `http_too_many_redirects` | Redirect limit exceeded. |
| `http_redirect_cross_origin` | Cross-origin or cross-scheme redirect blocked. |

**MCP:**

| Code | Meaning |
|------|---------|
| `mcp_initialize_failed` | Initialize handshake or response parsing failed. |
| `mcp_unexpected_status` | HTTP response status was not 200/202 where expected. |
| `mcp_missing_session` | Initialize response did not include `Mcp-Session-Id`. |
| `mcp_auth_failed` | Bearer authentication failed (401 with parseable WWW-Authenticate). |
| `mcp_unsupported_oauth` | Server returned 401 with an unsupported WWW-Authenticate scheme. |
| `mcp_list_resources_failed` | `resources/list` request failed or returned an error. |
| `mcp_list_tools_failed` | `tools/list` request failed or returned an error. |
| `mcp_list_prompts_failed` | `prompts/list` request failed or returned an error. |
| `mcp_list_templates_failed` | `resources/templates/list` request failed or returned an error. |

### Partial results

When a layer fails, `Greet` returns both an error **and** a partial `*GreetResult`. Layers that completed successfully are fully populated; the failing layer has `Success=false`; layers after it are absent from `result.Layers`.

```go
result, err := engine.Greet(ctx, "tls", "expired.example.com:443")
if err != nil {
    // result.Layers contains [tcp (success=true), tls (success=false)]
    // useful for logging even when the probe failed overall
}
```

## Lower-level API

### GreetStack

```go
func (e *Engine) GreetStack(ctx context.Context, stack *Stack, host string, port int, opts ...GreetOption) *GreetResult
```

Runs the probe directly on a `*Stack` you already have. Errors are embedded in `result.Error` rather than returned separately.

```go
stack, _ := engine.Resolve("tls")
result := engine.GreetStack(ctx, stack, "example.com", 443)
if !result.Success {
    fmt.Println(result.Error)
}
```

### Resolve

```go
func (e *Engine) Resolve(name string) (*Stack, error)
```

Looks up a protocol name in the registry and returns its `*Stack`. Useful when you want to reuse the same resolved stack across many probes.

### ResolveHost

```go
func ResolveHost(ctx context.Context, host string) (*DNSResult, error)
```

Resolves a hostname to an IP address and measures TTDR. Returns immediately (TTDR ≈ 0) when the input is already an IP.

```go
dns, err := greet.ResolveHost(ctx, "example.com")
fmt.Printf("resolved to %s in %.2fms\n", dns.Address, dns.TTDR.Seconds()*1000)
```

### ParseTarget

```go
func ParseTarget(target string, defaultPort int) (host string, port int, err error)
```

Splits a `"host"` or `"host:port"` string into components. When no port is present, `defaultPort` is used.

```go
host, port, err := greet.ParseTarget("example.com:8443", 443)
// host="example.com", port=8443
```

---

## Extending greet

This section is for teams embedding `greet` in their own service who need to add a custom protocol, or for fork maintainers adding new built-in protocols.

The engine is built around two interfaces: `TransportLayer` and `AppLayer`.

### TransportLayer

```go
type TransportLayer interface {
    Name() string
    DefaultPort() int
    Transport() Transport
    Dial(ctx context.Context, start time.Time, host string, port int) (conn net.Conn, ttdr time.Duration, rtt time.Duration, err error)
}
```

- `Name()` — slug used to reference this transport in stacks (e.g. `"tcp"`).
- `DefaultPort()` — fallback when no port is given by the caller.
- `Transport()` — returns `TransportTCP` or `TransportUDP`.
- `Dial()` — resolves host, establishes the connection, and returns the open `net.Conn`. `ttdr` and `rtt` are measured from the shared `start` anchor. The engine closes `conn` after all layers have run.

### AppLayer

```go
type AppLayer interface {
    Name() string
    Description() string
    DefaultPort() int
    BaseTransport() Transport
    Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *GreetConfig) (next net.Conn, data LayerData, timing LayerTiming, err error)
}
```

- `Name()` — slug (e.g. `"tls"`).
- `Description()` — one-line human-readable summary shown by `greet list`.
- `DefaultPort()` — well-known port for this protocol.
- `BaseTransport()` — which transport this layer requires (`TransportTCP` or `TransportUDP`).
- `Handshake()` — performs the application-level exchange over `conn`. Return the upgraded connection as `next` if you wrap the socket (e.g. TLS); return `conn` unchanged otherwise. `data` must implement `LayerData`.

### Registering your layer

```go
engine.RegisterTransport(&MyTransport{})  // for transport layers
engine.RegisterApp(&MyAppLayer{})          // for app layers
// then name a stack that uses it:
engine.RegisterStack("myproto", []string{"tcp", "myproto"}, 1234, "My custom protocol")
```

### Wiring into protocols/all

To include your protocol in `protoall.NewEngine()`, add a call to `Register` inside `protocols/all/all.go`:

```go
func Register(e *greet.Engine) {
    // ...existing registrations...
    myproto.Register(e)
}
```
