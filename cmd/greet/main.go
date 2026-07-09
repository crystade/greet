package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/crystade/greet"
	japiall "github.com/crystade/greet/jsonapi/all"
	protoall "github.com/crystade/greet/protocols/all"
)

func main() {
	engine := protoall.NewEngine()
	codec := japiall.NewCodec()

	if len(os.Args) < 2 {
		printUsage(engine)
		os.Exit(1)
	}

	protoName := os.Args[1]

	// Handle --help / -h before protocol lookup
	if protoName == "--help" || protoName == "-h" {
		printUsage(engine)
		os.Exit(0)
	}

	// Handle "list" subcommand
	if protoName == "list" {
		printProtocols(engine)
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
	stack, err := engine.Resolve(protoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: unknown protocol %q\n\n", protoName)
		printProtocols(engine)
		os.Exit(1)
	}

	// Build a shared FlagSet. Each FlaggedLayer in the stack registers its flags here.
	// Flag names are assumed collision-free across layers.
	fs := flag.NewFlagSet(protoName, flag.ContinueOnError)
	fs.String("output-format", "default", "Output format: default or json")
	fs.Duration("timeout", 5*time.Second, "Global handshake timeout")

	// Collect flag handlers for layers in this stack (transport + app layers).
	var layerFlagHandlers []flagHandler
	if h, ok := flagHandlers[stack.Transport.Name()]; ok {
		h.register(fs)
		layerFlagHandlers = append(layerFlagHandlers, h)
	}
	for _, app := range stack.Apps {
		if h, ok := flagHandlers[app.Name()]; ok {
			h.register(fs)
			layerFlagHandlers = append(layerFlagHandlers, h)
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
	for _, h := range layerFlagHandlers {
		extraOpts, err := h.parse(fs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, extraOpts...)
	}

	// Read global flags
	outputFormat := "default"
	if f := fs.Lookup("output-format"); f != nil {
		outputFormat = f.Value.String()
	}
	timeout := 5 * time.Second
	if f := fs.Lookup("timeout"); f != nil {
		if parsed, err := time.ParseDuration(f.Value.String()); err == nil {
			timeout = parsed
		}
	}

	// The CLI wraps the context with a timeout for usability.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
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

	result := engine.GreetStack(ctx, stack, host, port, opts...)
	printResultAs(result, outputFormat, codec)
	if !result.Success {
		// Print error
		printError(result.Error)
		os.Exit(1)
	}
}

func printUsage(engine *greet.Engine) {
	fmt.Fprintf(os.Stderr, "greet — pluggable protocol greeter\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  greet <protocol> <host>[:<port>] [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --output-format string   Output format: default or json (default \"default\")\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  greet list              List all registered protocols\n")
	fmt.Fprintf(os.Stderr, "  greet --help            Show this help\n\n")
	if engine != nil {
		printProtocols(engine)
	}
}

func printUsageFor(protoName string) {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  greet %s <host>[:<port>] [flags]\n", protoName)
}

func printProtocols(engine *greet.Engine) {
	layers := engine.ListAll()
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
