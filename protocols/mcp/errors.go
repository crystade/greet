package mcp

import "github.com/crystade/greet"

// MCP-specific error codes.
const (
	ErrMCPInitializeFailed    greet.ErrorCode = "mcp_initialize_failed"
	ErrMCPUnexpectedStatus    greet.ErrorCode = "mcp_unexpected_status"
	ErrMCPMissingSession      greet.ErrorCode = "mcp_missing_session"
	ErrMCPAuthFailed          greet.ErrorCode = "mcp_auth_failed"
	ErrMCPUnsupportedOAuth    greet.ErrorCode = "mcp_unsupported_oauth"
	ErrMCPListResourcesFailed greet.ErrorCode = "mcp_list_resources_failed"
	ErrMCPListToolsFailed     greet.ErrorCode = "mcp_list_tools_failed"
	ErrMCPListPromptsFailed   greet.ErrorCode = "mcp_list_prompts_failed"
	ErrMCPListTemplatesFailed greet.ErrorCode = "mcp_list_templates_failed"
)

// newError builds a GreetError for the MCP layer.
func newError(code greet.ErrorCode, message string, cause error) *greet.GreetError {
	return &greet.GreetError{
		Code:     code,
		Message:  message,
		Protocol: ProtocolName,
		Cause:    cause,
	}
}
