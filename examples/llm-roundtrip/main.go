// Package main — llm-roundtrip: a deliberator harness that asks Hermes for LLM
// completions (`llm_call`) and drives the answer-collection round trip.
//
// Every other shipped example covers text or tool_call loops. This one covers
// the decision type a harness uses when it wants *Hermes' model* to do the
// thinking — a workflow where the consumer has to invent the whole state
// machine (which model runs when, how answers are collected, when the loop is
// allowed to end). It is the runnable reference for that shape:
//
//	POST /v1/process            -> llm_call  (model A: draft an answer)
//	POST /v1/result llm_response-> llm_call  (model B: critique the draft)
//	POST /v1/result llm_response-> text      (synthesis, carries "VERDICT:", finished=true)
//	POST /v1/result text_sent   -> end       (reason task_complete)
//
// Four contracts this example demonstrates, all of them things consumers get
// wrong when they invent the loop themselves:
//
//  1. Models come from the request. `context.models` is what Hermes offers for
//     THIS session — `models: []` means "no model is available", so the harness
//     must NOT return `llm_call` with a model it made up. It falls back to a
//     plain `text` decision instead (the h3-test battery calls a hardcoded
//     model a hallucinated model).
//  2. History never shrinks. The session history is seeded once from
//     `context.history`, every user turn is appended to it, and a snapshot rides
//     on EVERY decision — including the result-driven ones, which is where a
//     naive implementation drops it.
//  3. Streaming is a request. A message containing "do not finish" asks for
//     unfinished text (`finished: false`); the NEXT result flips it to
//     `finished: true` rather than ending the session.
//  4. Per-session state is mutex-guarded. Two requests for the same session can
//     land concurrently; every read and write of the session map happens under
//     one lock.
//
// The harness is deliberately transport-only: no LLM is called here. Hermes
// executes the `llm_call` and hands the completion back through `/v1/result`,
// which is exactly what `fake_hermes.py` scripts.
//
// Run with:
//
//	go run ./examples/llm-roundtrip/            # serves on :9191
//	PORT=9291 go run ./examples/llm-roundtrip/  # when another harness holds :9191
//
// Then drive the full round trip with the scripted fake-Hermes client, which
// posts the results above and asserts the verdict, the completed session and
// the DELETE -> 404 teardown:
//
//	python3 examples/llm-roundtrip/fake_hermes.py          # default :9191
//	python3 examples/llm-roundtrip/fake_hermes.py 9291     # or PORT=9291 python3 …
//
// The same listener passes the full compliance battery:
//
//	PORT=9291 go run ./examples/llm-roundtrip/ &
//	h3-test --endpoint http://127.0.0.1:9291
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// step is where a session sits in the deliberation round trip. It is advanced
// only by OnResult, i.e. by Hermes handing back the answer it gathered.
type step int

const (
	// stepIdle — a fresh session that has not been asked anything yet.
	stepIdle step = iota
	// stepAwaitDraft — the process call returned llm_call; the next result
	// carries model A's draft.
	stepAwaitDraft
	// stepAwaitCritique — the draft came back; the next result carries model
	// B's critique.
	stepAwaitCritique
	// stepAwaitVerdictResult — the synthesis text was delivered; the next
	// result closes the session.
	stepAwaitVerdictResult
	// stepAwaitStreamResult — unfinished text was delivered ("do not finish");
	// the next result flips it to finished.
	stepAwaitStreamResult
	// stepAwaitTextResult — the plain-text fallback was delivered (no models
	// were offered); the next result closes the session.
	stepAwaitTextResult
	// stepDone — the session is over; a further result is a no-op end.
	stepDone
)

// sessionState is the per-session state of one deliberation. Every field is
// read and written under Deliberator.mu.
type sessionState struct {
	step      step
	task      string
	models    []string
	history   []protocol.HistoryEntry
	answers   []string
	streaming bool
}

// Deliberator implements harness.Harness: it turns each user turn into an
// `llm_call` (when Hermes offers models), then folds Hermes' completions into a
// verdict and ends the session.
//
// A mutex-guarded map holds the per-session state. Concurrent requests for the
// SAME session are a real case (a retrying client, or two clients sharing a
// chat id), so nothing here may read or write session state without the lock:
// the map read, the slice append and the step transition all happen inside one
// critical section.
type Deliberator struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
}

// NewDeliberator returns a ready harness. Use the constructor: a zero-value
// Deliberator has a nil map and panics on the first request ("assignment to
// entry in nil map"), which the middleware would report as a 500.
func NewDeliberator() *Deliberator {
	return &Deliberator{sessions: make(map[string]*sessionState)}
}

// session returns the state for sessionID, creating it on first use. It does NOT
// lock — every caller already holds h.mu, which is what keeps the lazy create
// race-free.
func (h *Deliberator) session(sessionID string) *sessionState {
	s, ok := h.sessions[sessionID]
	if !ok {
		s = &sessionState{step: stepIdle}
		h.sessions[sessionID] = s
	}
	return s
}

// snapshot copies a session's history. The decision that goes on the wire must
// own its slice: the SDK encodes it while the next request may already be
// appending to the live one.
func (s *sessionState) snapshot() []protocol.HistoryEntry {
	out := make([]protocol.HistoryEntry, len(s.history))
	copy(out, s.history)
	return out
}

// modelAt picks the model for the given round, falling back to the first model
// when Hermes offered only one. Both models are always taken from the request:
// this harness never names a model of its own.
func (s *sessionState) modelAt(i int) string {
	if len(s.models) == 0 {
		return ""
	}
	if i < len(s.models) {
		return s.models[i]
	}
	return s.models[0]
}

// OnProcess turns a user turn into the first decision of a deliberation.
//
// Precedence is deliberate:
//
//	streaming request ("do not finish") > model deliberation > plain text.
//
// The streaming clause comes first because it is what the caller asked for; the
// model clause is skipped entirely when `context.models` is empty, because
// returning `llm_call` without an offered model is a hallucinated model.
func (h *Deliberator) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := h.session(req.SessionID)

	// Seed the session history once, from the context Hermes sent, then append
	// every user turn. From here on the session's own copy is the truth and it
	// only ever grows.
	if s.history == nil {
		s.history = make([]protocol.HistoryEntry, 0, len(req.Context.History)+1)
		s.history = append(s.history, req.Context.History...)
	}
	s.history = append(s.history, protocol.HistoryEntry{
		Role:    protocol.RoleUser,
		Content: req.Message.Content,
	})
	history := s.snapshot()

	// A new user turn always restarts the loop, even on a session that has
	// already ended once.
	s.done()

	if isStreamingRequest(req.Message.Content) {
		s.step = stepAwaitStreamResult
		s.streaming = true
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: "dec-stream-start",
			History:    history,
			Text: &protocol.TextResp{
				Content:  "Starting a thought — more to come.",
				Finished: false,
			},
		}, nil
	}

	s.streaming = false
	s.task = req.Message.Content

	models := modelNames(req.Context.Models)
	if len(models) == 0 {
		// No model was offered for this session: answer with plain text. This
		// is the branch that keeps the harness honest — `context.models` is a
		// capability list, not a suggestion.
		s.step = stepAwaitTextResult
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: "dec-text-fallback",
			History:    history,
			Text: &protocol.TextResp{
				Content:  fmt.Sprintf("No model was offered for this session (context.models is empty), so I cannot ask Hermes for a completion. Echoing your request back: %s", req.Message.Content),
				Finished: true,
			},
		}, nil
	}

	// The first half of the round trip: ask Hermes to run the drafting model.
	s.models = models
	s.step = stepAwaitDraft
	s.answers = s.answers[:0]

	return &protocol.Decision{
		Decision:   protocol.DecisionLLMCall,
		DecisionID: "dec-llm-draft",
		History:    history,
		LLMCall: &protocol.LLMCall{
			Model:        s.modelAt(0),
			SystemPrompt: "You are one voice in a two-model deliberation. Answer the user's request directly and completely.",
			// The conversation so far rides along, which is why the history
			// snapshot matters on llm_call decisions too.
			Messages: historyMessages(history),
		},
	}, nil
}

// OnResult advances the round trip with whatever Hermes came back with. This is
// where a consumer's state machine either collects answers properly or loses
// them; each branch below is one hop of the loop, and every returned decision
// carries the (possibly grown) history snapshot.
func (h *Deliberator) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := h.session(req.SessionID)
	history := s.snapshot()

	// A failed execution ends the deliberation with the error reason instead of
	// walking the loop into a dead end. (The battery posts a failing result and
	// requires no 5xx: an error is a legitimate outcome, not a crash.)
	if req.Result.Success == nil || !*req.Result.Success || req.Result.Type == protocol.ResultError {
		s.step = stepDone
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "dec-end-error",
			History:    history,
			End: &protocol.End{
				Reason:  protocol.EndError,
				Summary: "the LLM call did not produce a usable result",
			},
		}, nil
	}

	if answer := resultText(req.Result.Data); answer != "" {
		s.answers = append(s.answers, answer)
	}

	switch s.step {
	case stepAwaitDraft:
		// Model A's draft is in hand: ask Hermes to run the critique model on
		// top of it. This second llm_call is the part of the round trip with no
		// shipped reference before this example.
		s.step = stepAwaitCritique
		return &protocol.Decision{
			Decision:   protocol.DecisionLLMCall,
			DecisionID: "dec-llm-critique",
			History:    history,
			LLMCall: &protocol.LLMCall{
				Model:        s.modelAt(1),
				SystemPrompt: "You are the second voice in a two-model deliberation. Critique the previous answer: name what is right, what is missing, and what you would change.",
				Messages:     historyMessages(history),
			},
		}, nil

	case stepAwaitCritique:
		// Both completions are in: deliver the synthesis as finished text. The
		// verdict is what the consumer's client asserts on.
		s.step = stepAwaitVerdictResult
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: "dec-text-verdict",
			History:    history,
			Text: &protocol.TextResp{
				Content:  s.verdict(),
				Finished: true,
			},
		}, nil

	case stepAwaitStreamResult:
		// The caller asked for an unfinished message; the next result finishes
		// it. Flipping to finished=true here (rather than ending) is the
		// streaming contract.
		s.step = stepAwaitTextResult
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: "dec-stream-finish",
			History:    history,
			Text: &protocol.TextResp{
				Content:  "…and that is the whole thought.",
				Finished: true,
			},
		}, nil

	default:
		// stepAwaitVerdictResult / stepAwaitTextResult (and anything already
		// done): the delivered message was accepted, so close the session.
		s.step = stepDone
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "dec-end-complete",
			History:    history,
			End: &protocol.End{
				Reason:  protocol.EndTaskComplete,
				Summary: "deliberation delivered",
			},
		}, nil
	}
}

// OnCancel marks the session cancelled and drops its answers.
func (h *Deliberator) OnCancel(req *protocol.CancelRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[req.SessionID]; ok {
		s.step = stepDone
		s.answers = nil
	}
	return nil
}

// OnSessionTerminate releases the session's state on DELETE /v1/sessions/{id}.
func (h *Deliberator) OnSessionTerminate(sessionID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, sessionID)
	return nil
}

// Health advertises the decision types this harness can actually return.
func (h *Deliberator) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities: []protocol.DecisionType{
			protocol.DecisionLLMCall,
			protocol.DecisionText,
			protocol.DecisionEnd,
		},
	}
}

// done resets a session to a fresh round. Called with h.mu held.
func (s *sessionState) done() {
	s.step = stepIdle
	s.answers = nil
}

// verdict builds the synthesis text. The "VERDICT:" marker is the contract the
// scripted client asserts on, so a caller can tell the deliberation's output
// apart from ordinary text.
func (s *sessionState) verdict() string {
	task := s.task
	if task == "" {
		task = "(no task recorded)"
	}
	if len(s.answers) == 0 {
		return fmt.Sprintf("VERDICT: no completion came back for %q — delivering the task unmodified.", task)
	}
	list := make([]string, 0, len(s.answers))
	for i, a := range s.answers {
		list = append(list, fmt.Sprintf("[round %d] %s", i+1, a))
	}
	return fmt.Sprintf("VERDICT: synthesised %d round(s) of %s for %q\n%s",
		len(s.answers), strings.Join(s.models, " + "), task, strings.Join(list, "\n"))
}

// isStreamingRequest reports whether the user asked for an unfinished message.
// This is the documented trigger ("do not finish") the battery and the other
// examples use.
func isStreamingRequest(content string) bool {
	return strings.Contains(strings.ToLower(content), "do not finish")
}

// modelNames extracts the offered model names, dropping blank entries. Hermes
// decides which models exist for a session; the harness only picks among them.
func modelNames(models []protocol.Model) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		if name := strings.TrimSpace(m.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// historyMessages converts the session history into llm_call messages, so the
// model sees the conversation rather than only the last turn.
func historyMessages(history []protocol.HistoryEntry) []protocol.LLMMessage {
	out := make([]protocol.LLMMessage, 0, len(history))
	for _, entry := range history {
		role := string(entry.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, protocol.LLMMessage{Role: role, Content: entry.Content})
	}
	if len(out) == 0 {
		out = append(out, protocol.LLMMessage{Role: "user", Content: "(empty conversation)"})
	}
	return out
}

// resultText pulls the completion text out of a result payload. The wire shape
// of `result.data` is producer-defined JSON, so the well-known keys are tried
// first and an unknown object is passed through as JSON rather than dropped.
func resultText(data any) string {
	switch v := data.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"content", "text", "output", "answer"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if len(v) > 0 {
			if b, err := json.Marshal(v); err == nil {
				return string(b)
			}
		}
	}
	return ""
}

func main() {
	addr := harness.ListenAddr()
	srv := harness.NewHTTPServer(NewDeliberator())
	log.Printf("h3 llm-roundtrip harness listening on %s (set %s to override)", addr, harness.PortEnv)
	// Serve never returns: it reports a bind collision with the shared hint
	// naming the PORT override, and every other listen error is fatal.
	harness.Serve(addr, srv)
}

var _ harness.Harness = (*Deliberator)(nil)
