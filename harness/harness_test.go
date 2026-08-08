package harness

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/get-h3/sdk-go/protocol"
)

// mockHarness implements Harness with configurable return values for testing.
type mockHarness struct {
	healthResp      *protocol.HealthResponse
	onProcessDec    *protocol.Decision
	onProcessErr    error
	onResultDec     *protocol.Decision
	onResultErr     error
	onCancelErr     error
	onTerminateErr  error
	cancelCalled    bool
	terminateCalled string
	lastProcessReq  *protocol.ProcessRequest
	lastResultReq   *protocol.ResultRequest
	lastCancelReq   *protocol.CancelRequest
	panicOnProcess  bool
}

func (m *mockHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	if m.panicOnProcess {
		panic("test panic in OnProcess")
	}
	m.lastProcessReq = req
	return m.onProcessDec, m.onProcessErr
}

func (m *mockHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	m.lastResultReq = req
	return m.onResultDec, m.onResultErr
}

func (m *mockHarness) OnCancel(req *protocol.CancelRequest) error {
	m.cancelCalled = true
	m.lastCancelReq = req
	return m.onCancelErr
}

func (m *mockHarness) OnSessionTerminate(sessionID string) error {
	m.terminateCalled = sessionID
	return m.onTerminateErr
}

func (m *mockHarness) Health() *protocol.HealthResponse {
	return m.healthResp
}

func newMockHarness() *mockHarness {
	return &mockHarness{
		healthResp: &protocol.HealthResponse{
			Status:          protocol.HealthOK,
			Version:         "1.0.0",
			Transport:       "rest",
			ProtocolVersion: "1.0",
		},
		onProcessDec: &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: "dec-test-001",
			Text: &protocol.TextResp{
				Content:  "Hello from test harness",
				Finished: true,
			},
		},
		onResultDec: &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "dec-test-002",
			End: &protocol.End{
				Reason:  protocol.EndTaskComplete,
				Summary: "done",
			},
		},
	}
}

func TestHealthEndpoint(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var hr protocol.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if hr.Status != protocol.HealthOK {
		t.Errorf("expected status ok, got %q", hr.Status)
	}
	if hr.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", hr.Version)
	}
}

func TestHealthEndpoint_NilResponse(t *testing.T) {
	m := &mockHarness{healthResp: nil}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var hr protocol.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	// Should return default ok response
	if hr.Status != protocol.HealthOK {
		t.Errorf("expected default status ok, got %q", hr.Status)
	}
}

func TestProcessEndpoint_Valid(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "sess-001",
		"message": {
			"role": "user",
			"content": "hello",
			"timestamp": "2026-07-14T00:00:00Z"
		},
		"identity": {
			"platform": "test",
			"chat_id": "chat-1",
			"user_name": "tester",
			"user_id": "user-1"
		},
		"context": {
			"history": [],
			"tools": [],
			"models": [],
			"config": {
				"max_iterations": 10,
				"timeout_seconds": 30
			},
			"session_state": {
				"turn_count": 0,
				"total_tool_calls": 0,
				"total_llm_calls": 0,
				"cost_so_far": 0,
				"started_at": "2026-07-14T00:00:00Z"
			}
		}
	}`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	if dec.Decision != protocol.DecisionText {
		t.Errorf("expected decision text, got %q", dec.Decision)
	}
	if m.lastProcessReq == nil {
		t.Fatal("OnProcess was not called")
	}
	if m.lastProcessReq.SessionID != "sess-001" {
		t.Errorf("expected session sess-001, got %q", m.lastProcessReq.SessionID)
	}
}

func TestProcessEndpoint_InvalidBody(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Missing required fields (session_id, identity, context)
	body := `{"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"}}`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
	}
}

func TestProcessEndpoint_AutoGenerateDecisionID(t *testing.T) {
	// When harness returns a Decision with empty decision_id, the handler
	// auto-generates a UUID v4 before validation (H3 protocol §2.1).
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "", // harness didn't set — handler must auto-generate
		Text: &protocol.TextResp{
			Content:  "hello",
			Finished: true,
		},
	}

	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "sess-auto",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}

	// Verify the handler auto-generated a non-empty UUID v4.
	if dec.DecisionID == "" {
		t.Error("expected auto-generated decision_id, got empty string")
	}
	// UUID v4 has exactly 36 characters: 8-4-4-4-12 hex.
	if len(dec.DecisionID) != 36 {
		t.Errorf("expected UUID v4 (36 chars), got %d: %q", len(dec.DecisionID), dec.DecisionID)
	}
	// Position 14 in a UUID v4 string is always '4'.
	if dec.DecisionID[14] != '4' {
		t.Errorf("expected UUID v4 (version bit at [14]), got: %q", dec.DecisionID)
	}
	if dec.Decision != protocol.DecisionText {
		t.Errorf("expected decision text, got %q", dec.Decision)
	}
}

func TestProcessEndpoint_OnProcessError(t *testing.T) {
	m := newMockHarness()
	m.onProcessErr = errors.New("harness internal error")

	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "sess-003",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestProcessEndpoint_MalformedJSON(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{not json at all`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestResultEndpoint(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)

	// First create a session via process
	ts := httptest.NewServer(srv)
	defer ts.Close()

	processBody := `{
		"session_id": "sess-r1",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`
	_, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(processBody))
	if err != nil {
		t.Fatalf("POST /v1/process (setup): %v", err)
	}

	resultBody := `{
		"session_id": "sess-r1",
		"decision_id": "dec-001",
		"result": {"type": "tool_result", "tool_name": "test", "success": true}
	}`

	resp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(resultBody))
	if err != nil {
		t.Fatalf("POST /v1/result: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	if dec.Decision != protocol.DecisionEnd {
		t.Errorf("expected decision end, got %q", dec.Decision)
	}
	if m.lastResultReq == nil {
		t.Fatal("OnResult was not called")
	}
}

func TestCancelEndpoint(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// GAP-DOG-002: cancel must 404 on unknown sessions — create the session
	// via POST /v1/process first so the cancel below targets a real session.
	processBody := `{
		"session_id": "sess-c1",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`
	if resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(processBody)); err != nil {
		t.Fatalf("POST /v1/process (setup): %v", err)
	} else {
		_ = resp.Body.Close()
	}

	body := `{"session_id": "sess-c1", "reason": "user_interrupt"}`

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !m.cancelCalled {
		t.Error("OnCancel was not called")
	}

	// GAP-003: response body must match the OpenAPI contract —
	// {"cancelled": true, "cancelled_decision_id": "..."}.
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read cancel response body: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if _, ok := fields["cancelled_decision_id"]; !ok {
		t.Error("expected cancelled_decision_id key in cancel response body")
	}
	var cr protocol.CancelResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("decode cancel response into CancelResponse: %v", err)
	}
	if !cr.Cancelled {
		t.Errorf("expected cancelled=true, got %v", cr.Cancelled)
	}
}

func TestCancelUnknownSession(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{"session_id": "never-created", "reason": "user_interrupt"}`
	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if m.cancelCalled {
		t.Error("OnCancel must NOT be called for an unknown session")
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %q", errResp.Error.Code)
	}
}

func TestResultUnknownSession(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "never-created",
		"decision_id": "dec-x",
		"result": {"type": "tool_result", "tool_name": "test", "success": true}
	}`
	resp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/result: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if m.lastResultReq != nil {
		t.Error("OnResult must NOT be called for an unknown session")
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %q", errResp.Error.Code)
	}
}

func TestGetSessionEndpoint(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a session first
	processBody := `{
		"session_id": "sess-g1",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`
	_, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(processBody))
	if err != nil {
		t.Fatalf("POST /v1/process (setup): %v", err)
	}

	resp, err := http.Get(ts.URL + "/v1/sessions/sess-g1")
	if err != nil {
		t.Fatalf("GET /v1/sessions/sess-g1: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sr protocol.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if sr.SessionID != "sess-g1" {
		t.Errorf("expected sess-g1, got %q", sr.SessionID)
	}
	if sr.Status != protocol.SessionActive {
		t.Errorf("expected active, got %q", sr.Status)
	}
}

func TestGetSessionEndpoint_NotFound(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/sessions/nonexistent")
	if err != nil {
		t.Fatalf("GET /v1/sessions/nonexistent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteSessionEndpoint(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a session
	processBody := `{
		"session_id": "sess-d1",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`
	_, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(processBody))
	if err != nil {
		t.Fatalf("POST /v1/process (setup): %v", err)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/v1/sessions/sess-d1", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/sessions/sess-d1: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if m.terminateCalled != "sess-d1" {
		t.Errorf("expected OnSessionTerminate called with sess-d1, got %q", m.terminateCalled)
	}

	// GAP-003: response body must match the OpenAPI contract —
	// {"terminated": true, "session_id": "sess-d1"} with HTTP 200.
	var tr protocol.SessionTerminateResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("decode session terminate response: %v", err)
	}
	if !tr.Terminated {
		t.Errorf("expected terminated=true, got %v", tr.Terminated)
	}
	if tr.SessionID != "sess-d1" {
		t.Errorf("expected session_id sess-d1, got %q", tr.SessionID)
	}
}

func TestDeleteSessionNotFound(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/v1/sessions/does-not-exist", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/sessions/does-not-exist: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if m.terminateCalled != "" {
		t.Errorf("expected OnSessionTerminate NOT called, got %q", m.terminateCalled)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %q", errResp.Error.Code)
	}
}

func TestPanicRecovery(t *testing.T) {
	m := newMockHarness()
	m.panicOnProcess = true

	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "sess-p1",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`

	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// GET on process endpoint should get 405
	resp, err := http.Get(ts.URL + "/v1/process")
	if err != nil {
		t.Fatalf("GET /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// TestHarnessTimeout verifies that a handler exceeding the deadline returns
// HTTP 504 with a JSON ErrorResponse containing code HARNESS_TIMEOUT.
func TestHarnessTimeout(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"slow":"should be discarded"}`))
	})
	// 100ms deadline vs 500ms sleep — generous deterministic margin.
	handler := withMiddlewareTimeout(slow, 100*time.Millisecond)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/process", "application/json",
		strings.NewReader(`{"session_id":"sess-to","message":{"role":"user","content":"hi","timestamp":"2026-01-01T00:00:00Z"},"identity":{"platform":"test","chat_id":"c","user_name":"u","user_id":"u"},"context":{"history":[],"tools":[],"models":[],"config":{"max_iterations":10,"timeout_seconds":30},"session_state":{"turn_count":0,"total_tool_calls":0,"total_llm_calls":0,"cost_so_far":0,"started_at":"2026-01-01T00:00:00Z"}}}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode ErrorResponse: %v", err)
	}
	if errResp.Error.Code != protocol.ErrHarnessTimeout {
		t.Errorf("expected code %q, got %q", protocol.ErrHarnessTimeout, errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestHarnessTimeout_NoTimeout verifies that a handler finishing before the
// deadline returns its normal response through the timeout wrapper.
func TestHarnessTimeout_NoTimeout(t *testing.T) {
	fast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	handler := withMiddlewareTimeout(fast, 1*time.Second)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes_contains(body, `{"ok":true}`) {
		t.Errorf("unexpected body: %s", body)
	}
}

// bytes_contains is a tiny helper to avoid importing bytes in the test file.
func bytes_contains(b []byte, s string) bool {
	return strings.Contains(string(b), s)
}

// processBody is a minimal valid POST /v1/process request body for session id sid.
func processBody(sid string) string {
	return `{
		"session_id": "` + sid + `",
		"message": {"role": "user", "content": "hello", "timestamp": "2026-07-14T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "t", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-07-14T00:00:00Z"}
		}
	}`
}

// postProcess posts a /v1/process request and returns the decoded Decision.
func postProcess(t *testing.T, ts *httptest.Server, sid string) protocol.Decision {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(processBody(sid)))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var dec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	return dec
}

// getSession GETs /v1/sessions/{sid} and returns the decoded SessionResponse.
func getSession(t *testing.T, ts *httptest.Server, sid string) protocol.SessionResponse {
	t.Helper()
	resp, err := http.Get(ts.URL + "/v1/sessions/" + sid)
	if err != nil {
		t.Fatalf("GET /v1/sessions/%s: %v", sid, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var sr protocol.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	return sr
}

// TestSessionObservability_ProcessShowsCurrentDecision (AC1):
// POST /v1/process with a tool_call decision → GET /v1/sessions/{id}
// shows current_decision and current_decision_type="tool_call".
func TestSessionObservability_ProcessShowsCurrentDecision(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionToolCall,
		DecisionID: "dec-tool-001",
		ToolCall: &protocol.ToolCall{
			Name:   "search",
			Params: map[string]any{"query": "hello"},
		},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	dec := postProcess(t, ts, "sess-ac1")
	if dec.DecisionID != "dec-tool-001" {
		t.Fatalf("expected decision_id dec-tool-001, got %q", dec.DecisionID)
	}

	sr := getSession(t, ts, "sess-ac1")
	if sr.CurrentDecision != "dec-tool-001" {
		t.Errorf("expected current_decision dec-tool-001, got %q", sr.CurrentDecision)
	}
	if sr.CurrentDecisionType != protocol.DecisionToolCall {
		t.Errorf("expected current_decision_type tool_call, got %q", sr.CurrentDecisionType)
	}
}

// TestSessionObservability_ResultUpdatesCurrentDecision (AC2):
// POST /v1/process (text) → POST /v1/result (text decision) →
// GET session shows current_decision = result decision's id and
// current_decision_type = "text".
func TestSessionObservability_ResultUpdatesCurrentDecision(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-proc-002",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-res-002",
		Text:       &protocol.TextResp{Content: "final answer", Finished: true},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-ac2")

	resultBody := `{
		"session_id": "sess-ac2",
		"decision_id": "dec-proc-002",
		"result": {"type": "tool_result", "tool_name": "test", "success": true}
	}`
	resp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(resultBody))
	if err != nil {
		t.Fatalf("POST /v1/result: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var resDec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&resDec); err != nil {
		t.Fatalf("decode result decision: %v", err)
	}
	if resDec.DecisionID != "dec-res-002" {
		t.Fatalf("expected result decision_id dec-res-002, got %q", resDec.DecisionID)
	}

	sr := getSession(t, ts, "sess-ac2")
	if sr.CurrentDecision != "dec-res-002" {
		t.Errorf("expected current_decision dec-res-002, got %q", sr.CurrentDecision)
	}
	if sr.CurrentDecisionType != protocol.DecisionText {
		t.Errorf("expected current_decision_type text, got %q", sr.CurrentDecisionType)
	}
}

// TestSessionObservability_CancelReturnsDecisionID (AC3):
// POST /v1/process → POST /v1/cancel → cancelled_decision_id is the
// process decision's id (NOT "").
func TestSessionObservability_CancelReturnsDecisionID(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionToolCall,
		DecisionID: "dec-cancel-003",
		ToolCall:   &protocol.ToolCall{Name: "search", Params: map[string]any{}},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-ac3")

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "sess-ac3", "reason": "user_interrupt"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read cancel response: %v", err)
	}
	var cr protocol.CancelResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if !cr.Cancelled {
		t.Error("expected cancelled=true")
	}
	if cr.CancelledDecisionID != "dec-cancel-003" {
		t.Errorf("expected cancelled_decision_id dec-cancel-003, got %q", cr.CancelledDecisionID)
	}
	// Verify the raw JSON contains the key with the expected value.
	if !strings.Contains(string(raw), `"cancelled_decision_id":"dec-cancel-003"`) {
		t.Errorf("raw body does not contain expected cancelled_decision_id: %s", raw)
	}
}

// blockingHarness blocks OnProcess until the release channel is closed,
// simulating a long-running decision that has not yet finalized.
type blockingHarness struct {
	mockHarness
	processStarted chan struct{}
	release        chan struct{}
}

func (b *blockingHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	b.processStarted <- struct{}{}
	<-b.release
	return b.onProcessDec, b.onProcessErr
}

// TestSessionObservability_CancelNoDecisionInFlight (AC4):
// POST /v1/process on a NEW session → POST /v1/cancel BEFORE the decision
// is finalized (harness blocked) → cancelled_decision_id is present in JSON
// but "" (nothing was in flight).
func TestSessionObservability_CancelNoDecisionInFlight(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-late-004",
		Text:       &protocol.TextResp{Content: "late", Finished: true},
	}
	bh := &blockingHarness{
		mockHarness:    *m,
		processStarted: make(chan struct{}),
		release:        make(chan struct{}),
	}
	srv := NewHTTPServer(bh)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Issue POST /v1/process in a goroutine — OnProcess will block.
	go func() {
		resp, err := http.Post(ts.URL+"/v1/process", "application/json",
			strings.NewReader(processBody("sess-ac4")))
		if err != nil {
			t.Errorf("POST /v1/process: %v", err)
			return
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}()

	// Wait until OnProcess has started (session created, decision NOT finalized).
	<-bh.processStarted

	// Now cancel — session exists but no decision was ever stored.
	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "sess-ac4", "reason": "user_interrupt"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read cancel response: %v", err)
	}
	var cr protocol.CancelResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if !cr.Cancelled {
		t.Error("expected cancelled=true")
	}
	if cr.CancelledDecisionID != "" {
		t.Errorf("expected empty cancelled_decision_id (nothing in flight), got %q", cr.CancelledDecisionID)
	}
	// Verify the key is present in raw JSON even though the value is empty.
	if !strings.Contains(string(raw), `"cancelled_decision_id"`) {
		t.Errorf("raw body missing cancelled_decision_id key: %s", raw)
	}

	// Release the blocked OnProcess so the goroutine can finish and not leak.
	close(bh.release)
}

// BenchmarkHandlerProcess measures end-to-end handler latency for a POST /v1/process request.
func BenchmarkHandlerProcess(b *testing.B) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{"session_id":"sess-001","message":{"role":"user","content":"Hello","timestamp":"2026-07-19T12:00:00Z"},"identity":{"provider":"test","chat_id":"c1","user_name":"tester","user_id":"u1"},"context":{"history":[],"tools":[],"models":[],"config":{"max_iterations":10,"timeout_seconds":30},"session_state":{"turn_count":0,"total_tool_calls":0,"total_llm_calls":0,"cost_so_far":0,"started_at":"2026-07-19T12:00:00Z"}}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(ts.URL+"/v1/process", "application/json", strings.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
}
