// Package all blank-imports every protocol to trigger their init() registration.
// Import this package to opt into all built-in protocols:
//
//	import _ "github.com/crystade/greet/protocols/all"
package all

import (
	"github.com/crystade/greet"
	_ "github.com/crystade/greet/protocols/http"
	_ "github.com/crystade/greet/protocols/minecraft"
	_ "github.com/crystade/greet/protocols/postgresql"
	_ "github.com/crystade/greet/protocols/ssh"
	_ "github.com/crystade/greet/protocols/tcp"
	_ "github.com/crystade/greet/protocols/tls"
	_ "github.com/crystade/greet/protocols/udp"
)

func init() {
	// Register composite stacks that span multiple layers here, after all
	// individual layer init() calls have run (guaranteed by Go's init order).
	greet.RegisterStack("https", []string{"tcp", "tls", "http"}, 443, "HTTPS request/response probe (TLS + HTTP)")
}
