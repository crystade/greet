package greet

// Generic TCP error codes shared by all TCP-based protocols.
const (
	ErrConnectionRefused  ErrorCode = "connection_refused"
	ErrConnectionTimeout  ErrorCode = "connection_timeout"
	ErrConnectionReset    ErrorCode = "connection_reset"
	ErrNetworkUnreachable ErrorCode = "network_unreachable"
	ErrConnectionFailed   ErrorCode = "connection_failed"
)
