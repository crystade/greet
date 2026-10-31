package redis

import "github.com/crystade/greet"

const (
	ErrRedisSendFailed         greet.ErrorCode = "send_failed"
	ErrRedisTimeout            greet.ErrorCode = "timeout"
	ErrRedisUnexpectedResponse greet.ErrorCode = "unexpected_response"
	ErrRedisMalformedResponse  greet.ErrorCode = "malformed_response"
)
