# Core

The `greet` package is a layered protocol handshake engine. It resolves a protocol name to a stack of layers (transport + application), dials the transport, runs each application layer in order over the shared connection, and returns a structured result with per-layer timing and data.

## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/crystade/greet"
    _ "github.com/crystade/greet/protocols/all" // register built-in protocols
)

func main() {
    ctx := context.Background()

    result, err := greet.Greet(ctx, "tls", "example.com:443")
    if err != nil {
        fmt.Println("greet failed:", err)
        // result may still be non-nil with partial layers (see error handling below)
    }
    if result != nil {
        fmt.Printf("protocol=%s transport=%s success=%v ttdr=%.2fms rtt=%.2fms\n",
            result.Protocol, result.Transport, result.Success, result.TTDR.Seconds()*1000, result.RTT.Seconds()*1000)
        for _, l := range result.Layers {
            fmt.Printf("  layer=%s success=%v ttfb=%.2fms ttlb=%.2fms\n",
                l.Name, l.Success, l.TTFB.Seconds()*1000, l.TTLB.Seconds()*1000)
        }
    }
}
```

## Supported protocols

| Name          | Stack                  | Default port | Description |
|---------------|------------------------|-------------|-------------|
| `tcp`         | TCP only               | 80          | Plain TCP connectivity check (dial and hang up). |
| `udp`         | UDP only               | 53          | UDP probe — sends an empty datagram and waits for any response. |
| `tls`         | TCP → TLS              | 443         | TLS handshake + full certificate chain inspection with Chrome-style `CERT_` status checks. |
| `http`        | TCP → (TLS) → HTTP     | 80          | HTTP request/response probe. Follows same-origin redirects and buffers body. |
| `ssh`         | TCP → SSH              | 22          | Reads the server's SSH version banner (anonymous — no client banner sent). |
| `minecraft`   | TCP → Minecraft        | 25565       | Minecraft Java Edition handshake + status request (version, MOTD, player count). |
| `postgresql`  | TCP → PostgreSQL       | 5432        | PostgreSQL `SSLRequest` probe — detects whether the server supports SSL. |

## Registration

Protocols register themselves via `init()`. To use the built-in protocols, blank-import them:

```go
import _ "github.com/crystade/greet/protocols/all"
```

This pulls in `tcp`, `udp`, `tls`, `ssh`, `minecraft`, and `postgresql`. Import individual packages for a smaller footprint:

```go
import (
    _ "github.com/crystade/greet/protocols/tcp"
    _ "github.com/crystade/greet/protocols/tls"
)
```

Use `greet.ListProtocols()` to see what's currently registered:

```go
fmt.Println(greet.ListProtocols()) // [minecraft postgresql ssh tcp tls udp]
```

## Primary API: `Greet`

```go
func Greet(ctx context.Context, protocol, target string, opts ...GreetOption) (*GreetResult, error)
```

`Greet` is the primary entry point. It:

1. Resolves `protocol` to a layer stack via `Resolve()`.
2. Parses `target` into host and port. If no port is given, the protocol's default is used.
3. Delegates to `GreetStack()`.

**Target formats:** `"host"`, `"host:port"`, `"[::1]:port"`, `"192.168.1.1:443"`.

```go
// Use default ports
greet.Greet(ctx, "tls", "example.com")       // → example.com:443
greet.Greet(ctx, "minecraft", "mc.hypixel.net") // → mc.hypixel.net:25565

// Explicit ports
greet.Greet(ctx, "tls", "example.com:8443")
greet.Greet(ctx, "ssh", "10.0.0.5:2222")

// IPv6
greet.Greet(ctx, "tcp", "[::1]:8080")
```

## `GreetResult` and `LayerResult`

### GreetResult

| Field       | Type            | Description |
|-------------|-----------------|-------------|
| `Protocol`  | `string`        | Name of the outermost application layer (e.g. `"tls"`). For bare TCP this is `"tcp"`. |
| `Transport` | `Transport`     | Base transport used — `TransportTCP` (`"tcp"`) or `TransportUDP` (`"udp"`). |
| `TTDR`      | `time.Duration` | Time to DNS resolution. Near-zero when target is already an IP. |
| `RTT`       | `time.Duration` | Time to establish the transport connection. |
| `Success`   | `bool`          | `true` only when **every** layer succeeded. |
| `Layers`    | `[]LayerResult` | Per-layer results in stack order (transport first, then each app layer). |

### LayerResult

| Field     | Type            | Description |
|-----------|-----------------|-------------|
| `Name`    | `string`        | Layer slug (`"tcp"`, `"tls"`, `"minecraft"`, …). |
| `TTFB`    | `time.Duration` | Time to first byte from the shared start anchor. |
| `TTLB`    | `time.Duration` | Time to last byte from the shared start anchor. |
| `Success` | `bool`          | Whether this layer completed without error. |
| `Data`    | `any`           | Protocol-specific payload (`nil` for transport layers). |

### Protocol-specific Data types

Each protocol provides a typed result:

| Protocol     | Data type                     | Key fields |
|-------------|-------------------------------|------------|
| `tls`       | `*tls.TLSResult`              | `CertChain []CertChainEntry`, `Status []string` |
| `http`      | `*http.HTTPResult`            | `StatusCode`, `Status`, `ResponseHeaders`, `Redirects` |
| `ssh`       | `*ssh.SSHResult`              | `VersionString string` |
| `minecraft` | `*minecraft.MinecraftResult`  | `Version`, `MOTD`, `Players`, `MaxPlayers` |
| `postgresql`| `*postgresql.PostgreSQLResult`| `SSLSupported bool` |

Type-assert `LayerResult.Data` to access the protocol-specific payload:

```go
result, _ := greet.Greet(ctx, "tls", "example.com:443")
for _, l := range result.Layers {
    if l.Name == "tls" {
        tlsResult := l.Data.(*tls.TLSResult)
        fmt.Printf("leaf cert subject: %s\n", tlsResult.CertChain[0].Subject)
        fmt.Printf("chain status: %v\n", tlsResult.Status)
    }
}
```

## Options

### Timeout

Override the default 5-second timeout:

```go
greet.Greet(ctx, "tls", "example.com:443", greet.WithTimeout(10*time.Second))
```

Always prefer the context deadline over timeouts for cancellation. The engine wraps the context with `WithTimeout` internally — any deadline you set on the incoming context becomes the effective cap.

### Per-layer config

Pass protocol-specific configuration via `WithLayerConfig`:

```go
greet.Greet(ctx, "minecraft", "mc.example.com",
    greet.WithLayerConfig("minecraft", &minecraft.MinecraftConfig{ProtocolVersion: 775}),
)
```

```go
greet.Greet(ctx, "postgresql", "db.example.com",
    greet.WithLayerConfig("postgresql", &postgresql.PostgreSQLConfig{SSLMode: "disable"}),
)
```

Each layer retrieves its config inside `Handshake` via `cfg.LayerConfig(name)` and type-asserts it.

| Protocol     | Config type                       | Fields |
|-------------|-----------------------------------|--------|
| `minecraft` | `*minecraft.MinecraftConfig`      | `ProtocolVersion int` (default 775) |
| `postgresql`| `*postgresql.PostgreSQLConfig`    | `SSLMode string` (`"prefer"` or `"disable"`) |
| `http`      | `*http.HTTPConfig`                | `Target`, `Method`, `Headers`, `MaxResponseBodyBytes`, `MaxRedirectFollow` |

## Error handling

All errors returned by the engine are `*greet.GreetError`:

```go
type GreetError struct {
    Code     ErrorCode  // machine-readable category
    Message  string     // human-readable description
    Protocol string     // originating protocol name, empty for common errors
    Cause    error      // underlying OS/network error, accessible via errors.Unwrap
}
```

Use `errors.As` or `errors.Is` for classification:

```go
result, err := greet.Greet(ctx, "tls", "bad:host:443")
var ge *greet.GreetError
if errors.As(err, &ge) {
    fmt.Printf("code=%s protocol=%s message=%s\n", ge.Code, ge.Protocol, ge.Message)
}
```

### Error codes

**Common (all protocols):**

| Code                    | Meaning |
|-------------------------|---------|
| `resolve_host_failed`   | DNS lookup failed or returned no addresses. |
| `invalid_address`       | Malformed target or port out of range. |
| `unknown_protocol`      | Protocol name not registered. |
| `invalid_config`        | Layer config type assertion failed. |

**TCP errors (all TCP-based protocols):**

| Code                   | Meaning |
|------------------------|---------|
| `connection_refused`   | Server actively rejected the connection (RST). |
| `connection_timeout`   | TCP SYN timed out. |
| `connection_reset`     | Connection reset by peer. |
| `network_unreachable`  | No route to host. |
| `connection_failed`    | Unclassified dial failure. |

**UDP errors:**

| Code                 | Meaning |
|----------------------|---------|
| `send_failed`        | Datagram could not be sent. |
| `receive_timeout`    | No response received within the timeout. |
| `port_unreachable`   | ICMP port unreachable. |

**HTTP errors:**

| Code                         | Meaning |
|------------------------------|---------|
| `http_request_failed`        | Invalid target or request build failure. |
| `http_response_failed`       | Response or body read/parse failure. |
| `http_timeout`               | Read or write timeout during HTTP exchange. |
| `http_redirect_ssrf`         | SSRF validator rejection. |
| `http_too_many_redirects`    | Redirect limit exceeded. |
| `http_redirect_cross_origin` | Cross-origin/cross-scheme redirect blocked. |

### Partial results on error

When a layer fails, `Greet` returns a partial `*GreetResult` along with the error. Previous layers are fully populated; the failing layer has `Success=false`, and subsequent layers are absent.

```go
result, err := greet.Greet(ctx, "tls", "expired.example.com:443")
if err != nil {
    // result.Layers will contain [tcp (success), tls (failed)]
    // Useful for logging even when the overall probe was unsuccessful.
}
```

## Timing model

All timing is anchored to a **single start time** (`time.Now()` just before the transport dial):

```
|------|                    TTDR:  DNS resolved
|------|--|                 RTT:   transport connection established
        |---|               TTFB:  first byte of layer response
        |-------|           TTLB:  last byte of layer response
```

- For transport layers (`tcp`, `udp`): `TTFB = TTLB = RTT`.
- For application layers: `TTFB` is the first application byte received, `TTLB` is the last.
- For TLS, `TTFB = TTLB` because the handshake is atomic.

All durations are `time.Duration` — use `.Seconds()`, `.Milliseconds()`, or `jsonapi.DurationToMs()` as needed.

## Lower-level API

### `GreetStack`

```go
func GreetStack(ctx context.Context, stack *Stack, host string, port int, opts ...GreetOption) (*GreetResult, error)
```

Use `GreetStack` when you already have a `*Stack` — e.g. from a custom resolution or when reusing the same stack for multiple targets.

```go
stack, _ := greet.Resolve("tls")
result, _ := greet.GreetStack(ctx, stack, "example.com", 443)
```

### `Resolve`

```go
func Resolve(name string) (*Stack, error)
```

Resolves a protocol name to its `Stack` (transport + app layers). `"tcp"` resolves to a transport-only stack; `"tls"` resolves to `[tcp, tls]`.

### `ResolveHost`

```go
func ResolveHost(ctx context.Context, host string) (*DNSResult, error)
```

Resolves a hostname to an IP address and measures TTDR. Returns immediately (TTDR ≈ 0) if the host is already an IP address.

```go
dns, err := greet.ResolveHost(ctx, "example.com")
fmt.Printf("resolved %s in %.2fms\n", dns.Address, dns.TTDR.Seconds()*1000)
```

### `ParseTarget`

```go
func ParseTarget(target string, defaultPort int) (host string, port int, err error)
```

Splits a target string into host and port. Use when you need to pre-parse targets before calling `GreetStack`:

```go
host, port, err := greet.ParseTarget("example.com:8443", 443)
// host="example.com", port=8443
```

## Adding a new protocol

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

- `Name()` — returns the slug (e.g. `"tcp"`, `"udp"`).
- `DefaultPort()` — fallback port when none is specified.
- `Transport()` — returns `TransportTCP` or `TransportUDP`.
- `Dial()` — resolves host, establishes the connection, returns the open `net.Conn` with TTDR and RTT measured relative to the shared `start` anchor. The caller closes `conn`.

### AppLayer

```go
type AppLayer interface {
    Name() string
    Description() string
    DefaultPort() int
    BaseTransport() Transport
    Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *GreetConfig) (data any, timing LayerTiming, err error)
}
```

- `Name()` — protocol slug, e.g. `"tls"`.
- `Description()` — one-line human-readable summary.
- `DefaultPort()` — well-known port.
- `BaseTransport()` — which transport this app layer requires.
- `Handshake()` — performs the application-level exchange over `conn`. `start` is shared across the whole stack — TTFB/TTLB must use `time.Since(start)`. Retrieve per-layer config via `cfg.LayerConfig(name)`.

### Registration

In your package's `init()`, call the appropriate registration function:

```go
func init() {
    greet.RegisterTransport(&MyTransport{})  // for transport layers
    greet.RegisterApp(&MyAppLayer{})          // for app layers
}
```

### Optional: CLI flags

If your layer supports CLI flags, implement the `FlaggedLayer` interface:

```go
type FlaggedLayer interface {
    RegisterFlags(fs *flag.FlagSet)
    ParseFlags(fs *flag.FlagSet) ([]GreetOption, error)
}
```

The CLI auto-discovers this via type assertion. See `protocols/minecraft/minecraft.go` and `protocols/postgresql/postgresql.go` for examples.

### Wire into `protocols/all`

Add a blank import to `protocols/all/all.go` so that `import _ ".../protocols/all"` pulls in your protocol:

```go
import (
    _ "github.com/crystade/greet/protocols/myproto"
)
```
