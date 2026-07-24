package protocol

import (
	"errors"
	"testing"
)

// TestValidationError_StructuredReturn verifies that Validate() returns
// a *ValidationError (not a plain fmt.Errorf) so callers can inspect the
// error code, message, and details programmatically.
func TestValidationError_StructuredReturn(t *testing.T) {
	tests := []struct {
		name    string
		req     ProcessRequest
		wantCode ErrorCode
		wantField string
	}{
		{
			name:    "missing session_id",
			req:     ProcessRequest{},
			wantCode: ErrInvalidRequest,
			wantField: "session_id",
		},
		{
			name: "missing message.role",
			req: ProcessRequest{
				SessionID: "sess-001",
			},
			wantCode: ErrInvalidRequest,
			wantField: "message.role",
		},
		{
			name: "missing identity.platform",
			req: ProcessRequest{
				SessionID: "sess-001",
				Message:   Message{Role: "user"},
			},
			wantCode: ErrInvalidRequest,
			wantField: "identity.platform",
		},
		{
			name: "missing identity.chat_id",
			req: ProcessRequest{
				SessionID: "sess-001",
				Message:   Message{Role: "user"},
				Identity:  Identity{Platform: "telegram"},
			},
			wantCode: ErrInvalidRequest,
			wantField: "identity.chat_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not *ValidationError, got %T: %v", err, err)
			}

			if ve.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", ve.Code, tt.wantCode)
			}
			if ve.Message == "" {
				t.Error("Message is empty")
			}
			field, ok := ve.Details["field"]
			if !ok {
				t.Error("Details missing 'field' key")
			} else if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
		})
	}
}

func TestDecisionValidationError_StructuredReturn(t *testing.T) {
	tests := []struct {
		name    string
		d       Decision
		wantCode ErrorCode
		wantField string
	}{
		{
			name:    "missing decision_id",
			d:       Decision{Decision: DecisionText, Text: &TextResp{Content: "hi"}},
			wantCode: ErrInvalidDecision,
			wantField: "decision_id",
		},
		{
			name:    "tool_call missing name",
			d:       Decision{Decision: DecisionToolCall, DecisionID: "d1", ToolCall: &ToolCall{}},
			wantCode: ErrInvalidDecision,
			wantField: "tool_call.name",
		},
		{
			name:    "tool_call nil payload",
			d:       Decision{Decision: DecisionToolCall, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "tool_call",
		},
		{
			name:    "llm_call missing model",
			d:       Decision{Decision: DecisionLLMCall, DecisionID: "d1", LLMCall: &LLMCall{Messages: []LLMMessage{{Role: "user", Content: "hi"}}}},
			wantCode: ErrInvalidDecision,
			wantField: "llm_call.model",
		},
		{
			name:    "llm_call empty messages",
			d:       Decision{Decision: DecisionLLMCall, DecisionID: "d1", LLMCall: &LLMCall{Model: "m1"}},
			wantCode: ErrInvalidDecision,
			wantField: "llm_call.messages",
		},
		{
			name:    "llm_call nil payload",
			d:       Decision{Decision: DecisionLLMCall, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "llm_call",
		},
		{
			name:    "text missing content",
			d:       Decision{Decision: DecisionText, DecisionID: "d1", Text: &TextResp{}},
			wantCode: ErrInvalidDecision,
			wantField: "text.content",
		},
		{
			name:    "text nil payload",
			d:       Decision{Decision: DecisionText, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "text",
		},
		{
			name:    "wait missing reason",
			d:       Decision{Decision: DecisionWait, DecisionID: "d1", Wait: &Wait{}},
			wantCode: ErrInvalidDecision,
			wantField: "wait.reason",
		},
		{
			name:    "wait nil payload",
			d:       Decision{Decision: DecisionWait, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "wait",
		},
		{
			name:    "delegate missing task",
			d:       Decision{Decision: DecisionDelegate, DecisionID: "d1", Delegate: &Delegate{}},
			wantCode: ErrInvalidDecision,
			wantField: "delegate.task",
		},
		{
			name:    "delegate nil payload",
			d:       Decision{Decision: DecisionDelegate, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "delegate",
		},
		{
			name:    "end missing reason",
			d:       Decision{Decision: DecisionEnd, DecisionID: "d1", End: &End{}},
			wantCode: ErrInvalidDecision,
			wantField: "end.reason",
		},
		{
			name:    "end nil payload",
			d:       Decision{Decision: DecisionEnd, DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "end",
		},
		{
			name:    "unknown decision type",
			d:       Decision{Decision: "bogus", DecisionID: "d1"},
			wantCode: ErrInvalidDecision,
			wantField: "", // uses "decision_type" key instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.d.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not *ValidationError, got %T: %v", err, err)
			}

			if ve.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", ve.Code, tt.wantCode)
			}
			if ve.Message == "" {
				t.Error("Message is empty")
			}

			if tt.wantField != "" {
				field, ok := ve.Details["field"]
				if !ok {
					t.Error("Details missing 'field' key")
				} else if field != tt.wantField {
					t.Errorf("field = %q, want %q", field, tt.wantField)
				}
			}
		})
	}
}

func TestValidationError_ErrorMethod(t *testing.T) {
	ve := &ValidationError{
		Code:    ErrInvalidRequest,
		Message: "session_id is required",
		Details: map[string]any{"field": "session_id"},
	}
	if ve.Error() != "session_id is required" {
		t.Errorf("Error() = %q, want %q", ve.Error(), "session_id is required")
	}
}

func TestValidationError_NilValidate(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "user", Timestamp: "2026-01-01T00:00:00Z"},
		Identity:  Identity{Platform: "tg", ChatID: "1", UserName: "u", UserID: "1"},
		Context: Context{
			Config:       Config{MaxIterations: 1, TimeoutSeconds: 1},
			SessionState: SessionState{StartedAt: "2026-01-01T00:00:00Z"},
		},
	}
	if err := r.Validate(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
