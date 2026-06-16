![](./assets/banner.jpeg)

**Greet** is a lightweight Go library to test greet / ping / handshake process in TCP, UDP and other popular protocols based on them.

## Goals
- Fast: The library only performs handshake without entering full session
- Secure: The process ends before authentication / authorization step
- Lightweight: The library constructs raw command from scratch and does not depend on third-party service clients
- Tracing: The library outputs helpful errors from connection init to handshake

## Use case
- Greet is good enough to test connectivity, service name and service version.

## Installation

### Library

```bash
go get github.com/crystade/greet@latest
```

Then import and use in your Go code:

```go
import "github.com/crystade/greet"

func main() {
    ctx := context.Background()
    result, err := greet.Greet(ctx, "ssh", "github.com:22")
    // ...
}
```

### CLI

```bash
go install github.com/crystade/greet/cmd/greet@latest
```

Or build from source:

```bash
go build -o greet ./cmd/greet/
```

## Library Usage

The [`greet.Greet()`](greet.go:14) function is the primary entry point:

```go
result, err := greet.Greet(ctx, "ssh", "github.com:22")
if err != nil {
    // handle *greet.GreetError
}
fmt.Println(result.Success, result.Latency, result.Data)
```

Customize behavior with functional options:

```go
result, err := greet.Greet(ctx, "minecraft", "hypixel.net:25565",
    greet.WithTimeout(10*time.Second),
    greet.WithProtocolConfig(&minecraft.Config{ProtocolVersion: 775}),
)
```

For pre-resolved protocols, use [`greet.GreetWith()`](greet.go:29):

```go
p, _ := greet.Get("ssh")
result, err := greet.GreetWith(ctx, p, "github.com", 22)
```

See [`greet.go`](greet.go) and [`options.go`](options.go) for the complete API.

## CLI Usage

```
greet <protocol> <host>[:<port>] [flags]
```

### Commands

| Command | Description |
|---------|-------------|
| `greet list` | List all registered protocols |
| `greet --help` | Show help |

### Examples

```bash
# Generic TCP — test connectivity to a port
greet tcp example.com:80

# SSH — read server version banner
greet ssh github.com:22

# PostgreSQL — probe SSL support
greet postgresql localhost:5432

# PostgreSQL — skip SSL probe
greet postgresql localhost:5432 --sslmode=disable

# Minecraft Java — query server status
greet minecraft hypixel.net:25565

# Minecraft — specify protocol version (default: 775)
greet minecraft localhost:25565 --protocol-version=775

# Use default port (from protocol's well-known port)
greet ssh github.com        # equivalent to github.com:22
```

### Output

All commands output structured JSON to stdout:

```json
{
  "protocol": "ssh",
  "transport": "tcp",
  "latency": "45.123ms",
  "latency_ms": 45.123,
  "success": true,
  "data": {
    "version_string": "SSH-2.0-OpenSSH_8.9"
  }
}
```

Errors are output as JSON to stderr:

```json
{
  "success": false,
  "error": "[ssh] connection_refused: connection to localhost:22 refused",
  "code": "connection_refused",
  "protocol": "ssh"
}
```

### Protocol-Specific Flags

| Protocol | Flag | Default | Description |
|----------|------|---------|-------------|
| `postgresql` | `--sslmode` | `prefer` | SSL mode: `prefer` (try SSL) or `disable` (skip SSL) |
| `minecraft` | `--protocol-version` | `775` | Minecraft protocol version (e.g. `775` for 26.1) |

## Protocols
### Error Code Reuse Hierarchy

Protocols inherit error codes from their transport layer:

```mermaid
graph TD
    COMMON[Common Errors]
    TCP[Generic TCP Errors]
    UDP[Generic UDP Errors]
    SSH[SSH Errors]
    PG[PostgreSQL Errors]
    MC[Minecraft Errors]

    COMMON --> TCP
    COMMON --> UDP
    TCP --> SSH
    TCP --> PG
    TCP --> MC
```

#### Common Errors

| Error Code | Description |
|---|---|
| `resolve_host_failed` | Failed to resolve hostname to IP address |
| `invalid_address` | Invalid host or port format |
| `unknown_protocol` | Requested protocol name is not registered |
| `invalid_config` | Protocol-specific configuration is malformed or of the wrong type |

#### Generic TCP Errors

| Error Code | Description |
|---|---|
| `connection_refused` | Server actively refused the connection |
| `connection_timeout` | TCP connection attempt timed out |
| `connection_reset` | Connection reset by remote peer |
| `network_unreachable` | Target network is unreachable |
| `connection_failed` | Generic connection failure (unknown OS error) |

#### Generic UDP Errors

| Error Code | Description |
|---|---|
| `send_failed` | Failed to send UDP datagram |
| `receive_timeout` | Timed out waiting for UDP response |
| `port_unreachable` | Received ICMP port unreachable |

---

### Generic TCP

- **Input**: Host, Port
- **Output**: Success with latency, or an error code from Common + Generic TCP errors

### Generic UDP

- **Input**: Host, Port
- **Output**: Success with latency, or an error code from Common + Generic UDP errors

### SSH (TCP)

- **Input**: Host, Port (default `22`)
- **Output**: Success with server version string (e.g. `SSH-2.0-OpenSSH_8.9`) and latency, or an error code

Inherits: Common + Generic TCP errors

| Error Code | Description |
|---|---|
| `banner_timeout` | Timed out waiting for SSH version banner |
| `invalid_banner` | Received malformed SSH version string |

### PostgreSQL (TCP)

- **Input**: Host, Port (default `5432`), SSL Mode (default `prefer`)
- **Output**: Success with server version, auth type, and latency, or an error code

Inherits: Common + Generic TCP errors

| Error Code | Description |
|---|---|
| `startup_timeout` | Timed out waiting for server response |
| `send_failed` | Failed to send packet to the server |
| `malformed_response` | Received malformed PostgreSQL message |
| `ssl_rejected` | Server does not support requested SSL mode |

### Minecraft Java (TCP)

- **Input**: Host, Port (default `25565`), Protocol Version (required, VarInt — e.g. `775` for Minecraft 26.1). Intent is automatically set to Status (1) by the library.
- **Output**: Success with server status JSON (MOTD, players, version) and latency, or an error code

Inherits: Common + Generic TCP errors

| Error Code | Description |
|---|---|
| `handshake_timeout` | Timed out waiting for status response |
| `handshake_failed` | Failed to send handshake or status request |
| `malformed_response` | Received malformed status response |

## Contribution
Due to overhead of maintenance, we only accept popular protocols for now.

Postpone:
- DNS: having the most RFCs and Drafts attached to it of all protocols, we are looking to provide support to a subset of popular DNS protocols only

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
