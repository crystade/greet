package main

import (
	"fmt"
	"strings"

	"github.com/crystade/greet"
	greetmcp "github.com/crystade/greet/protocols/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func printMCPData(data *greetmcp.MCPResult) {
	fmt.Printf("Protocol Version: %s\n", data.ProtocolVersion)

	if data.ServerInfo != nil {
		fmt.Printf("Server: %s %s\n", data.ServerInfo.Name, data.ServerInfo.Version)
	}

	if data.Capabilities != nil {
		printCapabilities(data.Capabilities)
	}

	if len(data.Resources) > 0 {
		fmt.Printf("Resources (%d):\n", len(data.Resources))
		for _, r := range data.Resources {
			printResource(r)
		}
	}

	if len(data.ResourceTemplates) > 0 {
		fmt.Printf("Resource Templates (%d):\n", len(data.ResourceTemplates))
		for _, t := range data.ResourceTemplates {
			printResourceTemplate(t)
		}
	}

	if len(data.Tools) > 0 {
		fmt.Printf("Tools (%d):\n", len(data.Tools))
		for _, t := range data.Tools {
			printTool(t)
		}
	}

	if len(data.Prompts) > 0 {
		fmt.Printf("Prompts (%d):\n", len(data.Prompts))
		for _, p := range data.Prompts {
			printPrompt(p)
		}
	}
}

func printCapabilities(caps *sdkmcp.ServerCapabilities) {
	var parts []string
	if caps.Prompts != nil {
		s := "prompts"
		if caps.Prompts.ListChanged {
			s += " (list-changed)"
		}
		parts = append(parts, s)
	}
	if caps.Resources != nil {
		s := "resources"
		if caps.Resources.Subscribe {
			s += " (subscribe)"
		}
		if caps.Resources.ListChanged {
			s += " (list-changed)"
		}
		parts = append(parts, s)
	}
	if caps.Tools != nil {
		s := "tools"
		if caps.Tools.ListChanged {
			s += " (list-changed)"
		}
		parts = append(parts, s)
	}
	if caps.Completions != nil {
		parts = append(parts, "completions")
	}
	if caps.Logging != nil {
		parts = append(parts, "logging")
	}
	if len(parts) > 0 {
		fmt.Printf("Capabilities: %s\n", strings.Join(parts, ", "))
	}
}

func printResource(r *greetmcp.ResourceInfo) {
	fmt.Printf("  - %s  (%s)\n", r.Name, r.URI)
}

func printResourceTemplate(t *greetmcp.ResourceTemplateInfo) {
	fmt.Printf("  - %s  (%s)\n", t.Name, t.URITemplate)
}

func printTool(t *greetmcp.ToolInfo) {
	fmt.Printf("  - %s\n", t.Name)
}

func printPrompt(p *greetmcp.PromptInfo) {
	fmt.Printf("  - %s\n", p.Name)
}

func init() {
	registerLayerPrinter("mcp", func(layer greet.LayerResult) {
		if r, ok := layer.Data.(*greetmcp.MCPResult); ok {
			printMCPData(r)
		}
	})
}
