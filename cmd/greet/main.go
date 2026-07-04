package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/crystade/greet"
	_ "github.com/crystade/greet/jsonapi/all"
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

	// Resolve the named protocol to its layer stack.
	stack, err := greet.Resolve(protoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: unknown protocol %q\n\n", protoName)
		printProtocols()
		os.Exit(1)
	}

	// Build a shared FlagSet. Each FlaggedLayer in the stack registers its flags here.
	// Flag names are assumed collision-free across layers.
	fs := flag.NewFlagSet(protoName, flag.ContinueOnError)
	fs.Duration("timeout", greet.DefaultTimeout, "Handshake timeout (e.g. 5s, 1m)")
	fs.String("output-format", "default", "Output format: default or json")

	// Collect all FlaggedLayer instances (transport + app layers).
	type flaggedEntry struct {
		layer greet.FlaggedLayer
	}
	var flagged []flaggedEntry
	if fl, ok := stack.Transport.(greet.FlaggedLayer); ok {
		fl.RegisterFlags(fs)
		flagged = append(flagged, flaggedEntry{fl})
	}
	for _, app := range stack.Apps {
		if fl, ok := app.(greet.FlaggedLayer); ok {
			fl.RegisterFlags(fs)
			flagged = append(flagged, flaggedEntry{fl})
		}
	}

	// Parse remaining args (everything after protocol and target)
	var opts []greet.GreetOption
	if len(os.Args) > 3 {
		if err := fs.Parse(os.Args[3:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	for _, fe := range flagged {
		extraOpts, err := fe.layer.ParseFlags(fs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, extraOpts...)
	}

	// Read timeout from flag.
	timeout := greet.DefaultTimeout
	if f := fs.Lookup("timeout"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(time.Duration); ok {
				timeout = v
			}
		}
	}
	opts = append(opts, greet.WithTimeout(timeout))

	// Read output-format from flag and validate it.
	outputFormat := "default"
	if f := fs.Lookup("output-format"); f != nil {
		outputFormat = f.Value.String()
	}
	switch outputFormat {
	case "default", "json":
		// valid
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid output-format %q: must be 'default' or 'json'\n", outputFormat)
		os.Exit(1)
	}

	// Parse target into host:port using the outermost layer's default port.
	host, port, err := greet.ParseTarget(target, stack.DefaultPort())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	result, err := greet.GreetStack(ctx, stack, host, port, opts...)
	if err != nil {
		// Print any partial results first, then the error.
		if result != nil && len(result.Layers) > 0 {
			printResultAs(result, outputFormat)
			fmt.Fprintln(os.Stderr)
		}
		printError(err)
		os.Exit(1)
	}

	printResultAs(result, outputFormat)
}

// printResultAs writes the result in the requested output format.
func printResultAs(result *greet.GreetResult, outputFormat string) {
	if outputFormat == "json" {
		printResultJSON(result)
		return
	}
	printResult(result)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "greet — pluggable protocol greeter\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  greet <protocol> <host>[:<port>] [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --timeout duration       Handshake timeout (default 5s)\n")
	fmt.Fprintf(os.Stderr, "  --output-format string   Output format: default or json (default \"default\")\n\n")
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
	layers := greet.ListAll()
	if len(layers) == 0 {
		fmt.Fprintf(os.Stderr, "No protocols registered.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "Available protocols:\n")
	for _, l := range layers {
		port := ""
		if l.DefaultPort > 0 {
			port = fmt.Sprintf(":%d", l.DefaultPort)
		}
		fmt.Fprintf(os.Stderr, "  %-14s %-6s %s\n", l.Name, port, l.Description)
	}
}
