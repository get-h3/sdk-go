package harness

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/get-h3/sdk-go/protocol"
)

// This file holds the regression tests for three findings that all live in
// harness/harness.go and therefore ship as one change set:
//
//	SDKGO-GAP-058 — the result-admission guard was check-then-act: two
//	                concurrent deliveries of one decision_id could both pass
//	                the correlation check and run OnResult twice.
//	SDKGO-GAP-059 — create-or-update reset turn_count/started_at on EVERY
//	                POST /v1/process, contradicting the documented re-open
//	                semantics (reset only when re-opening a COMPLETED session).
//	SDKGO-GAP-060 — every POST handler decoded r.Body with no size cap.

// ---------------------------------------------------------------------------
// SDKGO-GAP-058 — one decision_id, N concurrent deliveries, ONE OnResult call
// ---------------------------------------------------------------------------

// countingResultHarness counts harness callbacks and answers an accepted
// result with a NEW decision id, exactly like a harness whose loop advances.
// onResultDelay holds OnResult open long enough that every other concurrent
// delivery of the same decision_id is guaranteed to arrive while it runs —
// which is the window in which the pre-fix handler ran OnResult once per
// delivery instead of once per decision_id.
type countingResultHarness struct {
	mu            sync.Mutex
	processCalls  int
	resultCalls   int
	onResultDelay time.Duration
}

func (h *countingResultHarness) OnProcess(_ *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	h.processCalls++
	h.mu.Unlock()
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-claim-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}, nil
}

func (h *countingResultHarness) OnResult(_ *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	h.resultCalls++
	delay := h.onResultDelay
	h.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-claim-next",
		Text:       &protocol.TextResp{Content: "result applied", Finished: true},
	}, nil
}

func (h *countingResultHarness) OnCancel(_ *protocol.CancelRequest) error { return nil }

func (h *countingResultHarness) OnSessionTerminate(_ string) error { return nil }

func (h *countingResultHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities:    []protocol.DecisionType{protocol.DecisionText},
	}
}

func (h *countingResultHarness) counts() (process, result int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.processCalls, h.resultCalls
}

// postResultFromGoroutine is postResult for a non-test goroutine: it reports
// transport errors instead of calling t.Fatalf (FailNow is only valid on the
// goroutine running the test).
func postResultFromGoroutine(ts *httptest.Server, sid, decisionID string) (int, []byte, error) {
	resp, err := http.Post(ts.URL+"/v1/result", "application/json",
		strings.NewReader(resultJSON(sid, decisionID)))
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// TestResultEndpoint_ConcurrentDuplicateDeliveredOnce (SDKGO-GAP-058): N
// goroutines fire the SAME result (same decision_id) in parallel. The harness
// must see OnResult EXACTLY once, one delivery must be accepted, and every
// other delivery must be refused 400 INVALID_REQUEST.
//
// Red proof: before the fix the correlation read (a snapshot) and the
// LastResultDecisionID record were two separate lock acquisitions with
// OnResult in between, so all N deliveries passed the check while the first
// one was still inside OnResult — N side effects for one decision (this test
// measured OnResult==N, and N-1 extra turn counts).
func TestResultEndpoint_ConcurrentDuplicateDeliveredOnce(t *testing.T) {
	const (
		sessionID = "sess-gap058-dup"
		workers   = 8
	)

	h := &countingResultHarness{onResultDelay: 20 * time.Millisecond}
	srv := NewHTTPServer(h)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	decisionID := postProcess(t, ts, sessionID).DecisionID
	if decisionID == "" {
		t.Fatal("POST /v1/process returned an empty decision_id; cannot correlate")
	}

	type outcome struct {
		status int
		body   []byte
		err    error
	}
	outcomes := make([]outcome, workers)

	// Release every delivery at the same instant, so the overlap the fix has
	// to prevent is not left to scheduling luck.
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			status, raw, err := postResultFromGoroutine(ts, sessionID, decisionID)
			outcomes[i] = outcome{status: status, body: raw, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	accepted, refused := 0, 0
	for i, o := range outcomes {
		if o.err != nil {
			t.Errorf("delivery %d: POST /v1/result: %v", i, o.err)
			continue
		}
		switch o.status {
		case http.StatusOK:
			accepted++
		case http.StatusBadRequest:
			refused++
			var errResp protocol.ErrorResponse
			if err := json.Unmarshal(o.body, &errResp); err != nil {
				t.Errorf("delivery %d: decode 400 body %q: %v", i, o.body, err)
				continue
			}
			if errResp.Error.Code != protocol.ErrInvalidRequest {
				t.Errorf("delivery %d: expected %q, got %q", i, protocol.ErrInvalidRequest, errResp.Error.Code)
			}
			// Deterministic post-fix message: the losing deliveries are
			// answered after the winner advanced the session, so the id reads
			// as already resolved. (The "already being processed" variant can
			// only be observed if a delivery ever bypassed the session's
			// delivery lock.)
			if !strings.Contains(errResp.Error.Message, "already been resolved") {
				t.Errorf("delivery %d: expected 'already been resolved', got %q", i, errResp.Error.Message)
			}
		default:
			t.Errorf("delivery %d: expected 200 or 400, got %d (%s)", i, o.status, o.body)
		}
	}

	if accepted != 1 {
		t.Errorf("expected exactly 1 accepted delivery of %q, got %d", decisionID, accepted)
	}
	if refused != workers-1 {
		t.Errorf("expected %d refused deliveries, got %d", workers-1, refused)
	}

	if _, resultCalls := h.counts(); resultCalls != 1 {
		t.Errorf("OnResult ran %d times for one decision_id delivered %d times: the harness side effect must run exactly once",
			resultCalls, workers)
	}

	// One turn for the process and one for the single accepted result — the
	// refused deliveries must not have counted.
	if sr := getSession(t, ts, sessionID); sr.TurnCount != 2 {
		t.Errorf("expected turn_count 2 (process + one accepted result), got %d", sr.TurnCount)
	}
	if sr := getSession(t, ts, sessionID); sr.CurrentDecision != "dec-claim-next" {
		t.Errorf("expected current_decision dec-claim-next, got %q", sr.CurrentDecision)
	}
}

// flakyResultHarness fails its first OnResult — either with an error or with a
// decision that fails validation — and then succeeds. It is the fixture for the
// claim-release half of SDKGO-GAP-058: a delivery that does NOT resolve its
// decision must leave the session exactly as it found it, so a genuine retry of
// the same decision_id is admitted instead of being refused as still-claimed.
type flakyResultHarness struct {
	mu       sync.Mutex
	calls    int
	failMode string // "error" or "invalid-decision"
}

func (h *flakyResultHarness) OnProcess(_ *protocol.ProcessRequest) (*protocol.Decision, error) {
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-release-proc",
		Text:       &protocol.TextResp{Content: "thinking...", Finished: false},
	}, nil
}

func (h *flakyResultHarness) OnResult(_ *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	h.calls++
	first := h.calls == 1
	mode := h.failMode
	h.mu.Unlock()

	if first {
		switch mode {
		case "error":
			return nil, errors.New("harness blew up inside OnResult")
		case "invalid-decision":
			// No text payload: Decision.Validate() rejects it.
			return &protocol.Decision{Decision: protocol.DecisionText, DecisionID: "dec-release-invalid"}, nil
		}
	}
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-release-next",
		Text:       &protocol.TextResp{Content: "applied", Finished: true},
	}, nil
}

func (h *flakyResultHarness) OnCancel(_ *protocol.CancelRequest) error { return nil }

func (h *flakyResultHarness) OnSessionTerminate(_ string) error { return nil }

func (h *flakyResultHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
	}
}

// TestResultEndpoint_ClaimReleasedOnFailedDelivery (SDKGO-GAP-058): a delivery
// whose OnResult fails (error, or a decision that fails validation) must
// release the claim it took, leaving the decision_id retryable — and must not
// advance the session's decision bookkeeping. This is the guard against
// "fixing" the double-side-effect race by wedging every delivery of an id that
// failed once.
func TestResultEndpoint_ClaimReleasedOnFailedDelivery(t *testing.T) {
	cases := []struct {
		mode      string
		wantCode  protocol.ErrorCode
		wantState int
	}{
		{mode: "error", wantCode: protocol.ErrInternalError, wantState: http.StatusInternalServerError},
		{mode: "invalid-decision", wantCode: protocol.ErrInvalidDecision, wantState: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			h := &flakyResultHarness{failMode: tc.mode}
			srv := NewHTTPServer(h)
			ts := httptest.NewServer(srv)
			defer ts.Close()

			const sid = "sess-gap058-release"
			decisionID := postProcess(t, ts, sid).DecisionID

			// 1. The delivery fails.
			status, raw := postResult(t, ts, sid, decisionID, "tool_result")
			if status != tc.wantState {
				t.Fatalf("failing delivery: expected %d, got %d (%s)", tc.wantState, status, raw)
			}
			var errResp protocol.ErrorResponse
			if err := json.Unmarshal(raw, &errResp); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if errResp.Error.Code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, errResp.Error.Code)
			}

			// 2. The session did not resolve that decision: it is still the
			// in-flight decision, so the retry below has something to correlate
			// against. (turn_count already counted the admitted delivery — that
			// is the pre-existing behaviour, unchanged here.)
			sr := getSession(t, ts, sid)
			if sr.CurrentDecision != decisionID {
				t.Errorf("a failed delivery advanced the session: current_decision %q, want %q", sr.CurrentDecision, decisionID)
			}
			if sr.TurnCount != 2 {
				t.Errorf("expected turn_count 2 (process + admitted result), got %d", sr.TurnCount)
			}

			// 3. The retry of the SAME decision_id must be admitted — the claim
			// is gone, and the bookkeeping never advanced.
			status, raw = postResult(t, ts, sid, decisionID, "tool_result")
			if status != http.StatusOK {
				t.Fatalf("retry after a failed delivery: expected 200, got %d (%s)", status, raw)
			}
			var dec protocol.Decision
			if err := json.Unmarshal(raw, &dec); err != nil {
				t.Fatalf("decode retry decision: %v", err)
			}
			if dec.DecisionID != "dec-release-next" {
				t.Errorf("expected the retry to reach OnResult (decision dec-release-next), got %q", dec.DecisionID)
			}
			if sr := getSession(t, ts, sid); sr.CurrentDecision != "dec-release-next" {
				t.Errorf("expected current_decision dec-release-next after the retry, got %q", sr.CurrentDecision)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SDKGO-GAP-059 — accumulate on an active session, reset only when re-opening
// ---------------------------------------------------------------------------

// TestProcessEndpoint_TurnAccountingMatchesDocumentedSemantics (SDKGO-GAP-059):
// a repeated POST /v1/process on an ACTIVE session is another turn of the same
// session — turn_count accumulates and started_at is preserved. Only a
// COMPLETED session is re-opened with started_at/turn_count reset, and a
// CANCELLED session stays terminal (GAP-028).
//
// Red proof: before the fix every POST /v1/process rebuilt the session entry,
// so this test saw turn_count 1/1/1 (never 2 or 3) and a fresh started_at on
// each call.
func TestProcessEndpoint_TurnAccountingMatchesDocumentedSemantics(t *testing.T) {
	m := newMockHarness()
	// White-box server (not NewHTTPServer) so started_at can be compared at
	// full precision: GET /v1/sessions/{id} formats it as RFC3339, whose
	// one-second resolution would hide a reset that happens inside the same
	// second as the previous one.
	srv := newServer(m)
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	const sid = "sess-gap059-turns"
	startedAt := func() time.Time {
		entry, ok := srv.sessions.snapshot(sid)
		if !ok {
			t.Fatalf("session %q missing from the store", sid)
		}
		return entry.StartedAt
	}

	// Turn 1 — the session is created.
	postProcess(t, ts, sid)
	firstStart := startedAt()
	if sr := getSession(t, ts, sid); sr.TurnCount != 1 || sr.Status != protocol.SessionActive {
		t.Fatalf("after the first process: expected turn_count 1 / active, got %d / %q", sr.TurnCount, sr.Status)
	}

	// Turn 2 — same session, still active: accumulate, do not restart.
	postProcess(t, ts, sid)
	if sr := getSession(t, ts, sid); sr.TurnCount != 2 {
		t.Errorf("repeated POST /v1/process on an active session: turn_count %d, want 2 (a repeat is another turn, not a new session)", sr.TurnCount)
	}
	if got := startedAt(); !got.Equal(firstStart) {
		t.Errorf("started_at moved on a repeated POST /v1/process for an active session: %s -> %s", firstStart, got)
	}

	// Turn 3 — and it keeps accumulating.
	postProcess(t, ts, sid)
	if sr := getSession(t, ts, sid); sr.TurnCount != 3 {
		t.Errorf("third POST /v1/process: turn_count %d, want 3", sr.TurnCount)
	}
	activeStart := startedAt()

	// Complete the session: the mock's OnResult answers the in-flight decision
	// with an `end` decision.
	status, raw := postResult(t, ts, sid, "dec-test-001", "tool_result")
	if status != http.StatusOK {
		t.Fatalf("result driving the session to completion: expected 200, got %d (%s)", status, raw)
	}
	if sr := getSession(t, ts, sid); sr.Status != protocol.SessionCompleted {
		t.Fatalf("expected the session to be completed, got %q", sr.Status)
	}

	// Re-open — a completed session IS reset: back to active, started_at and
	// turn_count restart, and the re-opening call is turn 1.
	postProcess(t, ts, sid)
	sr := getSession(t, ts, sid)
	if sr.Status != protocol.SessionActive {
		t.Errorf("re-opened session: expected status %q, got %q", protocol.SessionActive, sr.Status)
	}
	if sr.TurnCount != 1 {
		t.Errorf("re-opened session: expected turn_count 1 (the re-opening process turn), got %d", sr.TurnCount)
	}
	if got := startedAt(); !got.After(activeStart) {
		t.Errorf("re-opened session: started_at was not reset (still %s)", got)
	}

	// Cancelled stays terminal: a late POST /v1/process neither revives the
	// session nor advances turn_count (GAP-028, still holding under the new
	// accounting).
	cancelResp, err := http.Post(ts.URL+"/v1/cancel", "application/json",
		strings.NewReader(`{"session_id": "`+sid+`", "reason": "user_interrupt"}`))
	if err != nil {
		t.Fatalf("POST /v1/cancel: %v", err)
	}
	_, _ = io.Copy(io.Discard, cancelResp.Body)
	_ = cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/cancel: expected 200, got %d", cancelResp.StatusCode)
	}

	postProcess(t, ts, sid)
	sr = getSession(t, ts, sid)
	if sr.Status != protocol.SessionCancelled {
		t.Errorf("late process on a cancelled session: status %q, want %q (cancelled is terminal)", sr.Status, protocol.SessionCancelled)
	}
	if sr.TurnCount != 1 {
		t.Errorf("late process on a cancelled session: turn_count %d, want 1 (unchanged)", sr.TurnCount)
	}
}

// ---------------------------------------------------------------------------
// SDKGO-GAP-060 — POST bodies are capped before they are decoded
// ---------------------------------------------------------------------------

// fillerBody streams a valid JSON prefix followed by filler characters up to
// total bytes, without ever allocating the body. It is what an unbounded body
// looks like from the handler's side, and it keeps the allocation the test
// MEASURES attributable to the handler rather than to the test.
type fillerBody struct {
	prefix  []byte
	total   int
	emitted int
}

func newFillerBody(sid string, total int) *fillerBody {
	return &fillerBody{
		prefix: []byte(`{"session_id":"` + sid + `","message":{"role":"user","content":"`),
		total:  total,
	}
}

func (b *fillerBody) Read(p []byte) (int, error) {
	if b.emitted >= b.total {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && b.emitted < b.total {
		if b.emitted < len(b.prefix) {
			p[n] = b.prefix[b.emitted]
		} else {
			p[n] = 'a'
		}
		n++
		b.emitted++
	}
	return n, nil
}

// TestPostHandlersRejectOversizedBody (SDKGO-GAP-060): every POST handler caps
// the body it will decode. An oversized body gets a JSON ErrorResponse and
// never reaches the harness; a normal-size request is unaffected.
//
// Red proof: before the fix the handlers fed r.Body straight to json.Decoder,
// so this request was decoded until EOF — the handler allocated the whole body
// (the memory assertion below fails pre-fix) and the harness was called.
func TestPostHandlersRejectOversizedBody(t *testing.T) {
	// The production default the handlers are wired with. The in-process
	// sub-tests inject a smaller cap so they can prove the memory bound without
	// moving 10 MiB per case; the socket sub-test drives the real default.
	if got := newServer(newMockHarness()).bodyLimit(); got != maxRequestBodyBytes {
		t.Errorf("default body limit = %d, want maxRequestBodyBytes (%d)", got, maxRequestBodyBytes)
	}

	const (
		limit    = 4 << 10 // injected cap for the in-process cases
		bodySize = 4 << 20 // 1000x the cap: unbounded if nothing caps it
	)

	cases := []struct{ name, path, sid string }{
		{"process", "/v1/process", "sess-cap-process"},
		{"result", "/v1/result", "sess-cap-result"},
		{"cancel", "/v1/cancel", "sess-cap-cancel"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockHarness()
			srv := newServer(m)
			srv.maxBodyBytes = limit
			handler := srv.handler()

			req := httptest.NewRequest(http.MethodPost, tc.path, newFillerBody(tc.sid, bodySize))
			rec := httptest.NewRecorder()

			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			started := time.Now()
			handler.ServeHTTP(rec, req)
			elapsed := time.Since(started)
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc

			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("%s with a %d-byte body against a %d-byte cap: expected 413, got %d (%s)",
					tc.path, bodySize, limit, rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
			var errResp protocol.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("decode error response %q: %v", rec.Body.String(), err)
			}
			if errResp.Error.Code != protocol.ErrInvalidRequest {
				t.Errorf("expected code %q, got %q", protocol.ErrInvalidRequest, errResp.Error.Code)
			}
			if !strings.Contains(errResp.Error.Message, "exceeds") {
				t.Errorf("expected the refusal to name the limit, got %q", errResp.Error.Message)
			}

			// The point of the cap: the memory one request can cost is bounded
			// by the limit, not by how much the caller sends.
			if allocated > 1<<20 {
				t.Errorf("handler allocated %d bytes for a %d-byte body against a %d-byte cap (budget 1 MiB): the body cap must bound it",
					allocated, bodySize, limit)
			}
			if elapsed > 5*time.Second {
				t.Errorf("oversized body took %s to reject; it must fail fast", elapsed)
			}

			// Refused before any harness callback.
			if m.lastProcessReq != nil || m.lastResultReq != nil || m.cancelCalled {
				t.Error("the harness was called for a request that never got past body admission")
			}
		})
	}

	t.Run("normal-size request unchanged", func(t *testing.T) {
		m := newMockHarness()
		srv := newServer(m)
		srv.maxBodyBytes = limit
		ts := httptest.NewServer(srv.handler())
		defer ts.Close()

		// The standard, well-formed request body is far below any cap and must
		// behave exactly as before the cap existed.
		dec := postProcess(t, ts, "sess-cap-normal")
		if dec.DecisionID != "dec-test-001" {
			t.Errorf("expected decision_id dec-test-001, got %q", dec.DecisionID)
		}
		if m.lastProcessReq == nil {
			t.Error("OnProcess was not called for a normal-size request")
		}
	})

	t.Run("default cap over a real socket", func(t *testing.T) {
		m := newMockHarness()
		ts := httptest.NewServer(NewHTTPServer(m))
		defer ts.Close()

		// One byte past the production cap, streamed (no test-side allocation).
		body := newFillerBody("sess-cap-default", int(maxRequestBodyBytes)+1)
		resp, err := http.Post(ts.URL+"/v1/process", "application/json", body)
		if err != nil {
			t.Fatalf("POST with an oversized body: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 413 from the default cap, got %d (%s)", resp.StatusCode, raw)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read 413 body: %v", err)
		}
		var errResp protocol.ErrorResponse
		if err := json.Unmarshal(raw, &errResp); err != nil {
			t.Fatalf("decode 413 body %q: %v", raw, err)
		}
		if errResp.Error.Code != protocol.ErrInvalidRequest {
			t.Errorf("expected code %q, got %q", protocol.ErrInvalidRequest, errResp.Error.Code)
		}
		if m.lastProcessReq != nil {
			t.Error("the harness was called for an oversized request")
		}
	})
}
