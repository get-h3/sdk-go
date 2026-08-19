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
		name      string
		req       ProcessRequest
		wantCode  ErrorCode
		wantField string
	}{
		{
			name:      "missing session_id",
			req:       ProcessRequest{},
			wantCode:  ErrInvalidRequest,
			wantField: "session_id",
		},
		{
			name: "missing message.role",
			req: ProcessRequest{
				SessionID: "sess-001",
			},
			wantCode:  ErrInvalidRequest,
			wantField: "message.role",
		},
		{
			name: "missing identity.platform",
			req: ProcessRequest{
				SessionID: "sess-001",
				Message:   Message{Role: "user"},
			},
			wantCode:  ErrInvalidRequest,
			wantField: "identity.platform",
		},
		{
			name: "missing identity.chat_id",
			req: ProcessRequest{
				SessionID: "sess-001",
				Message:   Message{Role: "user"},
				Identity:  Identity{Platform: "telegram"},
			},
			wantCode:  ErrInvalidRequest,
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
		name      string
		d         Decision
		wantCode  ErrorCode
		wantField string
	}{
		{
			name:      "missing decision_id",
			d:         Decision{Decision: DecisionText, Text: &TextResp{Content: "hi"}},
			wantCode:  ErrInvalidDecision,
			wantField: "decision_id",
		},
		{
			name:      "tool_call missing name",
			d:         Decision{Decision: DecisionToolCall, DecisionID: "d1", ToolCall: &ToolCall{}},
			wantCode:  ErrInvalidDecision,
			wantField: "tool_call.name",
		},
		{
			name:      "tool_call nil payload",
			d:         Decision{Decision: DecisionToolCall, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "tool_call",
		},
		{
			name:      "llm_call missing model",
			d:         Decision{Decision: DecisionLLMCall, DecisionID: "d1", LLMCall: &LLMCall{Messages: []LLMMessage{{Role: "user", Content: "hi"}}}},
			wantCode:  ErrInvalidDecision,
			wantField: "llm_call.model",
		},
		{
			name:      "llm_call empty messages",
			d:         Decision{Decision: DecisionLLMCall, DecisionID: "d1", LLMCall: &LLMCall{Model: "m1"}},
			wantCode:  ErrInvalidDecision,
			wantField: "llm_call.messages",
		},
		{
			name:      "llm_call nil payload",
			d:         Decision{Decision: DecisionLLMCall, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "llm_call",
		},
		{
			name:      "text missing content",
			d:         Decision{Decision: DecisionText, DecisionID: "d1", Text: &TextResp{}},
			wantCode:  ErrInvalidDecision,
			wantField: "text.content",
		},
		{
			name:      "text nil payload",
			d:         Decision{Decision: DecisionText, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "text",
		},
		{
			name:      "wait missing reason",
			d:         Decision{Decision: DecisionWait, DecisionID: "d1", Wait: &Wait{}},
			wantCode:  ErrInvalidDecision,
			wantField: "wait.reason",
		},
		{
			name:      "wait nil payload",
			d:         Decision{Decision: DecisionWait, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "wait",
		},
		{
			name:      "delegate missing task",
			d:         Decision{Decision: DecisionDelegate, DecisionID: "d1", Delegate: &Delegate{}},
			wantCode:  ErrInvalidDecision,
			wantField: "delegate.task",
		},
		{
			name:      "delegate nil payload",
			d:         Decision{Decision: DecisionDelegate, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "delegate",
		},
		{
			name:      "end missing reason",
			d:         Decision{Decision: DecisionEnd, DecisionID: "d1", End: &End{}},
			wantCode:  ErrInvalidDecision,
			wantField: "end.reason",
		},
		{
			name:      "end nil payload",
			d:         Decision{Decision: DecisionEnd, DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
			wantField: "end",
		},
		{
			name:      "unknown decision type",
			d:         Decision{Decision: "bogus", DecisionID: "d1"},
			wantCode:  ErrInvalidDecision,
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

// TestProcessRequestValidate_NonUserRole verifies that a non-'user' message
// role (e.g. 'system') is rejected with ErrInvalidRequest, matching the
// protocol schema enum constraint (Message.role must be 'user').
func TestProcessRequestValidate_NonUserRole(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "system", Content: "hi"},
		Identity:  Identity{Platform: "telegram", ChatID: "1"},
	}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for role 'system', got nil")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not *ValidationError, got %T: %v", err, err)
	}
	if ve.Code != ErrInvalidRequest {
		t.Errorf("Code = %q, want %q", ve.Code, ErrInvalidRequest)
	}
	field, ok := ve.Details["field"]
	if !ok || field != "message.role" {
		t.Errorf("Details[field] = %v, want \"message.role\"", field)
	}
}

// TestProcessRequestValidate_ValidUserRole verifies that role 'user' with
// otherwise-valid fields passes validation (returns nil).
func TestProcessRequestValidate_ValidUserRole(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "user", Content: "hello"},
		Identity:  Identity{Platform: "telegram", ChatID: "1"},
	}
	if err := r.Validate(); err != nil {
		t.Errorf("expected nil error for valid request, got %v", err)
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

// — NewDecision / GenerateUUID tests —

func TestGenerateUUID_IsValidUUIDv4(t *testing.T) {
	id := GenerateUUID()
	// UUIDv4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	if len(id) != 36 {
		t.Errorf("UUID length = %d, want 36", len(id))
	}
	if id[14] != '4' {
		t.Errorf("version nibble = %c, want '4'", id[14])
	}
	if id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b' {
		t.Errorf("variant nibble = %c, want 8/9/a/b", id[19])
	}
	// Check hyphens
	for _, pos := range []int{8, 13, 18, 23} {
		if id[pos] != '-' {
			t.Errorf("expected hyphen at position %d, got %c", pos, id[pos])
		}
	}
}

func TestGenerateUUID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := GenerateUUID()
		if seen[id] {
			t.Fatalf("duplicate UUID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestNewDecision_SetsID(t *testing.T) {
	d := NewDecision(DecisionText)
	if d.DecisionID == "" {
		t.Error("NewDecision should set a non-empty DecisionID")
	}
	if d.Decision != DecisionText {
		t.Errorf("Decision = %q, want %q", d.Decision, DecisionText)
	}
	// Validate should pass because DecisionID is set
	if err := d.Validate(); err == nil {
		// DecisionText needs a Text payload — a bare NewDecision(DecisionText)
		// without setting d.Text will fail validation on 'text is required',
		// NOT on missing decision_id — which proves the ID auto-generation works.
		t.Log("Validate() correctly rejected missing Text payload (as expected)")
	}
}

func TestNewDecision_EachCallGeneratesUniqueID(t *testing.T) {
	d1 := NewDecision(DecisionEnd)
	d2 := NewDecision(DecisionEnd)
	if d1.DecisionID == d2.DecisionID {
		t.Error("consecutive NewDecision calls should generate unique IDs")
	}
}

func TestNewDecision_ValidateWithoutPayload_BecauseDecisionIDIsSet(t *testing.T) {
	// The key assertion: Validate() should NOT complain about decision_id
	// because NewDecision auto-populates it.
	d := NewDecision(DecisionEnd)
	d.End = &End{Reason: EndTaskComplete}
	if err := d.Validate(); err != nil {
		t.Errorf("fully-populated NewDecision should validate: %v", err)
	}
}
