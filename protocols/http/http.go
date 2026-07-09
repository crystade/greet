package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/crystade/greet"
)

// ProtocolName is the registered name for the HTTP protocol.
const ProtocolName = "http"

// HTTP implements an HTTP/HTTPS application-layer probe.
// It sends a configurable request over the provided conn and captures
// the response status, headers, and a bounded slice of the body.
type HTTP struct{}

// New creates a new HTTP application layer.
func New() *HTTP {
	return &HTTP{}
}

func (h *HTTP) Name() string                   { return ProtocolName }
func (h *HTTP) Description() string            { return "HTTP request/response probe" }
func (h *HTTP) DefaultPort() int               { return DefaultHTTPPort }
func (h *HTTP) BaseTransport() greet.Transport { return greet.TransportTCP }

// Handshake performs the HTTP request/response exchange over conn.
// conn may be a plain *net.TCPConn or a *tls.Conn (for https).
// The upgraded conn (same as input — HTTP does not wrap the connection) is
// returned so subsequent layers (if any) could reuse it; in practice HTTP is
// always the outermost layer.
func (h *HTTP) Handshake(ctx context.Context, start time.Time, conn net.Conn, host string, port int, cfg *greet.GreetConfig) (net.Conn, greet.LayerData, greet.LayerTiming, error) {
	// --- resolve config ---
	httpCfg := &HTTPConfig{}
	if raw := cfg.LayerConfig(ProtocolName); raw != nil {
		c, ok := raw.(*HTTPConfig)
		if !ok {
			return nil, nil, greet.LayerTiming{}, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid layer config: expected *HTTPConfig, got %T", raw),
				Protocol: ProtocolName,
			}
		}
		httpCfg = c
	}

	method := strings.ToUpper(httpCfg.Method)
	if method == "" {
		method = "GET"
	}

	maxBody := httpCfg.MaxResponseBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxResponseBodyBytes
	}

	maxRedirectFollow := httpCfg.MaxRedirectFollow
	if maxRedirectFollow < 0 {
		maxRedirectFollow = DefaultMaxRedirectFollow
	}

	redirectValidator := httpCfg.RedirectValidator
	if redirectValidator == nil {
		redirectValidator = DefaultRedirectValidator
	}

	// --- resolve and validate target ---
	target := httpCfg.Target
	if target == "" {
		target = "/"
	}

	// Target must be origin-form: /path or /path?query.
	if !strings.HasPrefix(target, "/") {
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrHTTPRequestFailed,
			Message:  fmt.Sprintf("target %q must be origin-form (e.g. / or /path?query)", target),
			Protocol: ProtocolName,
		}
	}

	// --- build first request ---
	secure := isSecureConn(conn)
	scheme := "http"
	if secure {
		scheme = "https"
	}
	originalMethod := method
	rawURL := fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)), target)
	currentReq, parseErr := nethttp.NewRequestWithContext(ctx, method, rawURL, httpCfg.BodyReader)
	if parseErr != nil {
		return nil, nil, greet.LayerTiming{}, &greet.GreetError{
			Code:     ErrHTTPRequestFailed,
			Message:  fmt.Sprintf("failed to build HTTP request: %v", parseErr),
			Protocol: ProtocolName,
			Cause:    parseErr,
		}
	}

	// Apply caller-supplied headers (preserve case and duplicates).
	for _, hdr := range httpCfg.Headers {
		currentReq.Header.Add(hdr.Name, hdr.Value)
	}

	// Ensure Host header is set correctly.
	setHost(currentReq, secure, host, port)

	// --- redirect loop ---
	var (
		redirects     []RedirectEntry
		br            = bufio.NewReader(conn)
		currentMethod = method
		currentBody   = httpCfg.BodyReader
		ttfb          time.Duration
	)

	for redirectCount := 0; ; redirectCount++ {
		// Set deadline from context.
		if deadline, ok := ctx.Deadline(); ok {
			conn.SetDeadline(deadline) //nolint:errcheck
		}

		// --- send request ---
		reqHost := currentReq.URL.Host
		if writeErr := currentReq.Write(conn); writeErr != nil {
			var netErr net.Error
			if errors.As(writeErr, &netErr) && netErr.Timeout() {
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPTimeout,
					Message:  fmt.Sprintf("timed out sending HTTP request to %s", reqHost),
					Protocol: ProtocolName,
					Cause:    writeErr,
				}
			}
			return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
				Code:     ErrHTTPRequestFailed,
				Message:  fmt.Sprintf("failed to send HTTP request to %s: %v", reqHost, writeErr),
				Protocol: ProtocolName,
				Cause:    writeErr,
			}
		}

		// --- read response ---
		resp, readErr := nethttp.ReadResponse(br, currentReq)
		if redirectCount == 0 {
			ttfb = time.Since(start) // TTFB captured once from first response
		}
		if readErr != nil {
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPTimeout,
					Message:  fmt.Sprintf("timed out reading HTTP response from %s", reqHost),
					Protocol: ProtocolName,
					Cause:    readErr,
				}
			}
			return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
				Code:     ErrHTTPResponseFailed,
				Message:  fmt.Sprintf("failed to read HTTP response from %s: %v", reqHost, readErr),
				Protocol: ProtocolName,
				Cause:    readErr,
			}
		}

		// --- check whether to follow a redirect ---
		if isRedirect(resp.StatusCode) && resp.Header.Get("Location") != "" && redirectCount < maxRedirectFollow {
			location := resp.Header.Get("Location")

			// Resolve relative to current request URL.
			redirectURL, resolveErr := currentReq.URL.Parse(location)
			if resolveErr != nil {
				resp.Body.Close()
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPRequestFailed,
					Message:  fmt.Sprintf("failed to resolve redirect Location %q: %v", location, resolveErr),
					Protocol: ProtocolName,
					Cause:    resolveErr,
				}
			}
			// Same-origin, same-scheme gate: we reuse the same TCP/TLS
			// connection, so cross-origin or cross-scheme redirects are
			// physically impossible to follow.
			if !isSameOrigin(redirectURL, secure, host, port) {
				resp.Body.Close()
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPRedirectCrossOrigin,
					Message:  fmt.Sprintf("redirect to %q blocked: cross-origin redirects are not supported (reuse connection)", redirectURL.String()),
					Protocol: ProtocolName,
				}
			}
			// Validate redirect target (SSRF protection — only applied to
			// redirect targets, never the original user-specified URL).
			if err := redirectValidator(redirectURL.String()); err != nil {
				resp.Body.Close()
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPRedirectSSRF,
					Message:  fmt.Sprintf("redirect blocked: %v", err),
					Protocol: ProtocolName,
					Cause:    err,
				}
			}

			// Snapshot the method that produced this redirect response
			// before mutating it for the next hop.
			redirectMethod := currentMethod

			// Determine next method per HTTP semantics
			// (RFC 7231 §6.4 / RFC 7538 §3).
			switch resp.StatusCode {
			case 301, 302, 303:
				currentMethod = "GET"
				currentBody = nil
			case 307, 308:
				currentMethod = originalMethod
				currentBody = nil // body already consumed on first request
			}

			// Collect response headers from the redirect response.
			var redirectRespHeaders []Header
			for name, values := range resp.Header {
				for _, v := range values {
					redirectRespHeaders = append(redirectRespHeaders, Header{Name: name, Value: v})
				}
			}

			redirects = append(redirects, RedirectEntry{
				URL:             redirectURL.String(),
				StatusCode:      resp.StatusCode,
				Method:          redirectMethod,
				ResponseHeaders: redirectRespHeaders,
			})

			// Drain and close the redirect response body before reusing
			// the connection for the next request.
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			// Build the redirect request.
			currentReq, parseErr = nethttp.NewRequestWithContext(ctx, currentMethod, redirectURL.String(), currentBody)
			if parseErr != nil {
				return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
					Code:     ErrHTTPRequestFailed,
					Message:  fmt.Sprintf("failed to build redirect request: %v", parseErr),
					Protocol: ProtocolName,
					Cause:    parseErr,
				}
			}

			// Re-apply caller-supplied headers to the redirect request.
			for _, hdr := range httpCfg.Headers {
				currentReq.Header.Add(hdr.Name, hdr.Value)
			}

			// Set Host header for the redirect target.
			setHostFromURL(currentReq, redirectURL)

			continue
		}

		// --- redirect limit exceeded ---
		if isRedirect(resp.StatusCode) && maxRedirectFollow > 0 {
			resp.Body.Close()
			return nil, nil, greet.LayerTiming{TTFB: ttfb}, &greet.GreetError{
				Code:     ErrHTTPTooManyRedirects,
				Message:  fmt.Sprintf("too many redirects (%d followed, limit %d)", redirectCount, maxRedirectFollow),
				Protocol: ProtocolName,
			}
		}

		// --- final response: buffer body ---
		defer resp.Body.Close()
		bodyBuf := &bytes.Buffer{}
		_, copyErr := io.Copy(bodyBuf, io.LimitReader(resp.Body, maxBody))
		ttlb := time.Since(start)
		if copyErr != nil {
			var netErr net.Error
			if errors.As(copyErr, &netErr) && netErr.Timeout() {
				return nil, nil, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, &greet.GreetError{
					Code:     ErrHTTPTimeout,
					Message:  fmt.Sprintf("timed out reading HTTP response body from %s", reqHost),
					Protocol: ProtocolName,
					Cause:    copyErr,
				}
			}
			return nil, nil, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, &greet.GreetError{
				Code:     ErrHTTPResponseFailed,
				Message:  fmt.Sprintf("failed to read HTTP response body from %s: %v", reqHost, copyErr),
				Protocol: ProtocolName,
				Cause:    copyErr,
			}
		}

		// --- collect response headers ---
		var respHeaders []Header
		for name, values := range resp.Header {
			for _, v := range values {
				respHeaders = append(respHeaders, Header{Name: name, Value: v})
			}
		}

		result := &HTTPResult{
			ProtocolVersion: resp.Proto,
			StatusCode:      resp.StatusCode,
			Status:          resp.Status,
			ResponseHeaders: respHeaders,
			Body:            bytes.NewReader(bodyBuf.Bytes()),
			Redirects:       redirects,
		}

		return conn, result, greet.LayerTiming{TTFB: ttfb, TTLB: ttlb}, nil
	}
}

// isRedirect reports whether statusCode is a 3xx redirect status.
func isRedirect(statusCode int) bool {
	return statusCode >= 300 && statusCode < 400
}

// setHost sets the Host header on req based on scheme, host, and port.
func setHost(req *nethttp.Request, secure bool, host string, port int) {
	if (secure && port == 443) || (!secure && port == 80) {
		req.Host = host
	} else {
		req.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
}

// setHostFromURL sets the Host header on req from redirectURL. Omit default
// ports (80/443) to match the behaviour of setHost.
func setHostFromURL(req *nethttp.Request, redirectURL *url.URL) {
	if redirectURL.Port() == "" {
		req.Host = redirectURL.Hostname()
	} else {
		port, _ := strconv.Atoi(redirectURL.Port())
		if (redirectURL.Scheme == "https" && port == 443) || (redirectURL.Scheme == "http" && port == 80) {
			req.Host = redirectURL.Hostname()
		} else {
			req.Host = redirectURL.Host
		}
	}
}

// isSameOrigin reports whether redirectURL shares the same scheme, host, and
// port as the original connection, which is required for connection reuse.
func isSameOrigin(redirectURL *url.URL, secure bool, host string, port int) bool {
	redirectScheme := redirectURL.Scheme
	expectedScheme := "http"
	if secure {
		expectedScheme = "https"
	}
	if redirectScheme != expectedScheme {
		return false
	}

	if redirectURL.Hostname() != host {
		return false
	}

	redirectPort := portFromURL(redirectURL, secure)
	return redirectPort == port
}

// portFromURL returns the effective port for a URL, defaulting to 80 for http
// and 443 for https when no explicit port is present.
func portFromURL(u *url.URL, secure bool) int {
	if u.Port() != "" {
		p, _ := strconv.Atoi(u.Port())
		return p
	}
	if secure {
		return 443
	}
	return 80
}

// isSecureConn returns true when conn is a *tls.Conn (i.e. TLS is active).
func isSecureConn(conn net.Conn) bool {
	_, ok := conn.(*tls.Conn)
	return ok
}
