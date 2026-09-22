// Package testbed provides MockHermes and assertion helpers
// for unit-testing H3 harness logic.
//
// # Argument and return types — the two easy mistakes
//
// NewMockHermes takes a harness.Harness (your harness), and SendMessage,
// SendMessageWithHistory and SendResult return a *protocol.Decision. Nothing in
// this package accepts or returns an http.Handler, and nothing answers a bool:
//
//   - NewMockHermes(h harness.Harness) drives the harness in process. Passing
//     the http.Handler returned by harness.NewHTTPServer(h) does not compile —
//     `http.Handler does not implement harness.Harness (missing method
//     Health)`. That error names a LAYER MIX-UP, not a missing method on your
//     harness: the handler is the HTTP front end that calls the harness, and
//     only the harness has Health/OnProcess/OnResult/OnCancel/
//     OnSessionTerminate.
//   - To exercise the same harness over HTTP, pass the HARNESS (not the mock,
//     never the handler) to harness.NewHTTPServer and serve or post to the
//     handler it returns. NewMockHermesWithServer returns the mock and that
//     handler together, so the order cannot be inverted.
//   - A Decision is asserted field by field — dec.Decision, dec.Text,
//     dec.Text.Content, dec.Text.Finished, dec.End, … — next to the error. A
//     non-nil Decision says nothing about err, and a nil err says nothing about
//     the Decision's payload fields.
//
// # The full pattern
//
// Mock in process and the same harness over HTTP, one message each, asserted in
// turn:
//
//	h := &MyHarness{}
//	mh := testbed.NewMockHermes(h) // harness.Harness in
//	dec, err := mh.SendMessage("s1", "hello mock", "alice", "u1")
//	// dec is *protocol.Decision — dec.Decision, dec.Text.Content, dec.Text.Finished, …
//
//	ts := httptest.NewServer(harness.NewHTTPServer(h)) // the HTTP layer, over the harness
//	defer ts.Close()
//	resp, err := http.Post(ts.URL+"/v1/process", "application/json", body)
//	var overHTTP protocol.Decision
//	err = json.NewDecoder(resp.Body).Decode(&overHTTP)
//
// Both paths reach the same harness methods, so the Decision decoded from JSON
// agrees field for field with the one the mock returned in process. See
// ExampleNewMockHermes for the runnable form.
package testbed

import (
	"fmt"
	"net/http"
	"time"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// MockHermes wraps a harness.Harness and provides methods to drive it in tests.
// It simulates the Hermes runtime by calling the harness methods and tracking
// the results for test assertions.
type MockHermes struct {
	Harness harness.Harness

	// Tracking fields for test assertions
	LastDecision  *protocol.Decision
	LastError     error
	Decisions     []*protocol.Decision
	SessionCount  int
	decisionCount int
}

// NewMockHermes creates a MockHermes wrapping the given harness.
//
// The argument is a harness.Harness — the interface your harness implements
// (OnProcess, OnResult, OnCancel, OnSessionTerminate, Health). It is NOT an
// http.Handler: passing the handler returned by harness.NewHTTPServer(h) fails
// to compile with
//
//	http.Handler does not implement harness.Harness (missing method Health)
//
// which reports the layer mix-up, not a missing method on your harness. The two
// layers stack rather than substitute: the handler is the HTTP front end that
// calls the harness, and only the harness has Health and the On* methods. To
// drive the same harness over HTTP, wrap the HARNESS with harness.NewHTTPServer
// and post to the handler it returns.
//
// Minimal correct pattern — mock, then the HTTP layer, then one request, then a
// field-by-field assert on the returned Decision:
//
//	h := &MyHarness{}
//	mh := NewMockHermes(h) // a harness.Harness goes in
//
//	// In process: the mock drives the same harness directly.
//	dec, err := mh.SendMessage("s1", "hello mock", "alice", "u1")
//	if err != nil {
//		t.Fatal(err)
//	}
//	if dec.Decision != protocol.DecisionText || dec.Text == nil ||
//		dec.Text.Content != "Echo: hello mock" || !dec.Text.Finished {
//		t.Fatalf("unexpected decision: %+v", dec)
//	}
//
//	// Over HTTP: wrap the HARNESS — never the mock, never the handler — and post.
//	ts := httptest.NewServer(harness.NewHTTPServer(h))
//	defer ts.Close()
//	resp, err := http.Post(ts.URL+"/v1/process", "application/json", body)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer resp.Body.Close()
//	var overHTTP protocol.Decision
//	if err := json.NewDecoder(resp.Body).Decode(&overHTTP); err != nil {
//		t.Fatal(err)
//	}
//	// overHTTP.Decision / overHTTP.Text.Content / overHTTP.Text.Finished — the
//	// same fields as the in-process Decision above.
//
// SendMessage returns (*protocol.Decision, error) — a Decision POINTER and an
// error, never a bool. A bool lives inside the Decision (TextResp.Finished as
// dec.Text.Finished, or Result.Success); read it off there.
//
// NewMockHermesWithServer returns this mock together with the HTTP handler for
// the same harness, so the two layers cannot be swapped.
func NewMockHermes(h harness.Harness) *MockHermes {
	return &MockHermes{
		Harness: h,
	}
}

// NewMockHermesWithServer builds a MockHermes for h and the HTTP handler that
// fronts the SAME harness, in the one order that works: a harness.Harness goes
// in, and the returned http.Handler is what an httptest server (or
// http.ListenAndServe) serves.
//
// It is the additive convenience wrapper over
//
//	mh := testbed.NewMockHermes(h)
//	handler := harness.NewHTTPServer(h)
//
// and exists to make the layering impossible to get wrong: the handler it hands
// back can never be passed to NewMockHermes, because NewMockHermes accepts only
// a harness. Both return values read the same harness, so a Decision asserted
// through mh and the same Decision decoded from a POST to the handler agree
// field for field — mh is exactly NewMockHermes(h), with no extra state and no
// behaviour of its own (see ExampleNewMockHermes and TestMockHermesHTTPRoundtrip).
//
//	h := &MyHarness{}
//	mh, handler := testbed.NewMockHermesWithServer(h)
//	ts := httptest.NewServer(handler)
//	defer ts.Close()
//
//	dec, err := mh.SendMessage("s1", "hello mock", "alice", "u1") // in process
//	// … and POST ts.URL+"/v1/process" for the HTTP path over the same harness.
func NewMockHermesWithServer(h harness.Harness) (*MockHermes, http.Handler) {
	return NewMockHermes(h), harness.NewHTTPServer(h)
}

// recoverErr converts a recovered panic into an error prefixed with "harness panic:".
func (m *MockHermes) recoverErr(r any) error {
	if r == nil {
		return nil
	}
	return fmt.Errorf("harness panic: %v", r)
}

// SendMessage simulates Hermes sending a user message to the harness.
// It constructs a ProcessRequest with an empty conversation history
// (DefaultContext) and calls h.OnProcess.
// Returns the Decision from OnProcess.
//
// SendMessage is equivalent to SendMessageWithHistory with no history, so a
// request built here has the same shape it has always had: History is an empty,
// non-nil slice. To exercise harness logic that depends on prior conversation
// turns, call SendMessageWithHistory (or build the context with
// ContextWithHistory) instead of driving OnProcess directly.
func (m *MockHermes) SendMessage(sessionID, content, userName, userID string) (*protocol.Decision, error) {
	return m.SendMessageWithHistory(sessionID, content, nil, userName, userID)
}

// SendMessageWithHistory simulates Hermes sending a user message to the harness
// together with the prior conversation turns Hermes recorded for the session.
// It is the history-injecting companion to SendMessage: the same ProcessRequest
// (role "user" message, "test" platform identity) is built, but Context.History
// carries a copy of history instead of being empty.
//
// Passing a nil or empty history is identical to calling SendMessage, so a
// caller can thread a possibly-empty slice through without special-casing.
// The entries are copied, and the returned Decision is the harness's own (it is
// not synthesized or rewritten here).
//
// Typical use — verify the never-shrinking-history contract through the testbed
// API rather than by constructing a ProcessRequest by hand:
//
//	hist := []protocol.HistoryEntry{
//		{Role: protocol.RoleUser, Content: "first turn"},
//		{Role: protocol.RoleAssistant, Content: "first reply"},
//	}
//	dec, err := testbed.NewMockHermes(h).SendMessageWithHistory(
//		"sess-1", "second turn", hist, "alice", "u-1")
func (m *MockHermes) SendMessageWithHistory(sessionID, content string, history []protocol.HistoryEntry, userName, userID string) (dec *protocol.Decision, err error) {
	m.decisionCount++
	m.SessionCount++

	req := &protocol.ProcessRequest{
		SessionID: sessionID,
		Message: protocol.Message{
			Role:      "user",
			Content:   content,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Identity: protocol.Identity{
			Platform: "test",
			ChatID:   "test-chat-" + sessionID,
			UserName: userName,
			UserID:   userID,
		},
		Context: ContextWithHistory(history),
	}

	defer func() {
		if p := recover(); p != nil {
			err = m.recoverErr(p)
			m.LastError = err
			dec = nil
		}
	}()

	dec, err = m.Harness.OnProcess(req)
	m.LastDecision = dec
	m.LastError = err
	if dec != nil {
		m.Decisions = append(m.Decisions, dec)
	}
	return dec, err
}

// SendResult simulates Hermes sending a tool result back.
// It constructs a ResultRequest and calls h.OnResult.
// Returns the Decision from OnResult.
func (m *MockHermes) SendResult(sessionID, decisionID string, result protocol.Result) (dec *protocol.Decision, err error) {
	req := &protocol.ResultRequest{
		SessionID:  sessionID,
		DecisionID: decisionID,
		Result:     result,
	}

	defer func() {
		if p := recover(); p != nil {
			err = m.recoverErr(p)
			m.LastError = err
			dec = nil
		}
	}()

	dec, err = m.Harness.OnResult(req)
	m.LastDecision = dec
	m.LastError = err
	if dec != nil {
		m.Decisions = append(m.Decisions, dec)
	}
	return dec, err
}

// SendCancel simulates Hermes sending a cancel request.
func (m *MockHermes) SendCancel(sessionID string, reason protocol.CancelReason) (err error) {
	req := &protocol.CancelRequest{
		SessionID: sessionID,
		Reason:    reason,
	}

	defer func() {
		if p := recover(); p != nil {
			err = m.recoverErr(p)
			m.LastError = err
		}
	}()

	err = m.Harness.OnCancel(req)
	m.LastError = err
	return err
}

// Health calls the harness Health() method.
func (m *MockHermes) Health() *protocol.HealthResponse {
	return m.Harness.Health()
}

// TerminateSession calls OnSessionTerminate.
func (m *MockHermes) TerminateSession(sessionID string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = m.recoverErr(p)
			m.LastError = err
		}
	}()

	err = m.Harness.OnSessionTerminate(sessionID)
	m.LastError = err
	return err
}

// DefaultTools returns a standard set of test tools.
func DefaultTools() []protocol.Tool {
	return []protocol.Tool{
		{
			Name:        "read_file",
			Description: "Read a file from the filesystem",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file",
					},
				},
				"required": []any{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Content to write",
					},
				},
				"required": []any{"path", "content"},
			},
		},
		{
			Name:        "terminal",
			Description: "Execute a shell command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Command to execute",
					},
				},
				"required": []any{"command"},
			},
		},
	}
}

// DefaultModels returns a standard set of test models.
func DefaultModels() []protocol.Model {
	return []protocol.Model{
		{
			Name:                "test-model",
			Provider:            "test-provider",
			ContextWindow:       128000,
			SupportsToolCalling: true,
		},
		{
			Name:                "test-vision-model",
			Provider:            "test-provider",
			ContextWindow:       200000,
			SupportsVision:      true,
			SupportsToolCalling: false,
		},
	}
}

// DefaultContext returns a fully populated Context for testing.
func DefaultContext() protocol.Context {
	return protocol.Context{
		History: []protocol.HistoryEntry{},
		Tools:   DefaultTools(),
		Models:  DefaultModels(),
		Config: protocol.Config{
			MaxIterations:  10,
			TimeoutSeconds: 30,
		},
		SessionState: protocol.SessionState{
			TurnCount:      0,
			TotalToolCalls: 0,
			TotalLLMCalls:  0,
			CostSoFar:      0,
			StartedAt:      time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// ContextWithHistory returns a fully populated Context for testing (see
// DefaultContext) whose History carries a copy of the given conversation turns.
// It is the context-building half of SendMessageWithHistory, and is useful
// directly when a test needs to hand a harness a request context of its own.
//
// The returned slice is always non-nil: a nil or empty history yields an empty
// History slice, so ContextWithHistory(nil) equals DefaultContext() exactly.
func ContextWithHistory(history []protocol.HistoryEntry) protocol.Context {
	ctx := DefaultContext()
	ctx.History = cloneHistory(history)
	return ctx
}

// cloneHistory copies history so the returned slice never aliases the caller's
// (mutating the caller's slice after a call must not change what the harness
// saw) and is never nil.
func cloneHistory(history []protocol.HistoryEntry) []protocol.HistoryEntry {
	out := make([]protocol.HistoryEntry, len(history))
	copy(out, history)
	return out
}

// QuickIdentity returns an Identity for quick test setup.
func QuickIdentity(userName, userID string) protocol.Identity {
	return protocol.Identity{
		Platform: "test",
		ChatID:   "test-chat",
		UserName: userName,
		UserID:   userID,
	}
}

// QuickMessage returns a Message for quick test setup.
func QuickMessage(content string) protocol.Message {
	return protocol.Message{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
