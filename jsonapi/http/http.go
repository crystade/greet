// Package http provides the JSON-serializable model, mapper, and option
// decoder for the HTTP layer. It registers itself with the jsonapi registry
// via init().
package http

import (
	"encoding/json"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	greethttp "github.com/crystade/greet/protocols/http"
)

// Header is the JSON-serializable form of a single HTTP header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Data is the JSON-serializable payload for an HTTP layer.
// The response body is intentionally omitted from the JSON API.
type Data struct {
	ProtocolVersion string        `json:"protocolVersion"`
	StatusCode      int           `json:"statusCode"`
	Status          string        `json:"status"`
	ResponseHeaders []Header      `json:"responseHeaders,omitempty"`
	Redirects       []RedirectHop `json:"redirects,omitempty"`
}

// RedirectHop is a single redirect that was followed.
type RedirectHop struct {
	URL             string   `json:"url"`
	StatusCode      int      `json:"statusCode"`
	Method          string   `json:"method"`
	ResponseHeaders []Header `json:"responseHeaders,omitempty"`
}

// Options holds JSON-serializable HTTP layer options.
type Options struct {
	Target               string   `json:"target,omitempty"`
	Method               string   `json:"method,omitempty"`
	Headers              []Header `json:"headers,omitempty"`
	MaxResponseBodyBytes int64    `json:"maxResponseBodyBytes,omitempty"`
	MaxRedirectFollow    int      `json:"maxRedirectFollow,omitempty"`
}

// FromResult maps a *greethttp.HTTPResult to *Data.
// Returns the input unchanged when data is not a *greethttp.HTTPResult.
func FromResult(data any) any {
	r, ok := data.(*greethttp.HTTPResult)
	if !ok || r == nil {
		return data
	}

	out := &Data{
		ProtocolVersion: r.ProtocolVersion,
		StatusCode:      r.StatusCode,
		Status:          r.Status,
	}

	if len(r.ResponseHeaders) > 0 {
		out.ResponseHeaders = make([]Header, len(r.ResponseHeaders))
		for i, h := range r.ResponseHeaders {
			out.ResponseHeaders[i] = Header{Name: h.Name, Value: h.Value}
		}
	}

	if len(r.Redirects) > 0 {
		out.Redirects = make([]RedirectHop, len(r.Redirects))
		for i, re := range r.Redirects {
			rh := RedirectHop{
				URL:        re.URL,
				StatusCode: re.StatusCode,
				Method:     re.Method,
			}
			if len(re.ResponseHeaders) > 0 {
				rh.ResponseHeaders = make([]Header, len(re.ResponseHeaders))
				for j, h := range re.ResponseHeaders {
					rh.ResponseHeaders[j] = Header{Name: h.Name, Value: h.Value}
				}
			}
			out.Redirects[i] = rh
		}
	}

	return out
}

// DecodeOptions decodes raw HTTP options into engine GreetOptions.
func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
	var o Options
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}

	var headers []greethttp.Header
	for _, h := range o.Headers {
		headers = append(headers, greethttp.Header{Name: h.Name, Value: h.Value})
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(greethttp.ProtocolName, &greethttp.HTTPConfig{
			Target:               o.Target,
			Method:               o.Method,
			Headers:              headers,
			MaxResponseBodyBytes: o.MaxResponseBodyBytes,
			MaxRedirectFollow:    o.MaxRedirectFollow,
		}),
	}, nil
}

func init() {
	jsonapi.RegisterDataMapper(greethttp.ProtocolName, FromResult)
	jsonapi.RegisterOptionDecoder(greethttp.ProtocolName, DecodeOptions)
}
