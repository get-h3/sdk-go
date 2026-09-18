package testbed

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// echoHarness is a simple test harness that echoes back the user message.
type echoHarness struct {
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

// panicHarness panics in OnProcess to verify MockHermes recovers panics
// instead of crashing the test binary.
type panicHarness struct{}

func (h *panicHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	panic("boom in OnProcess")
}

func (h *panicHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	panic("boom in OnResult")
}

func (h *panicHarness) OnCancel(req *protocol.CancelRequest) error {
	panic("boom in OnCancel")
}

func (h *panicHarness) OnSessionTerminate(sessionID string) error {
	panic("boom in OnSessionTerminate")
}

func (h *panicHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
	}
}

func TestSendMessagePanicRecovery(t *testing.T) {
	mh := NewMockHermes(&panicHarness{})

	dec, err := mh.SendMessage("sess-001", "trigger panic", "tester", "user-1")
	if err == nil {
		t.Fatal("expected error from panicking harness, got nil")
	}
	if dec != nil {
		t.Errorf("expected nil decision on panic, got %+v", dec)
	}
	if mh.LastError == nil {
		t.Error("LastError should be set after panic recovery")
	}
}

func TestSendResultPanicRecovery(t *testing.T) {
	mh := NewMockHermes(&panicHarness{})

	dec, err := mh.SendResult("sess-001", "dec-001", protocol.Result{Type: protocol.ResultTool, Success: true})
	if err == nil {
		t.Fatal("expected error from panicking harness, got nil")
	}
	if dec != nil {
		t.Errorf("expected nil decision on panic, got %+v", dec)
	}
	if mh.LastError == nil {
		t.Error("LastError should be set after panic recovery")
	}
}

func TestSendCancelPanicRecovery(t *testing.T) {
	mh := NewMockHermes(&panicHarness{})

	err := mh.SendCancel("sess-001", protocol.CancelUserInterrupt)
	if err == nil {
		t.Fatal("expected error from panicking harness, got nil")
	}
	if mh.LastError == nil {
		t.Error("LastError should be set after panic recovery")
	}
}

func TestTerminateSessionPanicRecovery(t *testing.T) {
	mh := NewMockHermes(&panicHarness{})

	err := mh.TerminateSession("sess-001")
	if err == nil {
		t.Fatal("expected error from panicking harness, got nil")
	}
	if mh.LastError == nil {
		t.Error("LastError should be set after panic recovery")
	}
}

// historyHarness records the request it is handed (so a test can prove what
// reached the harness) and echoes the request history back in its Decision,
// mirroring the never-shrinking-history contract the h3-test battery enforces.
type historyHarness struct {
	processCalls int
	lastRequest  *protocol.ProcessRequest
}

func (h *historyHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.processCalls++
	h.lastRequest = req
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "hist-1",
		Text: &protocol.TextResp{
			Content:  "Hist: " + req.Message.Content,
			Finished: true,
		},
		History: req.Context.History,
	}, nil
}

func (h *historyHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	return &protocol.Decision{
		Decision:   protocol.DecisionEnd,
		DecisionID: "hist-end",
		End:        &protocol.End{Reason: protocol.EndTaskComplete, Summary: "done"},
	}, nil
}

func (h *historyHarness) OnCancel(req *protocol.CancelRequest) error { return nil }

func (h *historyHarness) OnSessionTerminate(sessionID string) error { return nil }

func (h *historyHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
	}
}

var _ harness.Harness = &historyHarness{}

// TestSendMessageWithHistory is the GAP-039 regression: conversation history
// seeded through the testbed API must reach the harness and must survive into
// the returned Decision — no raw h.OnProcess call, no hand-built ProcessRequest.
func TestSendMessageWithHistory(t *testing.T) {
	h := &historyHarness{}
	mh := NewMockHermes(h)

	history := []protocol.HistoryEntry{
		{Role: protocol.RoleUser, Content: "what is the capital of France?"},
		{Role: protocol.RoleAssistant, Content: "Paris."},
	}

	dec, err := mh.SendMessageWithHistory("sess-hist", "and of Italy?", history, "alice", "u-42")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionText)
	AssertTextContent(t, dec, "Hist: and of Italy?", true)

	// 1. The seeded history reached the harness, in order and verbatim.
	if h.lastRequest == nil {
		t.Fatal("harness never received a request")
	}
	if h.processCalls != 1 {
		t.Errorf("expected processCalls=1, got %d", h.processCalls)
	}
	got := h.lastRequest.Context.History
	if len(got) != len(history) {
		t.Fatalf("harness saw %d history entries, want %d: %+v", len(got), len(history), got)
	}
	for i, want := range history {
		if got[i] != want {
			t.Errorf("history[%d] = %+v, want %+v", i, got[i], want)
		}
	}

	// The rest of the request shape is unchanged: same user message and identity.
	if h.lastRequest.SessionID != "sess-hist" {
		t.Errorf("SessionID = %q, want %q", h.lastRequest.SessionID, "sess-hist")
	}
	if h.lastRequest.Message.Role != "user" || h.lastRequest.Message.Content != "and of Italy?" {
		t.Errorf("message = %+v, want role=user content=%q", h.lastRequest.Message, "and of Italy?")
	}
	if h.lastRequest.Identity.Platform != "test" ||
		h.lastRequest.Identity.UserName != "alice" ||
		h.lastRequest.Identity.UserID != "u-42" {
		t.Errorf("identity = %+v, want platform=test user=alice id=u-42", h.lastRequest.Identity)
	}

	// 2. The returned decision preserves the seeded history.
	if len(dec.History) != len(history) {
		t.Fatalf("decision history has %d entries, want %d: %+v", len(dec.History), len(history), dec.History)
	}
	for i, want := range history {
		if dec.History[i] != want {
			t.Errorf("decision history[%d] = %+v, want %+v", i, dec.History[i], want)
		}
	}

	// Tracking behavior is unchanged from SendMessage.
	if mh.SessionCount != 1 {
		t.Errorf("expected SessionCount=1, got %d", mh.SessionCount)
	}
	if len(mh.Decisions) != 1 || mh.Decisions[0] != dec {
		t.Errorf("expected 1 tracked decision identical to the return value, got %d", len(mh.Decisions))
	}
	if mh.LastDecision != dec {
		t.Error("LastDecision should match the returned decision")
	}
}

// TestSendMessageWithHistory_CompatibleAndCopied covers the compatibility half
// of GAP-039: an absent or empty history must leave every other caller's
// behavior untouched, and the request must not alias the caller's slice.
func TestSendMessageWithHistory_CompatibleAndCopied(t *testing.T) {
	// ContextWithHistory with no history is byte-for-byte DefaultContext().
	if got, want := ContextWithHistory(nil), DefaultContext(); !reflect.DeepEqual(got, want) {
		t.Errorf("ContextWithHistory(nil) = %+v, want DefaultContext() %+v", got, want)
	}
	if ctx := ContextWithHistory(nil); ctx.History == nil {
		t.Error("ContextWithHistory(nil).History is nil — want empty non-nil slice")
	}
	if ctx := ContextWithHistory([]protocol.HistoryEntry{}); ctx.History == nil {
		t.Error("ContextWithHistory(empty).History is nil — want empty non-nil slice")
	}

	// SendMessage and SendMessageWithHistory(nil) reach the harness identically.
	h1, h2 := &historyHarness{}, &historyHarness{}
	mh1, mh2 := NewMockHermes(h1), NewMockHermes(h2)
	dec1, err := mh1.SendMessage("sess-plain", "hello", "tester", "u-1")
	AssertNoError(t, err)
	dec2, err := mh2.SendMessageWithHistory("sess-plain", "hello", nil, "tester", "u-1")
	AssertNoError(t, err)
	if len(h1.lastRequest.Context.History) != 0 || len(h2.lastRequest.Context.History) != 0 {
		t.Errorf("expected empty history on both paths, got %d and %d",
			len(h1.lastRequest.Context.History), len(h2.lastRequest.Context.History))
	}
	if dec1.Text == nil || dec2.Text == nil || dec1.Text.Content != dec2.Text.Content {
		t.Errorf("SendMessage and SendMessageWithHistory(nil) diverged: %+v vs %+v", dec1.Text, dec2.Text)
	}

	// Mutating the caller's slice after the call must not rewrite history the
	// harness already saw.
	h3 := &historyHarness{}
	mh3 := NewMockHermes(h3)
	history := []protocol.HistoryEntry{
		{Role: protocol.RoleUser, Content: "first"},
		{Role: protocol.RoleAssistant, Content: "second"},
	}
	_, err = mh3.SendMessageWithHistory("sess-copy", "third", history, "tester", "u-1")
	AssertNoError(t, err)
	history[0].Content = "mutated-after-the-call"
	if got := h3.lastRequest.Context.History[0].Content; got != "first" {
		t.Errorf("request aliases the caller's slice: history[0] = %q, want %q", got, "first")
	}
}

// Verify MockHermes implements the harness.Harness interface via its wrapped harness.
var _ harness.Harness = &echoHarness{}
