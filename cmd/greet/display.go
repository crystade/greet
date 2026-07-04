package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
)

// layerDataPrinter prints protocol-specific fields for a single LayerResult.
type layerDataPrinter func(layer greet.LayerResult)

// layerPrinters is a registry of per-layer display functions, keyed by layer name.
var layerPrinters = map[string]layerDataPrinter{}

// registerLayerPrinter registers a display function for a layer name.
func registerLayerPrinter(name string, fn layerDataPrinter) {
	layerPrinters[name] = fn
}

// printResult writes the aggregated result to stdout.
// Transport-level metrics are printed first, then each app layer's timing and data.
func printResult(result *greet.GreetResult) {
	fmt.Printf("Protocol:  %s\n", result.Protocol)
	fmt.Printf("Transport: %s\n", result.Transport)
	fmt.Printf("TTDR:      %s\n", result.TTDR)
	fmt.Printf("RTT:       %s\n", result.RTT)
	fmt.Printf("Success:   %v\n", result.Success)

	for _, layer := range result.Layers {
		fmt.Printf("\n[%s]\n", layer.Name)
		fmt.Printf("  TTFB:    %s\n", layer.TTFB)
		fmt.Printf("  TTLB:    %s\n", layer.TTLB)
		fmt.Printf("  Success: %v\n", layer.Success)
		if printer, ok := layerPrinters[layer.Name]; ok {
			printer(layer)
		}
	}
}

// printResultJSON writes the result as indented JSON to stdout using the
// jsonapi models. Protocol-specific payloads are mapped via the registered
// jsonapi mappers (imported via jsonapi/all in main).
func printResultJSON(result *greet.GreetResult) {
	resp := jsonapi.FromGreetResult(result)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

// printError writes a human-readable error to stderr.
func printError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())

	var ge *greet.GreetError
	if errors.As(err, &ge) {
		fmt.Fprintf(os.Stderr, "Code: %s\n", ge.Code)
		if ge.Protocol != "" {
			fmt.Fprintf(os.Stderr, "Protocol: %s\n", ge.Protocol)
		}
	}
}
