# CLI

The `greet` CLI is a pluggable protocol greeter — it probes remote servers across layered protocol stacks and reports per-layer timing and data. It supports both human-readable and JSON output, and auto-discovers protocol-specific flags from registered layers.

## Installation

```bash
go install github.com/crystade/greet/cmd/greet@latest
```

Or build from source:

```bash
cd cmd/greet
go build -o greet .
```

## Basic usage

```
greet <protocol> <host>[:<port>] [flags]
```

The protocol name is required and must be one of the registered protocols. The target is `host` or `host:port` — if the port is omitted, the protocol's default is used.

```bash
# Probe a TLS server on port 443 (default)
greet tls example.com

# Probe an SSH server on a custom port
greet ssh 10.0.0.5:2222

# Check if a Minecraft server is online
greet minecraft mc.hypixel.net
```

### IPv6

Use bracket notation for IPv6 addresses:

```bash
greet tcp [::1]:8080
greet tls [2606:4700:4700::1111]:443
```

## Supported protocols

| Name          | Default port | Description |
|---------------|-------------|-------------|
| `tcp`         | 80          | Plain TCP connectivity check. |
| `udp`         | 53          | UDP probe — sends an empty datagram and waits for a response. |
| `tls`         | 443         | TLS handshake + certificate chain inspection. |
| `ssh`         | 22          | Reads the server's SSH version banner. |
| `minecraft`   | 25565       | Minecraft Java Edition handshake + status request. |
| `postgresql`  | 5432        | PostgreSQL `SSLRequest` probe. |

## Global flags

| Flag                | Type     | Default     | Description |
|---------------------|----------|-------------|-------------|
| `--timeout`         | duration | `5s`        | Handshake timeout (e.g. `3s`, `1m`, `500ms`). |
| `--output-format`   | string   | `"default"` | Output format: `"default"` (human-readable) or `"json"`. |

## Protocol-specific flags

Protocol layers can register their own flags. These are automatically discovered by the CLI. Only flags for layers in the resolved stack are available.

### Minecraft

| Flag                  | Type  | Default | Description |
|-----------------------|-------|---------|-------------|
| `--protocol-version`  | int   | `775`   | Minecraft protocol version (e.g. `775` for 1.20.4, `766` for 1.20.2). |

```bash
greet minecraft mc.example.com --protocol-version 766
```

### PostgreSQL

| Flag          | Type   | Default    | Description |
|---------------|--------|------------|-------------|
| `--sslmode`   | string | `"prefer"` | SSL mode: `"prefer"` (try SSL) or `"disable"` (skip SSL). |

```bash
greet postgresql db.example.com --sslmode disable
```

## Commands

### List registered protocols

```bash
greet list
```

Output:

```
Available protocols:
  minecraft      :25565 Minecraft Java Edition handshake + status request
  postgresql     :5432  PostgreSQL startup packet exchange
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

## Output formats

### Default (human-readable)

Shows protocol-level metrics followed by each layer's timing and protocol-specific data.

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

### SSH example

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

### Minecraft example

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

### PostgreSQL example

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

### JSON output

Use `--output-format json` to get machine-readable JSON using the `jsonapi` models:

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

The JSON schema is fully described in [JSON API.md](./JSON%20API.md).

## Error output

When a probe fails, the CLI prints the error to stderr and exits with code 1. If partial results are available (e.g. TCP succeeded but TLS failed), they are printed first.

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
| `0`  | All layers succeeded. |
| `1`  | Probe failed (connection error, handshake error, unknown protocol, invalid arguments). |

## Using flags

Flags must appear after the protocol and target:

```bash
# Correct — flags after target
greet minecraft mc.example.com --protocol-version 766 --timeout 3s

# Also correct — flags can be interspersed
greet tls example.com:8443 --output-format json --timeout 10s

# Wrong — flags before target will be misinterpreted
greet tls --timeout 3s example.com   ❌
```

## Scripting

The JSON output format is designed for pipelines:

```bash
# Check if TLS is valid for a domain
greet tls example.com --output-format json | jq '.success'

# Extract the TLS certificate subject
greet tls example.com --output-format json | jq '.layers[] | select(.name=="tls") | .data.certChain[0].subject'

# Get Minecraft player count
greet minecraft mc.hypixel.net --output-format json | jq '.layers[] | select(.name=="minecraft") | .data.players'

# Wait for a server to come online
while ! greet tcp my-server:8080 --timeout 2s; do sleep 5; done

# Check multiple servers
for host in server1.example.com server2.example.com; do
  greet tls "$host" --output-format json | jq '{host: "'"$host"'", success: .success, rttMs: .rttMs}'
done
```

## Timeout behavior

The `--timeout` flag sets the deadline for the entire handshake operation, including DNS resolution. The default is 5 seconds.

```bash
# Aggressive 1-second timeout for local network checks
greet tcp 192.168.1.50:3000 --timeout 1s

# Patient 30-second timeout for slow connections
greet tls overseas.example.com --timeout 30s
```

The timeout applies to the whole operation, not per-layer. If the combined DNS + TCP dial + TLS handshake takes longer than the timeout, the operation fails with a `deadline_exceeded` or `connection_timeout` error.
