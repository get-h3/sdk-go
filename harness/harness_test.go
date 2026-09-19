package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	// resultCalls counts OnResult invocations. GAP-049's regression gate: a
	// replayed (or invented) decision_id must be rejected by the HANDLER, so
	// this counter must not move for a delivery the handler refuses.
	resultCalls int
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
	m.resultCalls++
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

	// GAP-049: decision_id must be the session's in-flight decision — the id
	// POST /v1/process just returned (dec-test-001, the mock's OnProcess id).
	// A result naming anything else is now rejected 400 INVALID_REQUEST.
	resultBody := `{
		"session_id": "sess-r1",
		"decision_id": "dec-test-001",
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

// TestCancelEndpoint_MissingSessionID verifies GAP-048: a well-formed JSON body
// with no session_id is a MALFORMED cancel — 400 INVALID_REQUEST — and must not
// be reported as 404 SESSION_NOT_FOUND (which named an empty session and read to
// a consumer as a session that had vanished).
func TestCancelEndpoint_MissingSessionID(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"reason": "system"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error message")
	}
	if m.cancelCalled {
		t.Error("OnCancel must NOT be called for a malformed cancel")
	}
}

// TestCancelEndpoint_UnknownReasonOnLiveSession verifies GAP-048 on a live
// session: a reason outside the enum is rejected with 400 INVALID_REQUEST
// instead of being accepted and mutating the session. The reject must happen
// before the store lookup, so the session is left untouched (still active).
func TestCancelEndpoint_UnknownReasonOnLiveSession(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-gap048-reason")

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "sess-gap048-reason", "reason": "nonsense_reason"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
	}
	if m.cancelCalled {
		t.Error("OnCancel must NOT be called when the cancel request is invalid")
	}

	// The session must be untouched by a rejected cancel.
	if sr := getSession(t, ts, "sess-gap048-reason"); sr.Status == protocol.SessionCancelled {
		t.Errorf("rejected cancel must not cancel the session, got status %q", sr.Status)
	}
}

// TestCancelEndpoint_ValidCancelStillOK pins GAP-048's hard constraint: the wire
// contract for a VALID cancel is unchanged — 200 {"cancelled": true, ...} and
// OnCancel is called with the decoded request.
func TestCancelEndpoint_ValidCancelStillOK(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-gap048-ok")

	resp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "sess-gap048-ok", "reason": "user_interrupt"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cr protocol.CancelResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if !cr.Cancelled {
		t.Errorf("expected cancelled=true, got %v", cr.Cancelled)
	}
	if !m.cancelCalled {
		t.Error("OnCancel was not called for a valid cancel")
	}
	if m.lastCancelReq == nil || m.lastCancelReq.SessionID != "sess-gap048-ok" {
		t.Errorf("OnCancel received unexpected request: %+v", m.lastCancelReq)
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

// GAP-049 -------------------------------------------------------------------
//
// POST /v1/result used to accept ANY decision_id: an invented id still drove
// the loop, and re-delivering the SAME id called OnResult a second time —
// re-running the harness's tool step (measured live: one decision_id delivered
// twice produced two identical tool_calls). Under at-least-once delivery (a
// client retry, two clients on one chat id) that double-executes side effects.
// The handler now correlates decision_id with the session's in-flight decision
// and refuses a duplicate/stale/invented id BEFORE OnResult, so the harness
// callback counter is the regression gate.

// postResult posts a /v1/result for (sid, decisionID) with the given
// result.type and returns the HTTP status plus the raw response body.
func postResult(t *testing.T, ts *httptest.Server, sid, decisionID, resultType string) (int, []byte) {
	t.Helper()
	body := `{"session_id": "` + sid + `", "decision_id": "` + decisionID +
		`", "result": {"type": "` + resultType + `", "tool_name": "test", "success": true}}`
	resp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/result: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST /v1/result body: %v", err)
	}
	return resp.StatusCode, raw
}

// TestResultEndpoint_DuplicateDecisionIDRejected (GAP-049): delivering the SAME
// decision_id twice must be refused with 400 INVALID_REQUEST ("already been
// resolved") and — the point of the whole fix — the harness-side OnResult must
// NOT run a second time. Pre-fix the second delivery returned 200 and invoked
// OnResult again, re-executing the result's side effects.
func TestResultEndpoint_DuplicateDecisionIDRejected(t *testing.T) {
	m := newMockHarness()
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-dup-002",
		Text:       &protocol.TextResp{Content: "applied once", Finished: true},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const sid = "sess-gap049-dup"
	d1 := postProcess(t, ts, sid).DecisionID
	if d1 == "" {
		t.Fatal("POST /v1/process returned an empty decision_id; cannot correlate")
	}

	// 1. First delivery of d1 — accepted, harness runs once.
	status, raw := postResult(t, ts, sid, d1, "tool_result")
	if status != http.StatusOK {
		t.Fatalf("first delivery: expected 200, got %d (%s)", status, raw)
	}
	var dec protocol.Decision
	if err := json.Unmarshal(raw, &dec); err != nil {
		t.Fatalf("decode first result decision: %v", err)
	}
	if dec.DecisionID != "dec-dup-002" {
		t.Fatalf("expected next decision dec-dup-002, got %q", dec.DecisionID)
	}
	callsAfterFirst := m.resultCalls
	if callsAfterFirst != 1 {
		t.Fatalf("expected OnResult to have run exactly once, got %d", callsAfterFirst)
	}

	// 2. Replay of d1 — refused, and OnResult must not run again.
	status, raw = postResult(t, ts, sid, d1, "tool_result")
	if status != http.StatusBadRequest {
		t.Fatalf("replayed delivery: expected 400, got %d (%s)", status, raw)
	}
	var errResp protocol.ErrorResponse
	if err := json.Unmarshal(raw, &errResp); err != nil {
		t.Fatalf("decode replay error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "already been resolved") {
		t.Errorf("expected 'already been resolved' in message, got %q", errResp.Error.Message)
	}
	if m.resultCalls != callsAfterFirst {
		t.Errorf("OnResult ran again for a replayed decision_id (resultCalls %d -> %d): the duplicate re-executed harness side effects",
			callsAfterFirst, m.resultCalls)
	}

	// 3. The refused delivery left the session exactly where the accepted one
	// put it: one turn for process + one for the accepted result, and the
	// in-flight decision is still the decision that result returned.
	sr := getSession(t, ts, sid)
	if sr.TurnCount != 2 {
		t.Errorf("expected turn_count 2 (process + accepted result), got %d — a refused result must not count as a turn", sr.TurnCount)
	}
	if sr.CurrentDecision != "dec-dup-002" {
		t.Errorf("expected current_decision dec-dup-002, got %q", sr.CurrentDecision)
	}
}

// TestResultEndpoint_InventedDecisionIDRejected (GAP-049): a decision_id the
// session never issued must be refused with 400 INVALID_REQUEST and must not
// reach OnResult or touch session state. Pre-fix an invented id returned 200
// and produced a decision, i.e. it drove the loop.
func TestResultEndpoint_InventedDecisionIDRejected(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const sid = "sess-gap049-bogus"
	d1 := postProcess(t, ts, sid).DecisionID
	if d1 == "" {
		t.Fatal("POST /v1/process returned an empty decision_id; cannot correlate")
	}

	status, raw := postResult(t, ts, sid, "bogus-decision-id", "tool_result")
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invented decision_id, got %d (%s)", status, raw)
	}
	var errResp protocol.ErrorResponse
	if err := json.Unmarshal(raw, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "does not match the session's in-flight decision") {
		t.Errorf("expected mismatch message, got %q", errResp.Error.Message)
	}
	if !strings.Contains(errResp.Error.Message, d1) {
		t.Errorf("expected the in-flight decision id %q to be named in the message, got %q", d1, errResp.Error.Message)
	}
	if m.resultCalls != 0 || m.lastResultReq != nil {
		t.Errorf("OnResult must NOT be called for an invented decision_id (calls=%d, lastReq=%v)", m.resultCalls, m.lastResultReq)
	}

	// Session state untouched: still the single process turn, still d1 in flight.
	sr := getSession(t, ts, sid)
	if sr.TurnCount != 1 {
		t.Errorf("expected turn_count 1 (process only), got %d", sr.TurnCount)
	}
	if sr.CurrentDecision != d1 {
		t.Errorf("expected current_decision %q, got %q", d1, sr.CurrentDecision)
	}
}

// TestResultEndpoint_HappyPathCorrelation (GAP-049): the normal path is
// unchanged — a result naming the session's in-flight decision is accepted with
// 200 and the next decision, and GET /v1/sessions/{id} then reports the NEW
// decision as current.
func TestResultEndpoint_HappyPathCorrelation(t *testing.T) {
	m := newMockHarness()
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-happy-002",
		Text:       &protocol.TextResp{Content: "next step", Finished: true},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const sid = "sess-gap049-happy"
	d1 := postProcess(t, ts, sid).DecisionID

	status, raw := postResult(t, ts, sid, d1, "tool_result")
	if status != http.StatusOK {
		t.Fatalf("expected 200 for the in-flight decision_id, got %d (%s)", status, raw)
	}
	var dec protocol.Decision
	if err := json.Unmarshal(raw, &dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	if dec.DecisionID != "dec-happy-002" {
		t.Errorf("expected next decision dec-happy-002, got %q", dec.DecisionID)
	}
	if m.resultCalls != 1 {
		t.Errorf("expected OnResult to run once, got %d", m.resultCalls)
	}

	sr := getSession(t, ts, sid)
	if sr.CurrentDecision != "dec-happy-002" {
		t.Errorf("expected current_decision dec-happy-002 (the NEW decision), got %q", sr.CurrentDecision)
	}
	if sr.CurrentDecisionType != protocol.DecisionText {
		t.Errorf("expected current_decision_type text, got %q", sr.CurrentDecisionType)
	}
}

// TestResultEndpoint_NoDecisionInFlightAccepts (GAP-049): a session with NO
// decision in flight (POST /v1/process still running — the harness has not
// answered yet) accepts the result exactly as before. There is nothing to
// correlate against, so rejecting it would be a new failure mode rather than a
// guard.
func TestResultEndpoint_NoDecisionInFlightAccepts(t *testing.T) {
	m := newMockHarness()
	bh := &blockingHarness{
		mockHarness:    *m,
		processStarted: make(chan struct{}),
		release:        make(chan struct{}),
	}
	srv := NewHTTPServer(bh)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const sid = "sess-gap049-noflight"
	go func() {
		resp, err := http.Post(ts.URL+"/v1/process", "application/json",
			strings.NewReader(processBody(sid)))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// OnProcess is blocked, so the session exists with no decision finalized.
	<-bh.processStarted

	status, raw := postResult(t, ts, sid, "whatever-id", "tool_result")
	if status != http.StatusOK {
		t.Errorf("expected 200 while nothing is in flight, got %d (%s)", status, raw)
	}

	close(bh.release)
}

// TestResultEndpoint_Validation (GAP-049): session_id, decision_id and
// result.type are required. Each omission is 400 INVALID_REQUEST, and none of
// them may reach OnResult.
func TestResultEndpoint_Validation(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "empty decision_id",
			body:    `{"session_id": "sess-gap049-valid", "decision_id": "", "result": {"type": "tool_result", "success": true}}`,
			wantMsg: "decision_id is required",
		},
		{
			name:    "empty session_id",
			body:    `{"session_id": "", "decision_id": "dec-anything", "result": {"type": "tool_result", "success": true}}`,
			wantMsg: "session_id is required",
		},
		{
			name:    "empty result.type",
			body:    `{"session_id": "sess-gap049-valid", "decision_id": "dec-anything", "result": {"success": true}}`,
			wantMsg: "result.type is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockHarness()
			srv := NewHTTPServer(m)
			ts := httptest.NewServer(srv)
			defer ts.Close()

			resp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST /v1/result: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
			var errResp protocol.ErrorResponse
			if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if errResp.Error.Code != protocol.ErrInvalidRequest {
				t.Errorf("expected ErrInvalidRequest, got %q", errResp.Error.Code)
			}
			if !strings.Contains(errResp.Error.Message, tc.wantMsg) {
				t.Errorf("expected message containing %q, got %q", tc.wantMsg, errResp.Error.Message)
			}
			if m.resultCalls != 0 || m.lastResultReq != nil {
				t.Errorf("OnResult must NOT be called for an invalid result request (calls=%d)", m.resultCalls)
			}
		})
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

	// GAP-014: after DELETE, the session entry must be removed from the store
	// so GET /v1/sessions/{id} returns 404 (not 200 with a stale entry).
	getResp, err := http.Get(ts.URL + "/v1/sessions/sess-d1")
	if err != nil {
		t.Fatalf("GET /v1/sessions/sess-d1 after delete: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()

	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
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

// TestSessionLifecycle_ResultEndMarksCompleted (GAP-DOG-003):
// process -> harness returns text -> POST /v1/result with harness returning
// DecisionEnd -> GET session -> status "completed". Also assert
// current_decision is still populated (GAP-009 regression guard).
func TestSessionLifecycle_ResultEndMarksCompleted(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-lc1-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionEnd,
		DecisionID: "dec-lc1-end",
		End:        &protocol.End{Reason: protocol.EndTaskComplete, Summary: "done"},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-lc1")

	resultBody := `{
		"session_id": "sess-lc1",
		"decision_id": "dec-lc1-proc",
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
	if resDec.Decision != protocol.DecisionEnd {
		t.Fatalf("expected decision end, got %q", resDec.Decision)
	}

	sr := getSession(t, ts, "sess-lc1")
	if sr.Status != protocol.SessionCompleted {
		t.Errorf("expected status %q, got %q", protocol.SessionCompleted, sr.Status)
	}
	// GAP-009 regression guard: current_decision must still be populated.
	if sr.CurrentDecision != "dec-lc1-end" {
		t.Errorf("expected current_decision dec-lc1-end, got %q", sr.CurrentDecision)
	}
	if sr.CurrentDecisionType != protocol.DecisionEnd {
		t.Errorf("expected current_decision_type %q, got %q", protocol.DecisionEnd, sr.CurrentDecisionType)
	}
}

// TestSessionLifecycle_ProcessEndMarksCompleted (GAP-DOG-003):
// harness returns DecisionEnd directly from OnProcess -> GET session ->
// status "completed".
func TestSessionLifecycle_ProcessEndMarksCompleted(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionEnd,
		DecisionID: "dec-lc2-end",
		End:        &protocol.End{Reason: protocol.EndTaskComplete, Summary: "done"},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	dec := postProcess(t, ts, "sess-lc2")
	if dec.Decision != protocol.DecisionEnd {
		t.Fatalf("expected decision end, got %q", dec.Decision)
	}

	sr := getSession(t, ts, "sess-lc2")
	if sr.Status != protocol.SessionCompleted {
		t.Errorf("expected status %q, got %q", protocol.SessionCompleted, sr.Status)
	}
	if sr.CurrentDecision != "dec-lc2-end" {
		t.Errorf("expected current_decision dec-lc2-end, got %q", sr.CurrentDecision)
	}
}

// TestSessionLifecycle_NonEndKeepsActive (GAP-DOG-003):
// text decision via result -> GET session -> status "active" (no premature
// completion).
func TestSessionLifecycle_NonEndKeepsActive(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-lc3-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-lc3-text",
		Text:       &protocol.TextResp{Content: "final answer", Finished: true},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postProcess(t, ts, "sess-lc3")

	resultBody := `{
		"session_id": "sess-lc3",
		"decision_id": "dec-lc3-proc",
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
	if resDec.Decision != protocol.DecisionText {
		t.Fatalf("expected decision text, got %q", resDec.Decision)
	}

	sr := getSession(t, ts, "sess-lc3")
	if sr.Status != protocol.SessionActive {
		t.Errorf("expected status %q, got %q", protocol.SessionActive, sr.Status)
	}
}

// TestSessionLifecycle_CancelledIsTerminalOnLateResult (GAP-028):
// process -> cancel (status cancelled) -> late result (harness returns
// DecisionEnd) -> GET /v1/sessions/{id} status stays "cancelled" and
// turn_count unchanged (1). The late result must NOT resurrect the session
// to "completed" or increment turn_count.
func TestSessionLifecycle_CancelledIsTerminalOnLateResult(t *testing.T) {
	m := newMockHarness()
	m.onProcessDec = &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-gap028-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}
	m.onResultDec = &protocol.Decision{
		Decision:   protocol.DecisionEnd,
		DecisionID: "dec-gap028-end",
		End:        &protocol.End{Reason: protocol.EndTaskComplete, Summary: "done"},
	}
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. process — creates session, turn_count becomes 1.
	postProcess(t, ts, "sess-gap028")

	// 2. cancel — sets status to "cancelled".
	cancelResp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "sess-gap028", "reason": "user_interrupt"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	defer func() { _ = cancelResp.Body.Close() }()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", cancelResp.StatusCode)
	}

	// Verify status is cancelled right after cancel.
	sr := getSession(t, ts, "sess-gap028")
	if sr.Status != protocol.SessionCancelled {
		t.Fatalf("expected status %q after cancel, got %q", protocol.SessionCancelled, sr.Status)
	}
	if sr.TurnCount != 1 {
		t.Fatalf("expected turn_count 1 after process+cancel, got %d", sr.TurnCount)
	}

	// 3. late result — harness returns DecisionEnd, but session is cancelled.
	// The result must NOT overwrite status or increment turn_count.
	resultBody := `{
		"session_id": "sess-gap028",
		"decision_id": "dec-gap028-proc",
		"result": {"type": "tool_result", "tool_name": "test", "success": true}
	}`
	resultResp, err := http.Post(ts.URL+"/v1/result", "application/json", strings.NewReader(resultBody))
	if err != nil {
		t.Fatalf("POST /v1/result: %v", err)
	}
	defer func() { _ = resultResp.Body.Close() }()
	if resultResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (permissive), got %d", resultResp.StatusCode)
	}

	// 4. GET session — status must still be "cancelled", turn_count still 1.
	sr = getSession(t, ts, "sess-gap028")
	if sr.Status != protocol.SessionCancelled {
		t.Errorf("expected status %q (cancelled is terminal), got %q", protocol.SessionCancelled, sr.Status)
	}
	if sr.TurnCount != 1 {
		t.Errorf("expected turn_count 1 (unchanged by late result), got %d", sr.TurnCount)
	}
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

func TestUnknownRouteReturnsJSONNotFound(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/nonexistent-path")
	if err != nil {
		t.Fatalf("GET /v1/nonexistent-path: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %q", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error message")
	}
}

// GAP-043 -------------------------------------------------------------------
//
// The session store hands out RAW *sessionEntry pointers from get(): the
// handler-side reads of that struct happen with NO lock held, while
// POST /v1/result mutates the same fields inside update() (which does hold the
// write lock). Locked writes racing unlocked reads on one struct is a genuine
// data race whenever a single session is hit by concurrent requests.

// concurrentHarness is a Harness implementation that is itself race-free under
// concurrent handler calls. The shared mockHarness records lastResultReq into a
// plain field on every OnResult, so N concurrent POST /v1/result calls would
// make the race detector report the TEST's own field instead of the production
// bug under test.
type concurrentHarness struct {
	mu           sync.Mutex
	processCalls int
	resultCalls  int
}

func (h *concurrentHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	h.processCalls++
	h.mu.Unlock()
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-race-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}, nil
}

func (h *concurrentHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	h.resultCalls++
	h.mu.Unlock()
	// GAP-049 fixture adaptation: echo the decision_id the result was FOR back
	// as the next decision's id, so the id stays the session's in-flight
	// decision across iterations. Without this the first accepted result would
	// move CurrentDecisionID to a fixed id and every concurrent writer still
	// posting "dec-race-proc" would be rejected as already resolved — which is
	// the GAP-049 rule working, not the race under test. This test is about
	// the unlocked-read race (GAP-043) and must keep every writer's POST
	// landing, exactly as before.
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: req.DecisionID,
		Text:       &protocol.TextResp{Content: "result received", Finished: true},
	}, nil
}

func (h *concurrentHarness) OnCancel(req *protocol.CancelRequest) error { return nil }

func (h *concurrentHarness) OnSessionTerminate(sessionID string) error { return nil }

func (h *concurrentHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities:    []protocol.DecisionType{protocol.DecisionText},
	}
}

// resultJSON builds a minimal valid POST /v1/result body for session sid.
func resultJSON(sid, decisionID string) string {
	return `{"session_id": "` + sid + `", "decision_id": "` + decisionID +
		`", "result": {"type": "tool_result", "tool_name": "test", "success": true}}`
}

// TestConcurrentSameSessionRequestsNoRace (GAP-043): ONE session, then N>=8
// goroutines concurrently issue POST /v1/result (locked writers) and
// GET /v1/sessions/{id} (unlocked readers) against that same session id.
// Pre-fix this fails with "WARNING: DATA RACE" against harness.go; post-fix it
// passes with 0 races. The final turn-count assertion also pins that the fix is
// behavior-preserving: every result POST must still land its increment.
func TestConcurrentSameSessionRequestsNoRace(t *testing.T) {
	const (
		sessionID  = "sess-race-1"
		workers    = 16 // >= 8 by acceptance criteria; half writers, half readers
		iterations = 25
	)
	writers := workers / 2

	srv := NewHTTPServer(&concurrentHarness{})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create the ONE shared session up front (turn_count becomes 1).
	postProcess(t, ts, sessionID)

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		//nolint:gosec // loop var is captured by value via the parameter.
		go func(i int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if i%2 == 0 {
					// Locked writer: update() mutates LastActive/TurnCount/
					// CurrentDecisionID/CurrentDecisionType/Status.
					resp, err := http.Post(ts.URL+"/v1/result", "application/json",
						strings.NewReader(resultJSON(sessionID, "dec-race-proc")))
					if err != nil {
						errCh <- fmt.Errorf("worker %d: POST /v1/result: %w", i, err)
						return
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						errCh <- fmt.Errorf("worker %d: POST /v1/result: expected 200, got %d", i, resp.StatusCode)
						return
					}
					continue
				}

				// Unlocked reader: getSessionHandler dereferences the raw
				// *sessionEntry returned by get().
				resp, err := http.Get(ts.URL + "/v1/sessions/" + sessionID)
				if err != nil {
					errCh <- fmt.Errorf("worker %d: GET /v1/sessions/%s: %w", i, sessionID, err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("worker %d: GET /v1/sessions/%s: expected 200, got %d", i, sessionID, resp.StatusCode)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Behavior preservation: 1 turn from POST /v1/process + one increment per
	// concurrent POST /v1/result, status never leaves "active".
	sr := getSession(t, ts, sessionID)
	if sr.Status != protocol.SessionActive {
		t.Errorf("expected status %q, got %q", protocol.SessionActive, sr.Status)
	}
	if want := 1 + writers*iterations; sr.TurnCount != want {
		t.Errorf("expected turn_count %d, got %d", want, sr.TurnCount)
	}
}

func TestWrongMethodReturnsJSONMethodNotAllowed(t *testing.T) {
	m := newMockHarness()
	srv := NewHTTPServer(m)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/health", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var errResp protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != protocol.ErrMethodNotAllowed {
		t.Errorf("expected ErrMethodNotAllowed, got %q", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error message")
	}
}
