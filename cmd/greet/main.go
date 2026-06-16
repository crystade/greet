package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/crystade/greet"
	_ "github.com/crystade/greet/protocols/all"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	protoName := os.Args[1]

	// Handle --help / -h before protocol lookup
	if protoName == "--help" || protoName == "-h" {
		printUsage()
		os.Exit(0)
	}

	// Handle "list" subcommand
	if protoName == "list" {
		printProtocols()
		os.Exit(0)
	}

	// Need at least a target
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Error: missing target argument\n\n")
		printUsageFor(protoName)
		os.Exit(1)
	}

	target := os.Args[2]

	p, err := greet.Get(protoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: unknown protocol %q\n\n", protoName)
		printProtocols()
		os.Exit(1)
	}

	// Build a flag.FlagSet for this protocol
	fs := flag.NewFlagSet(protoName, flag.ContinueOnError)
	fs.Duration("timeout", greet.DefaultTimeout, "Handshake timeout (e.g. 5s, 1m)")

	// If the protocol implements FlaggedProtocol, register its flags
	var protoOpts []greet.GreetOption
	var fp greet.FlaggedProtocol
	isFlagged := false
	if v, ok := p.(greet.FlaggedProtocol); ok {
		fp = v
		isFlagged = true
		fp.RegisterFlags(fs)
	}

	// Parse remaining args (everything after protocol and target)
	if len(os.Args) > 3 {
		if err := fs.Parse(os.Args[3:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if isFlagged {
			extraOpts, err := fp.ParseFlags(fs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			protoOpts = append(protoOpts, extraOpts...)
		}
	}

	// Read timeout from flag — use flag.Getter to avoid re-parsing a string.
	timeout := greet.DefaultTimeout
	if f := fs.Lookup("timeout"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(time.Duration); ok {
				timeout = v
			}
		}
	}
	protoOpts = append(protoOpts, greet.WithTimeout(timeout))

	// Parse target into host:port
	host, port, err := greet.ParseTarget(target, p.DefaultPort())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	result, err := greet.GreetWith(ctx, p, host, port, protoOpts...)

	if err != nil {
		printError(err)
		os.Exit(1)
	}

	printResult(result)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "greet — pluggable protocol greeter\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  greet <protocol> <host>[:<port>] [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --timeout duration   Handshake timeout (default 5s)\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  greet list              List all registered protocols\n")
	fmt.Fprintf(os.Stderr, "  greet --help            Show this help\n\n")
	printProtocols()
}

func printUsageFor(protoName string) {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  greet %s <host>[:<port>] [flags]\n", protoName)
}

func printProtocols() {
	protocols := greet.List()
	if len(protocols) == 0 {
		fmt.Fprintf(os.Stderr, "No protocols registered.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "Available protocols:\n")
	for _, p := range protocols {
		port := ""
		if p.DefaultPort() > 0 {
			port = fmt.Sprintf(":%d", p.DefaultPort())
		}
		fmt.Fprintf(os.Stderr, "  %-14s %-6s %s\n", p.Name(), port, p.Description())
	}
}
