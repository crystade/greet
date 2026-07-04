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
	registerLayerPrinter("postgresql", func(layer greet.LayerResult) {
		if pg, ok := layer.Data.(*postgresql.PostgreSQLResult); ok {
			printPostgreSQLData(pg)
		}
	})
}
