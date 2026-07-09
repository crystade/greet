package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/crystade/greet"
	greethttp "github.com/crystade/greet/protocols/http"
)

func init() {
	registerFlagHandler(greethttp.ProtocolName, flagHandler{
		register: registerHTTPFlags,
		parse:    parseHTTPFlags,
	})
}

func registerHTTPFlags(fs *flag.FlagSet) {
	fs.String("method", "GET", "HTTP method (e.g. GET, POST, HEAD)")
	fs.String("path", "/", "Request path and optional query (e.g. /users?sort=name)")
	fs.Int64("max-body", greethttp.DefaultMaxResponseBodyBytes, "Maximum response body bytes to buffer")
	fs.Int("max-redirects", greethttp.DefaultMaxRedirectFollow, "Maximum number of redirects to follow (0 = none)")
	fs.Var(&headerFlag{}, "header", "Request header in Name:Value form; may be repeated")
}

func parseHTTPFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	method := fs.Lookup("method").Value.String()
	path := fs.Lookup("path").Value.String()

	var maxBody int64 = greethttp.DefaultMaxResponseBodyBytes
	if f := fs.Lookup("max-body"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(int64); ok {
				maxBody = v
			}
		}
	}

	var headers []greethttp.Header
	if f := fs.Lookup("header"); f != nil {
		if hf, ok := f.Value.(*headerFlag); ok {
			headers = hf.headers
		}
	}

	maxRedirects := greethttp.DefaultMaxRedirectFollow
	if f := fs.Lookup("max-redirects"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(int); ok {
				maxRedirects = v
			}
		}
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(greethttp.ProtocolName, &greethttp.HTTPConfig{
			Target:               path,
			Method:               method,
			Headers:              headers,
			MaxResponseBodyBytes: maxBody,
			MaxRedirectFollow:    maxRedirects,
		}),
	}, nil
}

// headerFlag implements flag.Value for repeated --header flags.
type headerFlag struct {
	headers []greethttp.Header
}

func (f *headerFlag) String() string { return "" }

func (f *headerFlag) Set(s string) error {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return fmt.Errorf("header %q must be in Name:Value form", s)
	}
	f.headers = append(f.headers, greethttp.Header{
		Name:  strings.TrimSpace(s[:idx]),
		Value: strings.TrimSpace(s[idx+1:]),
	})
	return nil
}

func (f *headerFlag) Get() any { return f.headers }
