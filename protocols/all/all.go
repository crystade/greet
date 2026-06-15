// Package all blank-imports every protocol to trigger their init() registration.
// Import this package to opt into all built-in protocols:
//
//	import _ "github.com/crystade/greet/protocols/all"
package all

import (
	_ "github.com/crystade/greet/protocols/minecraft"
	_ "github.com/crystade/greet/protocols/postgresql"
	_ "github.com/crystade/greet/protocols/ssh"
	_ "github.com/crystade/greet/protocols/tcp"
	_ "github.com/crystade/greet/protocols/udp"
)
