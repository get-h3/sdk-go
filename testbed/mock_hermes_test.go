package testbed

import (
	"fmt"
	"testing"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// echoHarness is a simple test harness that echoes back the user message.
type echoHarness struct {
	onProcessResponses []*protocol.Decision
	processCallCount   int
	resultCallCount    int
	cancelCallCount    int
	terminateCallCount int
}

func (h *echoHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.processCallCount++
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "echo-1",
		Text: &protocol.TextResp{
			Content:  "Echo: " + req.Message.Content,
			Finished: true,
		},
	}, nil
}

func (h *echoHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.resultCallCount++
	return &protocol.Decision{
		Decision:   protocol.DecisionEnd,
		DecisionID: "echo-end",
		End: &protocol.End{
			Reason:  protocol.EndTaskComplete,
			Summary: "done",
		},
	}, nil
}

func (h *echoHarness) OnCancel(req *protocol.CancelRequest) error {
	h.cancelCallCount++
	return nil
}

func (h *echoHarness) OnSessionTerminate(sessionID string) error {
	h.terminateCallCount++
	return nil
}

func (h *echoHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
	}
}

func TestSendMessage(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	dec, err := mh.SendMessage("sess-001", "hello world", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionText)
	AssertTextContent(t, dec, "Echo: hello world", true)
	AssertDecisionValid(t, dec)

	if mh.SessionCount != 1 {
		t.Errorf("expected SessionCount=1, got %d", mh.SessionCount)
	}
	if h.processCallCount != 1 {
		t.Errorf("expected processCallCount=1, got %d", h.processCallCount)
	}
	if len(mh.Decisions) != 1 {
		t.Errorf("expected 1 decision tracked, got %d", len(mh.Decisions))
	}
}

func TestSendResult(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	result := protocol.Result{
		Type:    protocol.ResultTool,
		Success: true,
	}

	dec, err := mh.SendResult("sess-001", "dec-001", result)
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionEnd)
	AssertEndReason(t, dec, protocol.EndTaskComplete)

	if h.resultCallCount != 1 {
		t.Errorf("expected resultCallCount=1, got %d", h.resultCallCount)
	}
}

func TestSendCancel(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	err := mh.SendCancel("sess-001", protocol.CancelUserInterrupt)
	AssertNoError(t, err)

	if h.cancelCallCount != 1 {
		t.Errorf("expected cancelCallCount=1, got %d", h.cancelCallCount)
	}
}

func TestTerminateSession(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	err := mh.TerminateSession("sess-001")
	AssertNoError(t, err)

	if h.terminateCallCount != 1 {
		t.Errorf("expected terminateCallCount=1, got %d", h.terminateCallCount)
	}
}

func TestWithEchoHarness(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	// Step 1: Send a message → expect echo
	dec, err := mh.SendMessage("sess-001", "hello", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionText)
	AssertTextContent(t, dec, "Echo: hello", true)

	// Step 2: Send result → expect end
	result := protocol.Result{
		Type:    protocol.ResultTextSent,
		Success: true,
	}
	dec2, err := mh.SendResult("sess-001", dec.DecisionID, result)
	AssertNoError(t, err)
	AssertDecisionType(t, dec2, protocol.DecisionEnd)
	AssertEndReason(t, dec2, protocol.EndTaskComplete)

	// Verify tracking
	if mh.SessionCount != 1 {
		t.Errorf("expected SessionCount=1, got %d", mh.SessionCount)
	}
	if len(mh.Decisions) != 2 {
		t.Errorf("expected 2 decisions tracked, got %d", len(mh.Decisions))
	}
}

func TestHealth(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	resp := mh.Health()
	if resp == nil {
		t.Fatal("expected health response, got nil")
	}
	if resp.Status != protocol.HealthOK {
		t.Errorf("expected status ok, got %q", resp.Status)
	}
}

func TestDefaultTools(t *testing.T) {
	tools := DefaultTools()
	if len(tools) == 0 {
		t.Error("DefaultTools returned empty slice")
	}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
	}
}

func TestDefaultModels(t *testing.T) {
	models := DefaultModels()
	if len(models) == 0 {
		t.Error("DefaultModels returned empty slice")
	}
	for _, model := range models {
		if model.Name == "" {
			t.Error("model has empty name")
		}
		if model.ContextWindow <= 0 {
			t.Errorf("model %q has invalid context window: %d", model.Name, model.ContextWindow)
		}
	}
}

func TestDefaultContext(t *testing.T) {
	ctx := DefaultContext()
	if ctx.Config.MaxIterations != 10 {
		t.Errorf("expected MaxIterations=10, got %d", ctx.Config.MaxIterations)
	}
	if ctx.Config.TimeoutSeconds != 30 {
		t.Errorf("expected TimeoutSeconds=30, got %d", ctx.Config.TimeoutSeconds)
	}
	if len(ctx.Tools) == 0 {
		t.Error("DefaultContext has no tools")
	}
	if len(ctx.Models) == 0 {
		t.Error("DefaultContext has no models")
	}
	if ctx.SessionState.TurnCount != 0 {
		t.Errorf("expected TurnCount=0, got %d", ctx.SessionState.TurnCount)
	}
}

func TestQuickIdentity(t *testing.T) {
	id := QuickIdentity("test-user", "test-id")
	if id.UserName != "test-user" {
		t.Errorf("expected UserName 'test-user', got %q", id.UserName)
	}
	if id.UserID != "test-id" {
		t.Errorf("expected UserID 'test-id', got %q", id.UserID)
	}
	if id.Platform != "test" {
		t.Errorf("expected Platform 'test', got %q", id.Platform)
	}
}

func TestQuickMessage(t *testing.T) {
	msg := QuickMessage("test content")
	if msg.Content != "test content" {
		t.Errorf("expected Content 'test content', got %q", msg.Content)
	}
	if msg.Role != "user" {
		t.Errorf("expected Role 'user', got %q", msg.Role)
	}
	if msg.Timestamp == "" {
		t.Error("expected Timestamp to be set")
	}
}

func TestMockHermes_LastDecisionAndError(t *testing.T) {
	h := &echoHarness{}
	mh := NewMockHermes(h)

	// Before any calls, fields should be nil
	if mh.LastDecision != nil {
		t.Error("LastDecision should be nil before any calls")
	}
	if mh.LastError != nil {
		t.Error("LastError should be nil before any calls")
	}

	dec, _ := mh.SendMessage("sess-001", "test", "tester", "user-1")
	if mh.LastDecision != dec {
		t.Error("LastDecision should match the returned decision")
	}
	if mh.LastError != nil {
		t.Error("LastError should be nil after successful call")
	}
}

// errorHarness returns errors for testing error tracking.
type errorHarness struct {
	processErr error
}

func (h *errorHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	return nil, h.processErr
}

func (h *errorHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	return nil, nil
}

func (h *errorHarness) OnCancel(req *protocol.CancelRequest) error {
	return nil
}

func (h *errorHarness) OnSessionTerminate(sessionID string) error {
	return nil
}

func (h *errorHarness) Health() *protocol.HealthResponse {
	return nil
}

func TestMockHermes_LastError(t *testing.T) {
	h := &errorHarness{
		processErr: fmt.Errorf("test error"),
	}
	mh := NewMockHermes(h)

	_, err := mh.SendMessage("sess-001", "test", "tester", "user-1")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if mh.LastError == nil {
		t.Error("LastError should be set after error")
	}
}

// Verify MockHermes implements the harness.Harness interface via its wrapped harness.
var _ harness.Harness = &echoHarness{}
