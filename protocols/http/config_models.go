package http

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
)

// DefaultHTTPPort is the well-known port for HTTP.
const DefaultHTTPPort = 80

// DefaultMaxResponseBodyBytes is the default body read limit (1 MiB).
const DefaultMaxResponseBodyBytes = 1 << 20

// DefaultMaxRedirectFollow is the default redirect-follow limit. 0 means
// no redirects are followed.
const DefaultMaxRedirectFollow = 0

// Header is a single HTTP header name/value pair. HTTP header names are
// case-insensitive and may be duplicated; an ordered slice preserves both.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HTTPConfig holds per-layer configuration for the HTTP layer.
type HTTPConfig struct {
	// Target is the request-target: either an origin-form ("/path?query")
	// or an absolute URL ("https://example.com/path?query").
	// Defaults to "/" when empty.
	Target string

	// Method is the HTTP method. Case-insensitive; defaults to "GET".
	Method string

	// Headers are additional request headers to send.
	Headers []Header

	// MaxResponseBodyBytes caps how many bytes of the response body are
	// buffered. 0 uses DefaultMaxResponseBodyBytes.
	MaxResponseBodyBytes int64

	// BodyReader, if non-nil, is written as the request body.
	BodyReader io.Reader

	// MaxRedirectFollow is the maximum number of redirects to follow.
	// 0 (DefaultMaxRedirectFollow) means no redirects are followed;
	// the original 3xx response is returned as-is.
	MaxRedirectFollow int

	// RedirectValidator is called with each redirect Location URL before
	// following it. If nil, DefaultRedirectValidator is used. Return an
	// error to reject the redirect. This is only invoked on redirect
	// targets, never on the original user-specified target.
	RedirectValidator func(locationURL string) error
}

// RedirectEntry records a single redirect hop that was followed.
type RedirectEntry struct {
	// URL is the full absolute URL that was followed.
	URL string

	// StatusCode is the 3xx status code that triggered this redirect.
	StatusCode int

	// Method is the HTTP method used for the redirected request.
	Method string

	// ResponseHeaders are the response headers from the 3xx redirect
	// response, in the order received.
	ResponseHeaders []Header
}

// HTTPResult holds the outcome of an HTTP request.
type HTTPResult struct {
	// ProtocolVersion is the HTTP version from the response status line, e.g. "HTTP/1.1".
	ProtocolVersion string

	// StatusCode is the numeric HTTP status code, e.g. 200.
	StatusCode int

	// Status is the full status text, e.g. "200 OK".
	Status string

	// ResponseHeaders are the response headers in the order received.
	ResponseHeaders []Header

	// Body is the buffered response body, limited to MaxResponseBodyBytes.
	// Only available in the Go API; the JSON API omits it.
	Body *bytes.Reader

	// Redirects holds the chain of redirects that were followed, in order.
	// Empty when no redirects occurred (either because the final status was
	// not 3xx, or MaxRedirectFollow was 0).
	Redirects []RedirectEntry
}

// DefaultRedirectValidator validates that a redirect Location URL is
// parseable and uses an http or https scheme. It does not perform DNS
// resolution or private-IP checks — those are unnecessary because only
// same-origin redirects are followed on the reused connection.
//
// This validator is ONLY applied to redirect Location URLs, never to the
// original user-specified target.
func DefaultRedirectValidator(locationURL string) error {
	u, err := url.Parse(locationURL)
	if err != nil {
		return fmt.Errorf("invalid redirect URL %q: %w", locationURL, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect to %q blocked: scheme %q is not allowed (only http/https)", locationURL, u.Scheme)
	}

	return nil
}
