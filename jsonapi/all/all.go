package all

import (
	"github.com/crystade/greet/jsonapi"
	japiHTTP "github.com/crystade/greet/jsonapi/http"
	japiMinecraft "github.com/crystade/greet/jsonapi/minecraft"
	japiPostgresql "github.com/crystade/greet/jsonapi/postgresql"
	japiSSH "github.com/crystade/greet/jsonapi/ssh"
	japiTLS "github.com/crystade/greet/jsonapi/tls"
)

// Register adds all built-in protocol mappers/decoders to the given Codec.
func Register(c *jsonapi.Codec) {
	japiTLS.Register(c)
	japiSSH.Register(c)
	japiHTTP.Register(c)
	japiMinecraft.Register(c)
	japiPostgresql.Register(c)
}

// NewCodec creates a Codec pre-loaded with all built-in protocol mappers.
func NewCodec() *jsonapi.Codec {
	c := jsonapi.NewCodec()
	Register(c)
	return c
}
