# CLI

The `greet` CLI probes remote servers across layered protocol stacks and reports per-layer timing and data. It supports human-readable and JSON output, and auto-discovers protocol-specific flags from registered layers.

## Installation

```bash
go install github.com/crystade/greet/cmd/greet@latest
```

Or build from source:

```bash
cd cmd/greet
go build -o greet .
```

## Quick start

```bash
# Check if a TLS server is reachable and inspect its certificate
greet tls example.com

# Check an SSH server on a non-standard port
greet ssh 10.0.0.5:2222

# Check if a Minecraft server is online
greet minecraft mc.hypixel.net
```

## Usage

```
greet <protocol> <host>[:<port>] [flags]
```

The protocol name must be one of the registered protocols (see [Supported protocols](./Core.md#supported-protocols) in Core.md). The target is `host` or `host:port`. When no port is given, the protocol's default is used.

Flags must appear **after** the protocol and target:

```bash
# Correct
greet minecraft mc.example.com --protocol-version 766 --timeout 3s

# Also correct — flags can be interspersed after the target
greet tls example.com:8443 --output-format json --timeout 10s

# Wrong — flags before the target will be misinterpreted
greet tls --timeout 3s example.com   ❌
```

### IPv6

Use bracket notation for IPv6 addresses:

```bash
greet tcp [::1]:8080
greet tls [2606:4700:4700::1111]:443
```

## Flags

### Global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--timeout` | duration | `5s` | Total operation timeout, including DNS. Accepts `3s`, `1m`, `500ms`, etc. |
| `--output-format` | string | `"default"` | Output format: `"default"` (human-readable) or `"json"`. |

### Protocol-specific flags

Protocol layers can expose CLI flags by registering `flagHandler` entries in the CLI's `init()` registry. Each protocol has a dedicated `flags_*.go` file in `cmd/greet/` that registers and parses its flags into `GreetOption` values.

#### HTTP

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--method` | string | `"GET"` | HTTP method (e.g. GET, POST, HEAD). |
| `--path` | string | `"/"` | Request path and optional query (e.g. `/users?sort=name`). |
| `--max-body` | int | 1 MiB | Maximum response body bytes to buffer. |
| `--max-redirects` | int | `0` | Maximum redirects to follow (0 = none). |
| `--header` | string | | Request header in `Name:Value` form; may be repeated. |

```bash
greet http example.com --method POST --path /login --max-redirects 3 --header "Content-Type:application/json"
```

#### Minecraft

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--protocol-version` | int | `775` | Minecraft protocol version (e.g. `775` for 1.20.4, `766` for 1.20.2). |

```bash
greet minecraft mc.example.com --protocol-version 766
```

#### PostgreSQL

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--sslmode` | string | `"prefer"` | SSL mode: `"prefer"` (try SSL) or `"disable"` (skip SSL probe). |

```bash
greet postgresql db.example.com --sslmode disable
```

#### MCP

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--path` | string | `"/"` | Request path for MCP streamable HTTP (e.g. `/mcp`). |
| `--bearer-token` | string | `""` | Bearer token for MCP Authorization header. |
| `--list-resources` | bool | `false` | List server resources after the handshake. |
| `--list-tools` | bool | `false` | List server tools after the handshake. |
| `--list-prompts` | bool | `false` | List server prompts after the handshake. |
| `--list-resource-templates` | bool | `false` | List resource templates after the handshake. |

```bash
greet mcp mcp.example.com --list-tools --list-resources
greet mcp mcp.example.com --path /mcp
greet mcp mcp.example.com --bearer-token "sk-abc123" --list-tools
```

#### TLS

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server-name` | string | (target host) | TLS SNI hostname override. |
| `--client-cert` | string | | Path to PEM client certificate for mTLS (key may be bundled). |
| `--client-key` | string | | Path to PEM client private key (default: bundled in `--client-cert`). |
| `--ca-cert` | string | | Path to PEM CA certificate for trust verification. |
| `--insecure` | bool | `true` | Skip certificate verification. |

```bash
greet tls internal.example.com --server-name public.example.com --insecure
greet tls mtls.example.com --client-cert cert.pem --client-key key.pem --insecure=false
```

## Commands

### List registered protocols

```bash
greet list
```

Output:

```
Available protocols:
  http           :80    HTTP request/response probe
  mcp            :443   MCP streamable HTTP initialize handshake + capability probe
  minecraft      :25565 Minecraft Java Edition handshake + status request
  postgresql     :5432  PostgreSQL SSLRequest probe
  ssh            :22    SSH version banner exchange
  tcp            :80    Raw TCP connectivity probe
  tls            :443   TLS certificate check
  udp            :53    UDP connectivity probe
```

### Help

```bash
greet --help
greet -h
```

## Output

### Human-readable (default)

Shows protocol-level metrics followed by each layer's timing and protocol-specific data.

**TLS:**

```bash
$ greet tls example.com
```

```
Protocol:  tls
Transport: tcp
TTDR:      6.5219ms
RTT:       18.6273ms
Success:   true

[tcp]
  TTFB:    18.6273ms
  TTLB:    18.6273ms
  Success: true

[tls]
  TTFB:    156.8465ms
  TTLB:    156.8465ms
  Success: true
  Subject: CN=www.example.org
  Issuer: CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1
  Serial: 0abc...
  Not Before: 2025-01-15T00:00:00Z
  Not After: 2026-02-15T23:59:59Z
  Version: 3
  DNS Names: [www.example.org example.com]
  Signature Algorithm: SHA256-RSA
  Public Key Algorithm: RSA
  Fingerprint (SHA-256): a1b2c3...
  Chain Status: ok

  Certificate Chain (3 certificates, leaf → root):

    [0] (leaf)
        Subject:           CN=www.example.org
        Issuer:            CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1
        Serial:            0abc...
        Not Before:        2025-01-15T00:00:00Z
        Not After:         2026-02-15T23:59:59Z
        Fingerprint:       a1b2c3...
        Status:            ok

    [1] (intermediate)
        Subject:           CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1
        ...

    [2] (root)
        Subject:           CN=DigiCert Global Root G2
        ...
```

**HTTP:**

```bash
$ greet http example.com --max-redirects 1
```

```
Protocol:  http
Transport: tcp
TTDR:      5.1ms
RTT:       14.2ms
Success:   true

[tcp]
  TTFB:    14.2ms
  TTLB:    14.2ms
  Success: true

[http]
  TTFB:    45.6ms
  TTLB:    46.1ms
  Success: true
  Protocol Version: HTTP/1.1
  Status: 200 OK
  Status Code: 200
```

**SSH:**

```bash
$ greet ssh github.com
```

```
Protocol:  ssh
Transport: tcp
TTDR:      5.2ms
RTT:       12.8ms
Success:   true

[tcp]
  TTFB:    12.8ms
  TTLB:    12.8ms
  Success: true

[ssh]
  TTFB:    45.3ms
  TTLB:    45.3ms
  Success: true
  Version String: SSH-2.0-babeld-77e6c0e7
```

**Minecraft:**

```bash
$ greet minecraft mc.hypixel.net
```

```
Protocol:  minecraft
Transport: tcp
TTDR:      3.1ms
RTT:       28.5ms
Success:   true

[tcp]
  TTFB:    28.5ms
  TTLB:    28.5ms
  Success: true

[minecraft]
  TTFB:    112.7ms
  TTLB:    113.1ms
  Success: true
  Version: 1.8.9
  MOTD: §aHypixel Network  §c[1.8-1.21]
  Players Online: 46832
  Players Max: 200000
```

**PostgreSQL:**

```bash
$ greet postgresql db.example.com
```

```
Protocol:  postgresql
Transport: tcp
TTDR:      4.2ms
RTT:       15.1ms
Success:   true

[tcp]
  TTFB:    15.1ms
  TTLB:    15.1ms
  Success: true

[postgresql]
  TTFB:    18.9ms
  TTLB:    18.9ms
  Success: true
  SSL Supported: true
```

**MCP:**

```bash
$ greet mcp mcp.example.com --list-tools
```

```
Protocol:  mcp
Transport: tcp
TTDR:      10.2ms
RTT:       28.7ms
Success:   true

[tcp]
  TTFB:    28.7ms
  TTLB:    28.7ms
  Success: true

[tls]
  TTFB:    55.1ms
  TTLB:    62.3ms
  Success: true
  Subject: CN=mcp.example.com
  Issuer: CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1
  Serial: 1f2a...
  Not Before: 2025-06-01T00:00:00Z
  Not After: 2026-07-01T23:59:59Z
  Version: 3
  DNS Names: [mcp.example.com]
  Signature Algorithm: SHA256-RSA
  Public Key Algorithm: RSA
  Fingerprint (SHA-256): d4e5f6...
  Chain Status: ok

[mcp]
  TTFB:    75.9ms
  TTLB:    145.2ms
  Success: true
  Protocol Version: 2026-07-28
  Server: example-mcp-server 2.1.0
  Instructions: Use this server to query the example API.
  Capabilities: tools
  Tools (1):
    - search
      Search the example dataset.
```

### JSON output

Use `--output-format json` for machine-readable output. The schema is fully described in [JSON API.md](./JSON%20API.md).

```bash
greet tls example.com --output-format json
```

```json
{
  "protocol": "tls",
  "transport": "tcp",
  "ttdrMs": 6.5219,
  "rttMs": 18.6273,
  "success": true,
  "layers": [
    {
      "name": "tcp",
      "ttfbMs": 18.6273,
      "ttlbMs": 18.6273,
      "success": true
    },
    {
      "name": "tls",
      "ttfbMs": 156.8465,
      "ttlbMs": 156.8465,
      "success": true,
      "data": {
        "certChain": [
          {
            "subject": "CN=www.example.org",
            "issuer": "CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1",
            "serial": "0abc...",
            "notBefore": "2025-01-15T00:00:00Z",
            "notAfter": "2026-02-15T23:59:59Z",
            "version": 3,
            "dnsNames": ["www.example.org", "example.com"],
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

## Errors

When a probe fails, the CLI prints the error to stderr and exits with code 1. If partial results are available (e.g. TCP succeeded but TLS failed), they are printed before the error.

```bash
$ greet tls 10.0.0.99:443
```

```
[tcp]
  TTFB:    5000ms
  TTLB:    5000ms
  Success: false

Error: [tcp] connection_timeout: dial tcp 10.0.0.99:443: i/o timeout
Code: connection_timeout
Protocol: tcp
```

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | All layers succeeded. |
| `1` | Probe failed (connection error, handshake error, unknown protocol, invalid arguments). |

## Scripting

The JSON output is designed for pipelines:

```bash
# Check if TLS is valid for a domain
greet tls example.com --output-format json | jq '.success'

# Extract the TLS certificate subject
greet tls example.com --output-format json | jq '.layers[] | select(.name=="tls") | .data.certChain[0].subject'

# Get Minecraft player count
greet minecraft mc.hypixel.net --output-format json | jq '.layers[] | select(.name=="minecraft") | .data.players'

# List MCP server tools
greet mcp mcp.example.com --output-format json --list-tools | jq '.layers[] | select(.name=="mcp") | .data.tools[] | .name'

# Extract MCP server version
greet mcp mcp.example.com --output-format json | jq '.layers[] | select(.name=="mcp") | .data.serverInfo.version'

# Wait for a server to come online
while ! greet tcp my-server:8080 --timeout 2s; do sleep 5; done

# Check multiple servers
for host in server1.example.com server2.example.com; do
  greet tls "$host" --output-format json | jq '{host: "'"$host"'", success: .success, rttMs: .rttMs}'
done
```

## Timeout behavior

`--timeout` sets the deadline for the entire operation including DNS resolution. The default is 5 seconds.

```bash
# Aggressive 1-second timeout for local network checks
greet tcp 192.168.1.50:3000 --timeout 1s

# Patient 30-second timeout for slow connections
greet tls overseas.example.com --timeout 30s
```

The timeout covers DNS + TCP dial + all layer handshakes combined. If the total exceeds the limit, the operation fails with a `deadline_exceeded` or `connection_timeout` error.

## Extending the CLI with flags

To add CLI flags for a new protocol, create a `flags_<name>.go` file in `cmd/greet/`. The file registers a `flagHandler` via `init()` that defines flag registration and parsing logic:

```go
// cmd/greet/flags_myproto.go
package main

import (
    "flag"
    "github.com/crystade/greet"
    "github.com/crystade/greet/protocols/myproto"
)

func init() {
    registerFlagHandler(myproto.ProtocolName, flagHandler{
        register: registerMyProtoFlags,
        parse:    parseMyProtoFlags,
    })
}

func registerMyProtoFlags(fs *flag.FlagSet) {
    fs.Int("my-flag", 1234, "My custom flag")
}

func parseMyProtoFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
    val := 1234
    if f := fs.Lookup("my-flag"); f != nil {
        if getter, ok := f.Value.(flag.Getter); ok {
            if v, ok := getter.Get().(int); ok {
                val = v
            }
        }
    }
    return []greet.GreetOption{
        greet.WithLayerConfig(myproto.ProtocolName, &myproto.MyProtoConfig{MyField: val}),
    }, nil
}
```

The CLI automatically discovers registered handlers when it resolves a protocol stack. No changes to the core library or protocol package are needed.
