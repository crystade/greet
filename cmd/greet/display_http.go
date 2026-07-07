package main

import (
	"fmt"

	"github.com/crystade/greet"
	greethttp "github.com/crystade/greet/protocols/http"
)

func printHTTPData(data *greethttp.HTTPResult) {
	fmt.Printf("  Protocol Version: %s\n", data.ProtocolVersion)
	fmt.Printf("  Status:           %s\n", data.Status)

	if len(data.Redirects) > 0 {
		fmt.Printf("  Redirects (%d):\n", len(data.Redirects))
		for _, re := range data.Redirects {
			fmt.Printf("    -> [%d] %s %s\n", re.StatusCode, re.Method, re.URL)
			for _, h := range re.ResponseHeaders {
				fmt.Printf("       %s: %s\n", h.Name, h.Value)
			}
		}
	}

	if len(data.ResponseHeaders) > 0 {
		fmt.Printf("  Response Headers:\n")
		for _, h := range data.ResponseHeaders {
			fmt.Printf("    %s: %s\n", h.Name, h.Value)
		}
	}
}

func init() {
	registerLayerPrinter("http", func(layer greet.LayerResult) {
		if r, ok := layer.Data.(*greethttp.HTTPResult); ok {
			printHTTPData(r)
		}
	})
}
