// Package protocol defines the H3 wire-format types and validation logic.
package protocol

import "fmt"

// ValidationError is a structured error returned by Validate().
// It carries a machine-readable error code, a human-readable message,
// and optional details (e.g., which field failed).
type ValidationError struct {
	Code    ErrorCode
	Message string
	Details map[string]any
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Message
}

// newValidationError is a helper to create a ValidationError with standard details.
func newValidationError(code ErrorCode, field string, msg string) *ValidationError {
	return &ValidationError{
		Code:    code,
		Message: msg,
		Details: map[string]any{"field": field},
	}
}

// Validate checks that a ProcessRequest is well-formed.
// Required fields: session_id, message.role, identity.platform, identity.chat_id.
// Timestamp, user_name, user_id, and config values are optional per the H3
// protocol; the h3-test battery sends minimal requests without them.
func (r *ProcessRequest) Validate() error {
	if r.SessionID == "" {
		return newValidationError(ErrInvalidRequest, "session_id", "session_id is required")
	}
	if r.Message.Role == "" {
		return newValidationError(ErrInvalidRequest, "message.role", "message.role is required")
	}
	if r.Identity.Platform == "" {
		return newValidationError(ErrInvalidRequest, "identity.platform", "identity.platform is required")
	}
	if r.Identity.ChatID == "" {
		return newValidationError(ErrInvalidRequest, "identity.chat_id", "identity.chat_id is required")
	}
	return nil
}

// Validate checks that a Decision is well-formed.
func (d *Decision) Validate() error {
	if d.DecisionID == "" {
		return newValidationError(ErrInvalidDecision, "decision_id", "decision_id is required")
	}
	switch d.Decision {
	case DecisionToolCall:
		if d.ToolCall == nil {
			return newValidationError(ErrInvalidDecision, "tool_call", "tool_call is required for decision type 'tool_call'")
		}
		if d.ToolCall.Name == "" {
			return newValidationError(ErrInvalidDecision, "tool_call.name", "tool_call.name is required")
		}
	case DecisionLLMCall:
		if d.LLMCall == nil {
			return newValidationError(ErrInvalidDecision, "llm_call", "llm_call is required for decision type 'llm_call'")
		}
		if d.LLMCall.Model == "" {
			return newValidationError(ErrInvalidDecision, "llm_call.model", "llm_call.model is required")
		}
		if len(d.LLMCall.Messages) == 0 {
			return newValidationError(ErrInvalidDecision, "llm_call.messages", "llm_call.messages must have at least one message")
		}
	case DecisionText:
		if d.Text == nil {
			return newValidationError(ErrInvalidDecision, "text", "text is required for decision type 'text'")
		}
		if d.Text.Content == "" {
			return newValidationError(ErrInvalidDecision, "text.content", "text.content is required")
		}
	case DecisionWait:
		if d.Wait == nil {
			return newValidationError(ErrInvalidDecision, "wait", "wait is required for decision type 'wait'")
		}
		if d.Wait.Reason == "" {
			return newValidationError(ErrInvalidDecision, "wait.reason", "wait.reason is required")
		}
	case DecisionDelegate:
		if d.Delegate == nil {
			return newValidationError(ErrInvalidDecision, "delegate", "delegate is required for decision type 'delegate'")
		}
		if d.Delegate.Task == "" {
			return newValidationError(ErrInvalidDecision, "delegate.task", "delegate.task is required")
		}
	case DecisionEnd:
		if d.End == nil {
			return newValidationError(ErrInvalidDecision, "end", "end is required for decision type 'end'")
		}
		if d.End.Reason == "" {
			return newValidationError(ErrInvalidDecision, "end.reason", "end.reason is required")
		}
	default:
		return &ValidationError{
			Code:    ErrInvalidDecision,
			Message: fmt.Sprintf("unknown decision type: %q", d.Decision),
			Details: map[string]any{"decision_type": string(d.Decision)},
		}
	}
	return nil
}
