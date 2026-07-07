# JSON API

The `jsonapi` package provides JSON-serializable request and response models for the **greet** engine, together with mappers that convert to and from the domain models. The domain models themselves carry no JSON tags — all JSON shaping lives here so the wire format can evolve independently of the core engine.

## Package structure

```
jsonapi/
  jsonapi.go          ← Request, Response, Layer types + mapper registry
  all/
    all.go            ← blank-imports every protocol subpackage to trigger init()
  minecraft/
    minecraft.go      ← Data, Options, FromResult, DecodeOptions
  postgresql/
    postgresql.go     ← Data, Options, FromResult, DecodeOptions
  ssh/
    ssh.go            ← Data, FromResult
  tls/
    tls.go            ← Data, CertChainEntry, FromResult
```

The root package `jsonapi` holds the core types and a registry of data mappers and option decoders. Each protocol subpackage registers its own mapper (and optionally an option decoder) via `init()`.

Import `github.com/crystade/greet/jsonapi/all` as a side-effect import to opt into all built-in protocol mappers:

```go
import _ "github.com/crystade/greet/jsonapi/all"
```

## Request

`Request` is the JSON-serializable input for a greet operation.

| Field      | Type                          | JSON key      | Required | Description |
|------------|-------------------------------|---------------|----------|-------------|
| `Protocol` | `string`                      | `"protocol"`  | yes      | Registered protocol name, e.g. `"tls"`, `"minecraft"`, `"ssh"`, `"postgresql"`. |
| `Target`   | `string`                      | `"target"`    | yes      | Host or `host:port` to probe. |
| `TimeoutMs`| `int64`                       | `"timeoutMs"` | no       | Handshake timeout in milliseconds. Zero/negative means use the engine default (5 s). |
| `Options`  | `map[string]json.RawMessage`  | `"options"`   | no       | Per-protocol options keyed by protocol name. Each value is decoded by the option decoder registered for that protocol. |

### Minimal request

```json
{
  "protocol": "tls",
  "target": "example.com:443"
}
```

### Request with timeout

```json
{
  "protocol": "minecraft",
  "target": "mc.example.com:25565",
  "timeoutMs": 3000
}
```

### Request with protocol options

```json
{
  "protocol": "minecraft",
  "target": "mc.example.com:25565",
  "timeoutMs": 3000,
  "options": {
    "minecraft": { "protocolVersion": 775 }
  }
}
```

### Method: `GreetOptions()`

`Request` exposes a `GreetOptions()` method that converts the JSON request into engine `greet.GreetOption` values:

```go
func (r *Request) GreetOptions() ([]greet.GreetOption, error)
```

- If `TimeoutMs > 0`, a `greet.WithTimeout(...)` option is produced.
- Each entry in `r.Options` is dispatched to the registered `OptionDecoder` for that protocol name.
- An unknown protocol key in `Options` returns an error.

## Response

`Response` is the JSON-serializable outcome of a greet operation.

| Field       | Type      | JSON key     | Description |
|-------------|-----------|--------------|-------------|
| `Protocol`  | `string`  | `"protocol"` | Name of the outermost application layer. |
| `Transport` | `string`  | `"transport"`| Base transport used (`"tcp"` or `"udp"`). |
| `TTDRMs`    | `float64` | `"ttdrMs"`   | Time to DNS resolution, in milliseconds. |
| `RTTMs`     | `float64` | `"rttMs"`    | Time to establish the transport connection, in milliseconds. |
| `Success`   | `bool`    | `"success"`  | `true` only when every layer in the stack succeeded. |
| `Layers`    | `[]Layer` | `"layers"`   | Per-layer results in stack order. |

### Layer

| Field     | Type      | JSON key     | Description |
|-----------|-----------|--------------|-------------|
| `Name`    | `string`  | `"name"`     | Layer slug, e.g. `"tcp"`, `"tls"`, `"minecraft"`. |
| `TTFBMs`  | `float64` | `"ttfbMs"`   | Time to first byte from the shared start anchor, in milliseconds. |
| `TTLBMs`  | `float64` | `"ttlbMs"`   | Time to last byte from the shared start anchor, in milliseconds. |
| `Success` | `bool`    | `"success"`  | Whether this layer completed without error. |
| `Data`    | `any`     | `"data"`     | Protocol-specific payload. `null` for transport-only layers (`"tcp"`, `"udp"`). |

### Timing model

```
|------|                    TTDR: DNS resolved
|------|--|                 RTT:  transport connection established
        per-layer TTFB/TTLB tracked in Layers[i]
```

All timing values share the same start anchor.

### Example response (TLS)

```json
{
  "protocol": "tls",
  "transport": "tcp",
  "ttdrMs": 12.4,
  "rttMs": 35.1,
  "success": true,
  "layers": [
    {
      "name": "tcp",
      "ttfbMs": 35.1,
      "ttlbMs": 35.1,
      "success": true
    },
    {
      "name": "tls",
      "ttfbMs": 67.3,
      "ttlbMs": 72.8,
      "success": true,
      "data": {
        "certChain": [
          {
            "subject": "CN=example.com",
            "issuer": "CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1",
            "serial": "0abc...",
            "notBefore": "2025-01-15T00:00:00Z",
            "notAfter": "2026-02-15T23:59:59Z",
            "version": 3,
            "dnsNames": ["example.com", "www.example.com"],
            "isCa": false,
            "signatureAlgo": "SHA256-RSA",
            "publicKeyAlgo": "RSA",
            "sha256Fingerprint": "a1b2c3...",
            "status": ["ok"]
          }
        ],
        "status": ["ok"]
      }
    }
  ]
}
```

## Protocol-specific data types

Each protocol subpackage defines its own `Data` struct for the JSON payload within a `Layer.Data` field.

### TLS (`jsonapi/tls`)

| Field          | JSON key              | Type               | Description |
|----------------|-----------------------|--------------------|-------------|
| `CertChain`    | `"certChain"`         | `[]CertChainEntry` | Ordered list of certificates. |
| `Status`       | `"status"`            | `[]string`         | Per-certificate status strings. |

**CertChainEntry:**

| Field              | JSON key              | Type       |
|--------------------|-----------------------|------------|
| `Subject`          | `"subject"`           | `string`   |
| `Issuer`           | `"issuer"`            | `string`   |
| `Serial`           | `"serial"`            | `string`   |
| `NotBefore`        | `"notBefore"`         | `string`   |
| `NotAfter`         | `"notAfter"`          | `string`   |
| `Version`          | `"version"`           | `int`      |
| `DNSNames`         | `"dnsNames"`          | `[]string` |
| `IPAddresses`      | `"ipAddresses"`       | `[]string` |
| `IsCA`             | `"isCa"`              | `bool`     |
| `SignatureAlgo`    | `"signatureAlgo"`     | `string`   |
| `PublicKeyAlgo`    | `"publicKeyAlgo"`     | `string`   |
| `SHA256Fingerprint`| `"sha256Fingerprint"` | `string`   |
| `Status`           | `"status"`            | `[]string` |

### SSH (`jsonapi/ssh`)

| Field           | JSON key           | Type     | Description |
|-----------------|--------------------|----------|-------------|
| `VersionString` | `"versionString"`  | `string` | SSH server version string, e.g. `"SSH-2.0-OpenSSH_9.6"`. |

### HTTP (`jsonapi/http`)

| Field             | JSON key          | Type            | Description |
|-------------------|-------------------|-----------------|-------------|
| `ProtocolVersion` | `"protocolVersion"`| `string`        | The HTTP version from the response status line. |
| `StatusCode`      | `"statusCode"`    | `int`           | The numeric HTTP status code. |
| `Status`          | `"status"`        | `string`        | The full status text. |
| `ResponseHeaders` | `"responseHeaders"`| `[]Header`      | The response headers in the order received. |
| `Redirects`       | `"redirects"`     | `[]RedirectHop` | The chain of redirects that were followed. |

**Header:**

| Field   | JSON key | Type     |
|---------|----------|----------|
| `Name`  | `"name"` | `string` |
| `Value` | `"value"`| `string` |

**RedirectHop:**

| Field             | JSON key          | Type       |
|-------------------|-------------------|------------|
| `URL`             | `"url"`           | `string`   |
| `StatusCode`      | `"statusCode"`    | `int`      |
| `Method`          | `"method"`        | `string`   |
| `ResponseHeaders` | `"responseHeaders"`| `[]Header` |

### Minecraft (`jsonapi/minecraft`)

| Field        | JSON key        | Type     | Description |
|--------------|-----------------|----------|-------------|
| `Version`    | `"version"`     | `string` | Server version string. |
| `MOTD`       | `"motd"`        | `string` | Message of the day. |
| `Players`    | `"players"`     | `int`    | Current player count. |
| `MaxPlayers` | `"maxPlayers"`  | `int`    | Maximum player count. |

### PostgreSQL (`jsonapi/postgresql`)

| Field          | JSON key          | Type   | Description |
|----------------|-------------------|--------|-------------|
| `SSLSupported` | `"sslSupported"`  | `bool` | Whether the server supports SSL/TLS. |

## Protocol options

Protocols can expose JSON-configurable options that are decoded into engine `GreetOption` values.

### Minecraft options

| Field              | JSON key            | Type  | Description |
|--------------------|---------------------|-------|-------------|
| `ProtocolVersion`  | `"protocolVersion"` | `int` | Minecraft protocol version number (e.g. 775 for 1.20.4). |

### HTTP options

| Field                  | JSON key               | Type       | Description |
|------------------------|------------------------|------------|-------------|
| `Target`               | `"target"`             | `string`   | Request path/target. |
| `Method`               | `"method"`             | `string`   | HTTP method. |
| `Headers`              | `"headers"`            | `[]Header` | Additional request headers to send. |
| `MaxResponseBodyBytes` | `"maxResponseBodyBytes"`| `int64`    | Max bytes of response body to buffer. |
| `MaxRedirectFollow`    | `"maxRedirectFollow"`  | `int`      | Max redirects to follow. |

### PostgreSQL options

| Field     | JSON key    | Type     | Description |
|-----------|-------------|----------|-------------|
| `SSLMode` | `"sslMode"` | `string` | SSL mode (`"disable"`, `"allow"`, `"prefer"`, `"require"`). |

TLS and SSH protocols do not accept options.

## Converting domain results to JSON

Use `jsonapi.FromGreetResult` to map a domain `*greet.GreetResult` into a JSON-serializable `*Response`:

```go
result, err := greet.Greet(ctx, "tls", "example.com:443", opts...)
// handle err
resp := jsonapi.FromGreetResult(result)
jsonBytes, _ := json.Marshal(resp)
```

- Returns `nil` when the input `*greet.GreetResult` is `nil`.
- Each layer's `Data` is mapped through the registered `DataMapper` for that layer name.
- Layers with no registered mapper (or `nil` data) pass through as-is.
- Durations are converted to fractional milliseconds via `jsonapi.DurationToMs`.

## Integrating a new protocol

To add JSON API support for a new protocol, create a subpackage under `jsonapi/` and register your types in `init()`.

### Step 1: Define your JSON data model

```go
// jsonapi/myproto/myproto.go
package myproto

import (
    "github.com/crystade/greet/jsonapi"
    "github.com/crystade/greet/protocols/myproto"
)

type Data struct {
    Foo string `json:"foo"`
    Bar int    `json:"bar"`
}
```

### Step 2: Implement FromResult

```go
func FromResult(data any) any {
    r, ok := data.(*myproto.MyProtoResult)
    if !ok || r == nil {
        return data
    }
    return &Data{Foo: r.Foo, Bar: r.Bar}
}
```

### Step 3 (optional): Implement DecodeOptions

If your protocol accepts per-request options:

```go
import (
    "encoding/json"
    "github.com/crystade/greet"
)

type Options struct {
    Qux string `json:"qux"`
}

func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
    var o Options
    if err := json.Unmarshal(raw, &o); err != nil {
        return nil, err
    }
    return []greet.GreetOption{
        greet.WithLayerConfig(myproto.ProtocolName, &myproto.Config{Qux: o.Qux}),
    }, nil
}
```

### Step 4: Register in init()

```go
func init() {
    jsonapi.RegisterDataMapper(myproto.ProtocolName, FromResult)
    jsonapi.RegisterOptionDecoder(myproto.ProtocolName, DecodeOptions)
}
```

### Step 5: Wire into `jsonapi/all`

Add your import to `jsonapi/all/all.go`:

```go
import (
    // ...existing imports...
    _ "github.com/crystade/greet/jsonapi/myproto"
)
```

### API reference for registration

| Function                                        | Description |
|-------------------------------------------------|-------------|
| `jsonapi.RegisterDataMapper(name, DataMapper)`  | Register a `DataMapper` for a protocol layer. Panics on duplicate name (case-insensitive). |
| `jsonapi.RegisterOptionDecoder(name, OptionDecoder)` | Register an `OptionDecoder` for a protocol. Panics on duplicate name (case-insensitive). |

**DataMapper** signature:
```go
type DataMapper func(data any) any
```

**OptionDecoder** signature:
```go
type OptionDecoder func(raw json.RawMessage) ([]greet.GreetOption, error)
```

## End-to-end integration

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/crystade/greet"
    _ "github.com/crystade/greet/jsonapi/all"
    "github.com/crystade/greet/jsonapi"
    _ "github.com/crystade/greet/protocols/all"
)

func main() {
    raw := []byte(`{
        "protocol": "tls",
        "target": "example.com:443"
    }`)

    var req jsonapi.Request
    if err := json.Unmarshal(raw, &req); err != nil {
        panic(err)
    }

    opts, err := req.GreetOptions()
    if err != nil {
        panic(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), greet.DefaultTimeout)
    defer cancel()

    result, err := greet.Greet(ctx, req.Protocol, req.Target, opts...)
    if err != nil {
        panic(err)
    }

    resp := jsonapi.FromGreetResult(result)
    out, _ := json.MarshalIndent(resp, "", "  ")
    fmt.Println(string(out))
}
```

## Injecting runtime configuration

Some protocol configurations require runtime objects that cannot be serialized to JSON, such as the HTTP layer's `BodyReader` (`io.Reader`) or `RedirectValidator` (callback function).

When using `req.GreetOptions()` to decode JSON options, it generates a `greet.WithLayerConfig` that completely overwrites any previous configuration for that layer. To inject runtime objects into a JSON-derived configuration, you must append a custom `GreetOption` that mutates the decoded pointer.

```go
opts, err := req.GreetOptions()
if err != nil {
    panic(err)
}

// Check if the JSON request included HTTP options
if _, hasHTTP := req.Options[greethttp.ProtocolName]; hasHTTP {
    // If yes, append a custom option to mutate the JSON-decoded config pointer
    opts = append(opts, func(c *greet.GreetConfig) {
        if raw := c.LayerConfig(greethttp.ProtocolName); raw != nil {
            if cfg, ok := raw.(*greethttp.HTTPConfig); ok {
                cfg.BodyReader = strings.NewReader(`{"login":"admin"}`)
                cfg.RedirectValidator = myCustomValidator
            }
        }
    })
} else {
    // If no JSON options were provided, use the standard option
    opts = append(opts, greet.WithLayerConfig(greethttp.ProtocolName, &greethttp.HTTPConfig{
        BodyReader: strings.NewReader(`{"login":"admin"}`),
        RedirectValidator: myCustomValidator,
    }))
}

// Run the greet operation with the merged options
result, err := greet.Greet(ctx, req.Protocol, req.Target, opts...)
```

## Field naming convention

All JSON field names use **camelCase**. The domain models carry no JSON tags — the `jsonapi` package is the sole owner of the wire format.
