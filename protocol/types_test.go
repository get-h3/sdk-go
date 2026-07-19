package protocol

import (
	"encoding/json"
	"testing"
)

func TestDecisionRoundTrip_Text(t *testing.T) {
	orig := Decision{
		Decision:   DecisionText,
		DecisionID: "dec-001",
		Text: &TextResp{
			Content:  "Hello, world!",
			Finished: true,
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Decision != DecisionText {
		t.Errorf("decision = %q, want %q", parsed.Decision, DecisionText)
	}
	if parsed.DecisionID != "dec-001" {
		t.Errorf("decision_id = %q, want dec-001", parsed.DecisionID)
	}
	if parsed.Text == nil {
		t.Fatal("text is nil")
	}
	if parsed.Text.Content != "Hello, world!" {
		t.Errorf("text.content = %q", parsed.Text.Content)
	}
	if !parsed.Text.Finished {
		t.Error("text.finished should be true")
	}

	// Verify unused variants are nil
	if parsed.ToolCall != nil || parsed.LLMCall != nil || parsed.Wait != nil || parsed.Delegate != nil || parsed.End != nil {
		t.Error("unused decision variants should be nil")
	}
}

func TestDecisionRoundTrip_ToolCall(t *testing.T) {
	orig := Decision{
		Decision:   DecisionToolCall,
		DecisionID: "dec-002",
		ToolCall: &ToolCall{
			Name: "read_file",
			Params: map[string]any{
				"path": "/tmp/test.txt",
			},
			Reasoning: "Need to read the config file",
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.ToolCall == nil {
		t.Fatal("tool_call is nil")
	}
	if parsed.ToolCall.Name != "read_file" {
		t.Errorf("name = %q", parsed.ToolCall.Name)
	}
	if parsed.ToolCall.Reasoning != "Need to read the config file" {
		t.Errorf("reasoning = %q", parsed.ToolCall.Reasoning)
	}
}

func TestDecisionRoundTrip_LLMCall(t *testing.T) {
	orig := Decision{
		Decision:   DecisionLLMCall,
		DecisionID: "dec-003",
		LLMCall: &LLMCall{
			Model:        "deepseek-v4-pro",
			SystemPrompt: "You are helpful.",
			Messages: []LLMMessage{
				{Role: "user", Content: "Hello"},
			},
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.LLMCall == nil {
		t.Fatal("llm_call is nil")
	}
	if parsed.LLMCall.Model != "deepseek-v4-pro" {
		t.Errorf("model = %q", parsed.LLMCall.Model)
	}
	if len(parsed.LLMCall.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(parsed.LLMCall.Messages))
	}
}

func TestDecisionRoundTrip_Wait(t *testing.T) {
	dur := 30
	orig := Decision{
		Decision:   DecisionWait,
		DecisionID: "dec-004",
		Wait: &Wait{
			Reason:          "Waiting for file upload",
			DurationSeconds: &dur,
			PollEndpoint:    "https://example.com/status",
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Wait == nil {
		t.Fatal("wait is nil")
	}
	if parsed.Wait.Reason != "Waiting for file upload" {
		t.Errorf("reason = %q", parsed.Wait.Reason)
	}
	if parsed.Wait.DurationSeconds == nil || *parsed.Wait.DurationSeconds != 30 {
		t.Errorf("duration_seconds = %v", parsed.Wait.DurationSeconds)
	}
	if parsed.Wait.PollEndpoint != "https://example.com/status" {
		t.Errorf("poll_endpoint = %q", parsed.Wait.PollEndpoint)
	}
}

func TestDecisionRoundTrip_Delegate(t *testing.T) {
	orig := Decision{
		Decision:   DecisionDelegate,
		DecisionID: "dec-005",
		Delegate: &Delegate{
			Agent:    "code-reviewer",
			Task:     "Review the auth module",
			Context:  "Focus on SQL injection",
			Model:    "deepseek-v4-flash",
			Provider: "opencode-go",
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Delegate == nil {
		t.Fatal("delegate is nil")
	}
	if parsed.Delegate.Task != "Review the auth module" {
		t.Errorf("task = %q", parsed.Delegate.Task)
	}
	if parsed.Delegate.Agent != "code-reviewer" {
		t.Errorf("agent = %q", parsed.Delegate.Agent)
	}
}

func TestDecisionRoundTrip_End(t *testing.T) {
	orig := Decision{
		Decision:   DecisionEnd,
		DecisionID: "dec-006",
		End: &End{
			Reason:  EndTaskComplete,
			Summary: "All tasks finished",
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.End == nil {
		t.Fatal("end is nil")
	}
	if parsed.End.Reason != EndTaskComplete {
		t.Errorf("reason = %q", parsed.End.Reason)
	}
	if parsed.End.Summary != "All tasks finished" {
		t.Errorf("summary = %q", parsed.End.Summary)
	}
}

func TestProcessRequestRoundTrip(t *testing.T) {
	orig := ProcessRequest{
		SessionID: "sess-001",
		Message: Message{
			Role:    "user",
			Content: "What is the weather?",
			Attachments: []Attachment{
				{Type: AttachmentImage, URL: "https://example.com/img.png", MimeType: "image/png"},
			},
			Timestamp: "2026-07-14T00:00:00Z",
		},
		Identity: Identity{
			Platform: "telegram",
			ChatID:   "-1001234567890",
			ThreadID: "12345",
			UserName: "testuser",
			UserID:   "987654",
		},
		Context: Context{
			History: []HistoryEntry{
				{Role: RoleUser, Content: "Hi"},
				{Role: RoleAssistant, Content: "Hello!"},
			},
			Tools: []Tool{
				{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"path": map[string]any{"type": "string"}}},
			},
			Models: []Model{
				{Name: "deepseek-v4-flash", Provider: "deepseek", ContextWindow: 1000000},
			},
			Memory: "user prefers concise answers",
			Skills: []string{"coding-hermes-foreman"},
			Config: Config{
				MaxIterations:  100,
				TimeoutSeconds: 300,
				ProjectDir:     "/home/test/project",
			},
			SessionState: SessionState{
				TurnCount:      0,
				TotalToolCalls: 0,
				TotalLLMCalls:  0,
				CostSoFar:      0.0,
				StartedAt:      "2026-07-14T00:00:00Z",
			},
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed ProcessRequest
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.SessionID != "sess-001" {
		t.Errorf("session_id = %q", parsed.SessionID)
	}
	if parsed.Message.Role != "user" {
		t.Errorf("message.role = %q", parsed.Message.Role)
	}
	if len(parsed.Message.Attachments) != 1 {
		t.Errorf("attachments len = %d", len(parsed.Message.Attachments))
	}
	if parsed.Identity.Platform != "telegram" {
		t.Errorf("identity.platform = %q", parsed.Identity.Platform)
	}
	if len(parsed.Context.History) != 2 {
		t.Errorf("history len = %d", len(parsed.Context.History))
	}
	if parsed.Context.Config.MaxIterations != 100 {
		t.Errorf("max_iterations = %d", parsed.Context.Config.MaxIterations)
	}
	if parsed.Context.SessionState.TurnCount != 0 {
		t.Errorf("turn_count = %d", parsed.Context.SessionState.TurnCount)
	}
}

func TestResultRequestRoundTrip(t *testing.T) {
	orig := ResultRequest{
		SessionID:  "sess-001",
		DecisionID: "dec-001",
		Result: Result{
			Type:       ResultTool,
			ToolName:   "read_file",
			Data:       map[string]any{"content": "file contents here"},
			DurationMs: 150,
			Success:    true,
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed ResultRequest
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Result.Type != ResultTool {
		t.Errorf("result.type = %q", parsed.Result.Type)
	}
	if !parsed.Result.Success {
		t.Error("result.success should be true")
	}
	if parsed.Result.DurationMs != 150 {
		t.Errorf("duration_ms = %.0f", parsed.Result.DurationMs)
	}
}

func TestCancelRequestRoundTrip(t *testing.T) {
	orig := CancelRequest{
		SessionID: "sess-001",
		Reason:    CancelUserInterrupt,
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed CancelRequest
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Reason != CancelUserInterrupt {
		t.Errorf("reason = %q", parsed.Reason)
	}
}

func TestHealthResponseRoundTrip(t *testing.T) {
	orig := HealthResponse{
		Status:          HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		UptimeSeconds:   3600,
		ActiveSessions:  5,
		Capabilities:    []DecisionType{DecisionText, DecisionToolCall},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed HealthResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Status != HealthOK {
		t.Errorf("status = %q", parsed.Status)
	}
	if parsed.UptimeSeconds != 3600 {
		t.Errorf("uptime_seconds = %d", parsed.UptimeSeconds)
	}
	if len(parsed.Capabilities) != 2 {
		t.Errorf("capabilities len = %d", len(parsed.Capabilities))
	}
}

func TestSessionResponseRoundTrip(t *testing.T) {
	orig := SessionResponse{
		SessionID:           "sess-001",
		StartedAt:           "2026-07-14T00:00:00Z",
		LastActive:          "2026-07-14T01:00:00Z",
		TurnCount:           5,
		Status:              SessionActive,
		CurrentDecision:     "dec-003",
		CurrentDecisionType: DecisionLLMCall,
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed SessionResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Status != SessionActive {
		t.Errorf("status = %q", parsed.Status)
	}
	if parsed.TurnCount != 5 {
		t.Errorf("turn_count = %d", parsed.TurnCount)
	}
	if parsed.CurrentDecisionType != DecisionLLMCall {
		t.Errorf("current_decision_type = %q", parsed.CurrentDecisionType)
	}
}

func TestErrorResponseRoundTrip(t *testing.T) {
	orig := ErrorResponse{
		Error: ErrorDetail{
			Code:    ErrSessionNotFound,
			Message: "Session sess-999 not found",
			Details: map[string]any{"session_id": "sess-999"},
		},
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed ErrorResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.Error.Code != ErrSessionNotFound {
		t.Errorf("code = %q", parsed.Error.Code)
	}
	if parsed.Error.Message != "Session sess-999 not found" {
		t.Errorf("message = %q", parsed.Error.Message)
	}
	if parsed.Error.Details["session_id"] != "sess-999" {
		t.Errorf("details.session_id = %v", parsed.Error.Details["session_id"])
	}
}

// — Validation tests —

func TestProcessRequestValidate_Valid(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message: Message{
			Role:      "user",
			Content:   "Hello",
			Timestamp: "2026-07-14T00:00:00Z",
		},
		Identity: Identity{
			Platform: "telegram",
			ChatID:   "-100123",
			UserName: "test",
			UserID:   "456",
		},
		Context: Context{
			History: []HistoryEntry{},
			Tools:   []Tool{},
			Models:  []Model{},
			Config: Config{
				MaxIterations:  10,
				TimeoutSeconds: 60,
			},
			SessionState: SessionState{
				StartedAt: "2026-07-14T00:00:00Z",
			},
		},
	}
	if err := r.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestProcessRequestValidate_MissingSessionID(t *testing.T) {
	r := ProcessRequest{}
	if err := r.Validate(); err == nil {
		t.Error("expected error for missing session_id")
	}
}

func TestProcessRequestValidate_MissingMessageContent(t *testing.T) {
	// Empty content is valid — harness handles it gracefully (per h3-test §5.4).
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
		t.Errorf("empty content should be valid: %v", err)
	}
}

func TestDecisionValidate_InvalidType(t *testing.T) {
	d := Decision{
		Decision:   "invalid",
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for invalid decision type")
	}
}

func TestDecisionValidate_MissingToolCall(t *testing.T) {
	d := Decision{
		Decision:   DecisionToolCall,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for tool_call decision without tool_call payload")
	}
}

func TestDecisionValidate_MissingLLMCallMessages(t *testing.T) {
	d := Decision{
		Decision:   DecisionLLMCall,
		DecisionID: "dec-001",
		LLMCall: &LLMCall{
			Model:    "deepseek",
			Messages: []LLMMessage{},
		},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for llm_call with empty messages")
	}
}

func TestDecisionValidate_EndMissingReason(t *testing.T) {
	d := Decision{
		Decision:   DecisionEnd,
		DecisionID: "dec-001",
		End:        &End{},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for end decision with empty reason")
	}
}

// — Additional validation tests for uncovered error paths —

func TestProcessRequestValidate_MissingRole(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "", Timestamp: "2026-01-01T00:00:00Z"},
		Identity:  Identity{Platform: "tg", ChatID: "1", UserName: "u", UserID: "1"},
		Context: Context{
			Config:       Config{MaxIterations: 1, TimeoutSeconds: 1},
			SessionState: SessionState{StartedAt: "2026-01-01T00:00:00Z"},
		},
	}
	if err := r.Validate(); err == nil {
		t.Error("expected error for missing message.role")
	}
}

func TestProcessRequestValidate_MissingPlatform(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "user", Timestamp: "2026-01-01T00:00:00Z"},
		Identity:  Identity{Platform: "", ChatID: "1", UserName: "u", UserID: "1"},
		Context: Context{
			Config:       Config{MaxIterations: 1, TimeoutSeconds: 1},
			SessionState: SessionState{StartedAt: "2026-01-01T00:00:00Z"},
		},
	}
	if err := r.Validate(); err == nil {
		t.Error("expected error for missing identity.platform")
	}
}

func TestProcessRequestValidate_MissingChatID(t *testing.T) {
	r := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "user", Timestamp: "2026-01-01T00:00:00Z"},
		Identity:  Identity{Platform: "tg", ChatID: "", UserName: "u", UserID: "1"},
		Context: Context{
			Config:       Config{MaxIterations: 1, TimeoutSeconds: 1},
			SessionState: SessionState{StartedAt: "2026-01-01T00:00:00Z"},
		},
	}
	if err := r.Validate(); err == nil {
		t.Error("expected error for missing identity.chat_id")
	}
}

func TestDecisionValidate_MissingDecisionID(t *testing.T) {
	d := Decision{
		Decision:   DecisionText,
		DecisionID: "",
		Text:       &TextResp{Content: "hello", Finished: true},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for missing decision_id")
	}
}

func TestDecisionValidate_ToolCallMissingName(t *testing.T) {
	d := Decision{
		Decision:   DecisionToolCall,
		DecisionID: "dec-001",
		ToolCall:   &ToolCall{Name: ""},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for tool_call with empty name")
	}
}

func TestDecisionValidate_LLMCallMissingModel(t *testing.T) {
	d := Decision{
		Decision:   DecisionLLMCall,
		DecisionID: "dec-001",
		LLMCall: &LLMCall{
			Model:    "",
			Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for llm_call with empty model")
	}
}

func TestDecisionValidate_LLMCallNil(t *testing.T) {
	d := Decision{
		Decision:   DecisionLLMCall,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for llm_call decision without llm_call payload")
	}
}

func TestDecisionValidate_TextMissingContent(t *testing.T) {
	d := Decision{
		Decision:   DecisionText,
		DecisionID: "dec-001",
		Text:       &TextResp{Content: ""},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for text decision with empty content")
	}
}

func TestDecisionValidate_TextNil(t *testing.T) {
	d := Decision{
		Decision:   DecisionText,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for text decision without text payload")
	}
}

func TestDecisionValidate_WaitMissingReason(t *testing.T) {
	d := Decision{
		Decision:   DecisionWait,
		DecisionID: "dec-001",
		Wait:       &Wait{Reason: ""},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for wait decision with empty reason")
	}
}

func TestDecisionValidate_WaitNil(t *testing.T) {
	d := Decision{
		Decision:   DecisionWait,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for wait decision without wait payload")
	}
}

func TestDecisionValidate_DelegateMissingTask(t *testing.T) {
	d := Decision{
		Decision:   DecisionDelegate,
		DecisionID: "dec-001",
		Delegate:   &Delegate{Task: ""},
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for delegate decision with empty task")
	}
}

func TestDecisionValidate_DelegateNil(t *testing.T) {
	d := Decision{
		Decision:   DecisionDelegate,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for delegate decision without delegate payload")
	}
}

func TestDecisionValidate_EndNil(t *testing.T) {
	d := Decision{
		Decision:   DecisionEnd,
		DecisionID: "dec-001",
	}
	if err := d.Validate(); err == nil {
		t.Error("expected error for end decision without end payload")
	}
}

func TestDecisionValidate_ValidToolCall(t *testing.T) {
	d := Decision{
		Decision:   DecisionToolCall,
		DecisionID: "dec-001",
		ToolCall:   &ToolCall{Name: "read_file"},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid tool_call decision, got: %v", err)
	}
}

func TestDecisionValidate_ValidLLMCall(t *testing.T) {
	d := Decision{
		Decision:   DecisionLLMCall,
		DecisionID: "dec-001",
		LLMCall: &LLMCall{
			Model:    "deepseek",
			Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid llm_call decision, got: %v", err)
	}
}

func TestDecisionValidate_ValidText(t *testing.T) {
	d := Decision{
		Decision:   DecisionText,
		DecisionID: "dec-001",
		Text:       &TextResp{Content: "hello", Finished: true},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid text decision, got: %v", err)
	}
}

func TestDecisionValidate_ValidWait(t *testing.T) {
	d := Decision{
		Decision:   DecisionWait,
		DecisionID: "dec-001",
		Wait:       &Wait{Reason: "waiting"},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid wait decision, got: %v", err)
	}
}

func TestDecisionValidate_ValidDelegate(t *testing.T) {
	d := Decision{
		Decision:   DecisionDelegate,
		DecisionID: "dec-001",
		Delegate:   &Delegate{Task: "review"},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid delegate decision, got: %v", err)
	}
}

func TestDecisionValidate_ValidEnd(t *testing.T) {
	d := Decision{
		Decision:   DecisionEnd,
		DecisionID: "dec-001",
		End:        &End{Reason: EndTaskComplete},
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid end decision, got: %v", err)
	}
}

// BenchmarkDecisionMarshal measures JSON marshal performance for a typical Decision.
func BenchmarkDecisionMarshal(b *testing.B) {
	d := Decision{
		Decision:   DecisionText,
		DecisionID: "dec-001",
		Text:       &TextResp{Content: "Hello, world!", Finished: true},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(d)
	}
}

// BenchmarkDecisionUnmarshal measures JSON unmarshal performance for a typical Decision.
func BenchmarkDecisionUnmarshal(b *testing.B) {
	data := []byte(`{"decision":"text","decision_id":"dec-001","text":{"content":"Hello, world!","finished":true}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var d Decision
		_ = json.Unmarshal(data, &d)
	}
}

// BenchmarkProcessRequestMarshal measures JSON marshal for a full ProcessRequest.
func BenchmarkProcessRequestMarshal(b *testing.B) {
	req := ProcessRequest{
		SessionID: "sess-001",
		Message:   Message{Role: "user", Content: "Hello, world!", Timestamp: "2026-07-19T12:00:00Z"},
		Identity:  Identity{Platform: "test", ChatID: "c1", UserName: "tester", UserID: "u1"},
		Context: Context{
			History:      []HistoryEntry{},
			Tools:        []Tool{{Name: "search", Description: "Search the web", Parameters: map[string]any{}}},
			Models:       []Model{{Name: "gpt-4", Provider: "openai"}},
			Config:       Config{MaxIterations: 10, TimeoutSeconds: 30},
			SessionState: SessionState{TurnCount: 0, TotalToolCalls: 0, TotalLLMCalls: 0, CostSoFar: 0, StartedAt: "2026-07-19T12:00:00Z"},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(req)
	}
}

// BenchmarkProcessRequestUnmarshal measures JSON unmarshal for a full ProcessRequest.
func BenchmarkProcessRequestUnmarshal(b *testing.B) {
	data := []byte(`{"session_id":"sess-001","message":{"role":"user","content":"Hello, world!","timestamp":"2026-07-19T12:00:00Z"},"identity":{"platform":"test","chat_id":"c1","user_name":"tester","user_id":"u1"},"context":{"history":[],"tools":[{"name":"search","description":"Search the web","parameters":{}}],"models":[{"name":"gpt-4","provider":"openai"}],"config":{"max_iterations":10,"timeout_seconds":30},"session_state":{"turn_count":0,"total_tool_calls":0,"total_llm_calls":0,"cost_so_far":0,"started_at":"2026-07-19T12:00:00Z"}}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req ProcessRequest
		_ = json.Unmarshal(data, &req)
	}
}
