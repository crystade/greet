package main

import (
	"flag"
	"fmt"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/postgresql"
)

func init() {
	registerFlagHandler(postgresql.ProtocolName, flagHandler{
		register: registerPostgreSQLFlags,
		parse:    parsePostgreSQLFlags,
	})
}

func registerPostgreSQLFlags(fs *flag.FlagSet) {
	fs.String("sslmode", "prefer", "SSL mode: prefer (try SSL) or disable (skip SSL)")
}

func parsePostgreSQLFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	sslMode := fs.Lookup("sslmode").Value.String()
	switch sslMode {
	case "prefer", "disable":
		// valid
	default:
		return nil, fmt.Errorf("invalid sslmode %q: must be 'prefer' or 'disable'", sslMode)
	}
	return []greet.GreetOption{greet.WithLayerConfig(postgresql.ProtocolName, &postgresql.PostgreSQLConfig{SSLMode: sslMode})}, nil
}
