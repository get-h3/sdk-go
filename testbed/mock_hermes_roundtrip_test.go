package testbed

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// This file is the executable proof of the contract documented on
// NewMockHermes and in the package doc (DF-H3-SDK-GO-FOREMAN-9): the argument is
// a harness.Harness — NOT the http.Handler from harness.NewHTTPServer — and the
// Send* methods answer with a *protocol.Decision, NOT a bool. The roundtrip
// asserted here is the one the docs describe: mock -> NewHTTPServer -> request
// -> assert the Decision fields.

// processBodyFor returns a minimal valid POST /v1/process body for session id
// sid. It carries only the fields the protocol guarantees, and its message
// content is "hello mock" so echoHarness answers "Echo: hello mock".
func processBodyFor(sid string) string {
	return `{
		"session_id": "` + sid + `",
		"message": {"role": "user", "content": "hello mock", "timestamp": "2026-09-22T00:00:00Z"},
		"identity": {"platform": "test", "chat_id": "c1", "user_name": "alice", "user_id": "u1"},
		"context": {
			"history": [], "tools": [], "models": [],
			"config": {"max_iterations": 10, "timeout_seconds": 30},
			"session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "2026-09-22T00:00:00Z"}
		}
	}`
}

// postProcessTo posts one /v1/process request to ts and returns the decoded
// Decision — the HTTP half of the documented pattern.
func postProcessTo(t *testing.T, ts *httptest.Server, sid string) protocol.Decision {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/process", "application/json",
		strings.NewReader(processBodyFor(sid)))
	if err != nil {
		t.Fatalf("POST /v1/process: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/process: expected 200, got %d (%s)", resp.StatusCode, raw)
	}
	var dec protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	return dec
}

// TestMockHermesHTTPRoundtrip is the executable form of NewMockHermes's godoc:
// a harness.Harness (the echoHarness fixture) goes in — the http.Handler from
// harness.NewHTTPServer does not, and does not even compile — and both the mock
// and the HTTP path answer with a *protocol.Decision whose fields are asserted
// directly: decision=text, text="Echo: hello mock", finished=true.
//
// The layer claim is asserted, not narrated: harness.NewHTTPServer's handler is
// checked NOT to satisfy harness.Harness. That is exactly why
// `NewMockHermes(handler)` fails with "missing method Health", and the day the
// two types converge this test says so instead of the docs going quietly wrong.
func TestMockHermesHTTPRoundtrip(t *testing.T) {
	h := &echoHarness{}
	handler := harness.NewHTTPServer(h)
	if _, ok := any(handler).(harness.Harness); ok {
		t.Fatal("harness.NewHTTPServer's http.Handler now implements harness.Harness — NewMockHermes's documented argument contract (harness in, handler wrapped around it, never swapped) no longer holds")
	}
	var _ harness.Harness = h // the argument NewMockHermes wants: a harness, not a handler

	// The documented pattern: mock over the harness, server over the same
	// harness, one request, assert the Decision.
	mh := NewMockHermes(h)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	overHTTP := postProcessTo(t, ts, "sess-roundtrip")
	AssertDecisionType(t, &overHTTP, protocol.DecisionText)
	AssertTextContent(t, &overHTTP, "Echo: hello mock", true)
	AssertDecisionValid(t, &overHTTP)

	// In process, through the mock: same harness, same fields. SendMessage
	// returns (*protocol.Decision, error) — the Decision here is a pointer, and
	// `finished` is a field of it (dec.Text.Finished), never a returned bool.
	inProc, err := mh.SendMessage("sess-roundtrip", "hello mock", "alice", "u1")
	AssertNoError(t, err)
	AssertDecisionType(t, inProc, protocol.DecisionText)
	AssertTextContent(t, inProc, "Echo: hello mock", true)

	if inProc.Decision != overHTTP.Decision || inProc.DecisionID != overHTTP.DecisionID {
		t.Errorf("layers disagree on the decision: in-process %q/%q vs over HTTP %q/%q",
			inProc.Decision, inProc.DecisionID, overHTTP.Decision, overHTTP.DecisionID)
	}
	if inProc.Text == nil || overHTTP.Text == nil ||
		inProc.Text.Content != overHTTP.Text.Content ||
		inProc.Text.Finished != overHTTP.Text.Finished {
		t.Errorf("layers disagree on the text payload: in-process %+v vs over HTTP %+v", inProc.Text, overHTTP.Text)
	}
	if mh.SessionCount != 1 || len(mh.Decisions) != 1 || mh.Decisions[0] != inProc {
		t.Errorf("mock tracking lost the decision: SessionCount=%d Decisions=%d", mh.SessionCount, len(mh.Decisions))
	}

	t.Run("NewMockHermesWithServer keeps the layer order", func(t *testing.T) {
		h := &echoHarness{}
		mh, handler := NewMockHermesWithServer(h)
		ts := httptest.NewServer(handler)
		defer ts.Close()

		viaMock, err := mh.SendMessage("sess-wrapper", "hello mock", "alice", "u1")
		AssertNoError(t, err)
		AssertDecisionType(t, viaMock, protocol.DecisionText)
		AssertTextContent(t, viaMock, "Echo: hello mock", true)

		viaHTTP := postProcessTo(t, ts, "sess-wrapper")
		AssertDecisionType(t, &viaHTTP, protocol.DecisionText)
		AssertTextContent(t, &viaHTTP, "Echo: hello mock", true)

		if viaMock.Text == nil || viaHTTP.Text == nil || viaMock.Text.Content != viaHTTP.Text.Content ||
			viaMock.Text.Finished != viaHTTP.Text.Finished {
			t.Errorf("wrapper's two layers disagree: mock %+v vs HTTP %+v", viaMock.Text, viaHTTP.Text)
		}
	})
}

// ExampleNewMockHermes is the runnable form of the pattern the godoc on
// NewMockHermes shows: a harness.Harness goes in, the HTTP layer wraps the same
// harness, and both paths hand back a *protocol.Decision to assert on — never a
// bool, never an http.Handler as input.
func ExampleNewMockHermes() {
	h := &echoHarness{}
	mh := NewMockHermes(h) // harness.Harness in, *MockHermes out

	// In process: the mock drives the harness directly.
	dec, err := mh.SendMessage("s1", "hello mock", "alice", "u1")
	if err != nil {
		panic(err)
	}
	fmt.Printf("in-process:  decision=%s finished=%t text=%q\n",
		dec.Decision, dec.Text.Finished, dec.Text.Content)

	// Over HTTP: the HARNESS goes into the server — never the handler into a mock.
	ts := httptest.NewServer(harness.NewHTTPServer(h))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/process", "application/json",
		strings.NewReader(processBodyFor("s1")))
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var overHTTP protocol.Decision
	if err := json.NewDecoder(resp.Body).Decode(&overHTTP); err != nil {
		panic(err)
	}
	fmt.Printf("over HTTP:   decision=%s finished=%t text=%q\n",
		overHTTP.Decision, overHTTP.Text.Finished, overHTTP.Text.Content)

	// Output:
	// in-process:  decision=text finished=true text="Echo: hello mock"
	// over HTTP:   decision=text finished=true text="Echo: hello mock"
}
