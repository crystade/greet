package main

import (
	"fmt"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/postgresql"
)

func printPostgreSQLData(data *postgresql.PostgreSQLResult) {
	fmt.Printf("SSL Supported: %v\n", data.SSLSupported)
}

func init() {
	registerDataPrinter("postgresql", func(result *greet.GreetResult) {
		if pg, ok := result.Data.(*postgresql.PostgreSQLResult); ok {
			printPostgreSQLData(pg)
		}
	})
}
