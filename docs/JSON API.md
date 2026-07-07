# JSON API

The `jsonapi` package provides JSON-serializable request and response models for the `greet` engine. It is the right choice when you want to expose `greet` over HTTP, accept probe requests via an API, or serialize results for storage or forwarding. The domain models in the core package carry no JSON tags — all JSON shaping lives here, so the wire format can evolve independently of the engine.

## Setup

Import `github.com/crystade/greet/jsonapi/all` to get a `Codec` with every built-in protocol registered:

```go
import (
    japiall "github.com/crystade/greet/jsonapi/all"
    protoall "github.com/crystade/greet/protocols/all"
)

engine := protoall.NewEngine()
codec  := japiall.NewCodec()
```

## Request

`Request` is the JSON-deserializable input for a probe.

| Field | JSON key | Required | Description |
|-------|----------|----------|-------------|
| `Protocol` | `"protocol"` | yes | Registered protocol name, e.g. `"tls"`, `"minecraft"`, `"ssh"`, `"postgresql"`. |
| `Target` | `"target"` | yes | Host or `host:port` to probe. |
| `Options` | `"options"` | no | Per-protocol options keyed by protocol name. |

### Minimal request

```json
{
  "protocol": "tls",
  "target": "example.com:443"
}
```

### Request with options

```json
{
  "protocol": "minecraft",
  "target": "mc.example.com:25565",
  "options": {
    "minecraft": { "protocolVersion": 775 }
  }
}
```

## Response

`Response` is the JSON-serializable result of a probe.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| `Protocol` | `"protocol"` | string | Name of the outermost application layer. |
| `Transport` | `"transport"` | string | Base transport used (`"tcp"` or `"udp"`). |
| `TTDRMs` | `"ttdrMs"` | float64 | Time to DNS resolution, in milliseconds. |
| `RTTMs` | `"rttMs"` | float64 | Time to establish the transport connection, in milliseconds. |
| `Success` | `"success"` | bool | `true` only when every layer succeeded. |
| `Layers` | `"layers"` | `[]Layer` | Per-layer results in stack order. |

### Layer

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| `Name` | `"name"` | string | Layer slug, e.g. `"tcp"`, `"tls"`, `"minecraft"`. |
| `TTFBMs` | `"ttfbMs"` | float64 | Time to first byte from the shared start anchor, in milliseconds. |
| `TTLBMs` | `"ttlbMs"` | float64 | Time to last byte from the shared start anchor, in milliseconds. |
| `Success` | `"success"` | bool | Whether this layer completed without error. |
| `Data` | `"data"` | any | Protocol-specific payload. `null` for transport-only layers (`"tcp"`, `"udp"`). |

All timing values share the same start anchor — see [Timing model](./Core.md#timing-model) in Core.md.

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

## Protocol data schemas

Each protocol defines its own `data` payload within a layer.

### TLS

| JSON key | Type | Description |
|----------|------|-------------|
| `"certChain"` | `[]CertChainEntry` | Ordered certificate chain, leaf first. |
| `"status"` | `[]string` | Overall chain status strings. |

**CertChainEntry:**

| JSON key | Type |
|----------|------|
| `"subject"` | string |
| `"issuer"` | string |
| `"serial"` | string |
| `"notBefore"` | string (RFC3339) |
| `"notAfter"` | string (RFC3339) |
| `"version"` | int |
| `"dnsNames"` | `[]string` |
| `"ipAddresses"` | `[]string` |
| `"isCa"` | bool |
| `"signatureAlgo"` | string |
| `"publicKeyAlgo"` | string |
| `"sha256Fingerprint"` | string |
| `"status"` | `[]string` |

### SSH

| JSON key | Type | Description |
|----------|------|-------------|
| `"versionString"` | string | SSH server version string, e.g. `"SSH-2.0-OpenSSH_9.6"`. |

### HTTP

| JSON key | Type | Description |
|----------|------|-------------|
| `"protocolVersion"` | string | HTTP version from the response status line. |
| `"statusCode"` | int | Numeric HTTP status code. |
| `"status"` | string | Full status text. |
| `"responseHeaders"` | `[]Header` | Response headers in the order received. |
| `"redirects"` | `[]RedirectHop` | Chain of redirects that were followed. |

**Header:** `{ "name": string, "value": string }`

**RedirectHop:**

| JSON key | Type |
|----------|------|
| `"url"` | string |
| `"statusCode"` | int |
| `"method"` | string |
| `"responseHeaders"` | `[]Header` |

### Minecraft

| JSON key | Type | Description |
|----------|------|-------------|
| `"version"` | string | Server version string. |
| `"motd"` | string | Message of the day. |
| `"players"` | int | Current player count. |
| `"maxPlayers"` | int | Maximum player count. |

### PostgreSQL

| JSON key | Type | Description |
|----------|------|-------------|
| `"sslSupported"` | bool | Whether the server supports SSL/TLS. |

## Protocol options

Options are supplied in the `"options"` field of the request, keyed by protocol name.

### Minecraft options

| JSON key | Type | Description |
|----------|------|-------------|
| `"protocolVersion"` | int | Minecraft protocol version (e.g. 775 for 1.20.4). |

### HTTP options

| JSON key | Type | Description |
|----------|------|-------------|
| `"target"` | string | Request path/target. |
| `"method"` | string | HTTP method. |
| `"headers"` | `[]Header` | Additional request headers. |
| `"maxResponseBodyBytes"` | int64 | Max bytes of response body to buffer. |
| `"maxRedirectFollow"` | int | Max redirects to follow. |

### PostgreSQL options

| JSON key | Type | Description |
|----------|------|-------------|
| `"sslMode"` | string | SSL mode: `"prefer"` (default) or `"disable"`. |

TLS and SSH do not accept options.

## Using the codec in Go

### Decoding a request and running a probe

```go
var req jsonapi.Request
if err := json.Unmarshal(raw, &req); err != nil {
    return err
}

opts, err := req.GreetOptions(codec)
if err != nil {
    return err
}

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := engine.Greet(ctx, req.Protocol, req.Target, opts...)
if err != nil {
    // result may be non-nil with partial layer data
}
```

### Encoding a result to JSON

```go
resp := codec.FromGreetResult(result)
out, _ := json.MarshalIndent(resp, "", "  ")
fmt.Println(string(out))
```

`codec.FromGreetResult` returns `nil` when the input is `nil`. Each layer's `data` is mapped through the registered mapper for that layer name; layers without a mapper pass through as-is. Durations are converted to fractional milliseconds.

### End-to-end example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/crystade/greet/jsonapi"
    japiall "github.com/crystade/greet/jsonapi/all"
    protoall "github.com/crystade/greet/protocols/all"
)

func main() {
    engine := protoall.NewEngine()
    codec  := japiall.NewCodec()

    raw := []byte(`{
        "protocol": "tls",
        "target": "example.com:443"
    }`)

    var req jsonapi.Request
    if err := json.Unmarshal(raw, &req); err != nil {
        panic(err)
    }

    opts, err := req.GreetOptions(codec)
    if err != nil {
        panic(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    result, err := engine.Greet(ctx, req.Protocol, req.Target, opts...)
    if err != nil {
        panic(err)
    }

    resp := codec.FromGreetResult(result)
    out, _ := json.MarshalIndent(resp, "", "  ")
    fmt.Println(string(out))
}
```

### Injecting runtime configuration

Some protocol options cannot be serialized to JSON — for example, the HTTP layer's `BodyReader` (`io.Reader`) or `RedirectValidator` (a callback). When you decode a request with `req.GreetOptions(codec)`, it generates a `greet.WithLayerConfig` that replaces the entire layer config. To inject runtime objects into JSON-decoded config, append a custom option that mutates the decoded pointer:

```go
opts, err := req.GreetOptions(codec)
if err != nil {
    panic(err)
}

if _, hasHTTP := req.Options[greethttp.ProtocolName]; hasHTTP {
    // Mutate the JSON-decoded config to add the runtime BodyReader
    opts = append(opts, func(c *greet.GreetConfig) {
        if raw := c.LayerConfig(greethttp.ProtocolName); raw != nil {
            if cfg, ok := raw.(*greethttp.HTTPConfig); ok {
                cfg.BodyReader = strings.NewReader(`{"login":"admin"}`)
                cfg.RedirectValidator = myCustomValidator
            }
        }
    })
} else {
    // No JSON options were provided — construct the config from scratch
    opts = append(opts, greet.WithLayerConfig(greethttp.ProtocolName, &greethttp.HTTPConfig{
        BodyReader:        strings.NewReader(`{"login":"admin"}`),
        RedirectValidator: myCustomValidator,
    }))
}

result, err := engine.Greet(ctx, req.Protocol, req.Target, opts...)
```

---

## Extending the JSON API

This section is for teams adding a custom protocol to their own `greet` service or fork who also want that protocol to be serializable via the JSON API.

For each new protocol, create a subpackage under `jsonapi/` and implement two things: a **data mapper** that converts the domain result struct to a JSON-serializable struct, and (if your protocol accepts options) an **option decoder** that converts `json.RawMessage` into `[]greet.GreetOption`.

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
func FromResult(data greet.LayerData) any {
    r, ok := data.(*myproto.MyProtoResult)
    if !ok || r == nil {
        return data
    }
    return &Data{Foo: r.Foo, Bar: r.Bar}
}
```

### Step 3 (optional): Implement DecodeOptions

```go
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

### Step 4: Register

```go
func Register(c *jsonapi.Codec) {
    c.RegisterDataMapper(myproto.ProtocolName, FromResult)
    c.RegisterOptionDecoder(myproto.ProtocolName, DecodeOptions)
}
```

| Function | Description |
|----------|-------------|
| `c.RegisterDataMapper(name, fn)` | Maps a `greet.LayerData` to a JSON-serializable value. Panics on duplicate name. |
| `c.RegisterOptionDecoder(name, fn)` | Decodes `json.RawMessage` options into `[]greet.GreetOption`. Panics on duplicate name. |

### Step 5: Wire into jsonapi/all

Add a call to your `Register` function inside `jsonapi/all/all.go`:

```go
func Register(c *jsonapi.Codec) {
    // ...existing registrations...
    myproto.Register(c)
}
```

---

## Field naming

All JSON field names use **camelCase**. The domain models carry no JSON tags — the `jsonapi` package is the sole owner of the wire format.
