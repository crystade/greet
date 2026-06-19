package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/crystade/greet"
)

// dataPrinter is a function that displays protocol-specific data fields to stdout.
type dataPrinter func(result *greet.GreetResult)

// dataPrinters is a registry of per-protocol display functions, keyed by protocol name.
var dataPrinters = map[string]dataPrinter{}

// registerDataPrinter registers a display function for a protocol name.
func registerDataPrinter(name string, fn dataPrinter) {
	dataPrinters[name] = fn
}

// printResult writes the result fields to stdout as plain text, one field per line.
func printResult(result *greet.GreetResult) {
	fmt.Printf("Protocol: %s\n", result.Protocol)
	fmt.Printf("Transport: %s\n", result.Transport)
	fmt.Printf("TTDR: %s\n", result.TTDR)
	fmt.Printf("RTT: %s\n", result.RTT)
	fmt.Printf("TTFB: %s\n", result.TTFB)
	fmt.Printf("TTLB: %s\n", result.TTLB)
	fmt.Printf("Success: %v\n", result.Success)

	if printer, ok := dataPrinters[result.Protocol]; ok {
		printer(result)
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
