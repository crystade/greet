package all

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/http"
	"github.com/crystade/greet/protocols/mcp"
	"github.com/crystade/greet/protocols/minecraft"
	"github.com/crystade/greet/protocols/postgresql"
	"github.com/crystade/greet/protocols/ssh"
	"github.com/crystade/greet/protocols/tcp"
	"github.com/crystade/greet/protocols/tls"
	"github.com/crystade/greet/protocols/udp"
)

// Register adds all built-in protocols to the given engine.
func Register(e *greet.Engine) {
	e.RegisterTransport(tcp.New())
	e.RegisterTransport(udp.New())
	e.RegisterApp(tls.New())
	e.RegisterApp(ssh.New())
	e.RegisterApp(http.New())
	e.RegisterApp(minecraft.New())
	e.RegisterApp(postgresql.New())
	e.RegisterApp(mcp.New())
	e.RegisterStack("mcp", []string{"tcp", "tls", "mcp"}, 443, "MCP handshake + capability probe")
	e.RegisterStack("https", []string{"tcp", "tls", "http"}, 443, "HTTPS request/response probe (TLS + HTTP)")
}

// NewEngine creates an Engine pre-loaded with all built-in protocols.
// Convenience wrapper around NewEngine() + Register().
func NewEngine() *greet.Engine {
	e := greet.NewEngine()
	Register(e)
	return e
}
