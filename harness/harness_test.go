package harness

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

func TestProcessEndpoint_InvalidDecision(t *testing.T) {
	m := newMockHarness()
	// Return a decision that fails validation (missing decision_id)
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "", // invalid — must be set
		Text: &protocol.TextResp{
			Content:  "bad",
			Finished: true,
		},
	}

	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := `{
		"session_id": "sess-002",
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

	body := `{"session_id": "sess-c1", "reason": "user_interrupt"}`

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !m.cancelCalled {
		t.Error("OnCancel was not called")
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if m.terminateCalled != "sess-d1" {
		t.Errorf("expected OnSessionTerminate called with sess-d1, got %q", m.terminateCalled)
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}
