package http

import (
	"github.com/crystade/greet"
)

const (
	ErrHTTPRequestFailed       greet.ErrorCode = "http_request_failed"
	ErrHTTPResponseFailed      greet.ErrorCode = "http_response_failed"
	ErrHTTPTimeout             greet.ErrorCode = "http_timeout"
	ErrHTTPRedirectSSRF        greet.ErrorCode = "http_redirect_ssrf"
	ErrHTTPTooManyRedirects    greet.ErrorCode = "http_too_many_redirects"
	ErrHTTPRedirectCrossOrigin greet.ErrorCode = "http_redirect_cross_origin"
)
