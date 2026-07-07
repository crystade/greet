// Package all blank-imports every jsonapi protocol subpackage to trigger their
// init() registration. Import this package to opt into all built-in mappers:
//
//	import _ "github.com/crystade/greet/jsonapi/all"
package all

import (
	_ "github.com/crystade/greet/jsonapi/http"
	_ "github.com/crystade/greet/jsonapi/minecraft"
	_ "github.com/crystade/greet/jsonapi/postgresql"
	_ "github.com/crystade/greet/jsonapi/ssh"
	_ "github.com/crystade/greet/jsonapi/tls"
)
