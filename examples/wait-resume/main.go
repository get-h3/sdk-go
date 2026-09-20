// Package main — wait/resume H3 harness example (GAP-042).
// Demonstrates the documented async pattern for work that may exceed the
// harness's fixed 30s middleware timeout: OnProcess returns a `wait`
// decision (with poll_endpoint) while a background goroutine does the
// work; when Hermes posts the wait_timeout result, OnResult hands back
// the completed output as finished text, and the next roundtrip ends
// the task.
//
// Run with:
//
//	go run ./examples/wait-resume/
//
// Every example honors the PORT environment variable (default 9191), so a second
// harness can run next to one that already holds the default port:
//
//	PORT=9293 go run ./examples/wait-resume/
//
// The simulated "long" work here is only ~3s so the demo completes fast;
// in production this is where a >30s job (LLM batch, tool chain, deploy)
// would run. See README.md for the full consumer curl flow.
package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// workDuration is the simulated long-running job length. It matches the
// wait decision's duration_seconds so the consumer's wait_timeout result
// arrives just after the work completes. Keep it short for the demo.
const workDuration = 3 * time.Second

// jobState tracks one background job, keyed by the wait decision ID.
// All access goes through WaitResumeHarness.mu — the worker goroutine and
// the HTTP handlers never touch the map or the job fields without it,
// so the outcome may land before or after the consumer posts /v1/result.
type jobState struct {
	done   bool
	output string
}

// WaitResumeHarness implements harness.Harness with an async wait/resume flow.
type WaitResumeHarness struct {
	mu sync.Mutex
	// jobs holds background work keyed by wait decision ID.
	jobs map[string]*jobState
	// textDecisions records decision IDs of text decisions this harness
	// returned, so OnResult can tell "the wait timed out" from "the text
	// was delivered" and end the task on the latter.
	textDecisions map[string]bool
	// counter mints unique decision IDs across sessions.
	counter int
}

// nextID mints a unique decision ID. Callers must hold h.mu.
func (h *WaitResumeHarness) nextID(prefix string) string {
	h.counter++
	return fmt.Sprintf("%s-%03d", prefix, h.counter)
}

// OnProcess starts the simulated long work in a background goroutine and
// immediately returns a wait decision pointing Hermes at the session's
// poll endpoint. The handler does NOT block on the work — blocking here is
// what would trip the 30s middleware timeout on a real job.
func (h *WaitResumeHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	decisionID := h.nextID("wait")
	job := &jobState{}
	h.jobs[decisionID] = job
	h.mu.Unlock()

	// Background worker: simulates the >30s-class job (batch LLM call,
	// tool chain, deploy...) then records the outcome under the mutex.
	// It is safe whether the consumer polls before or after this lands.
	go func(id string, j *jobState) {
		time.Sleep(workDuration)
		output := fmt.Sprintf("background job %s completed: computed result-1729", id)
		h.mu.Lock()
		j.done = true
		j.output = output
		h.mu.Unlock()
	}(decisionID, job)

	return &protocol.Decision{
		Decision:   protocol.DecisionWait,
		DecisionID: decisionID,
		Wait: &protocol.Wait{
			Reason:          "long-running work started in background",
			DurationSeconds: intPtr(int(workDuration.Seconds())),
			PollEndpoint:    "/v1/sessions/" + req.SessionID,
		},
	}, nil
}

// OnResult drives the resume half of the pattern:
//   - wait_timeout for the wait decision → return the completed work as
//     finished text (Hermes delivers it, then posts a text_sent result).
//   - that text decision's delivery result → end the task.
//   - anything before the work finished → re-wait briefly instead of
//     blocking (the goroutine outcome lands under the same mutex).
func (h *WaitResumeHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	job, known := h.jobs[req.DecisionID]
	_, wasText := h.textDecisions[req.DecisionID]

	if wasText {
		// The finished text was delivered — task complete.
		h.mu.Unlock()
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: h.nextID("end"),
			End: &protocol.End{
				Reason:  protocol.EndTaskComplete,
				Summary: "wait/resume task complete: background result delivered",
			},
		}, nil
	}

	if known && job.done {
		// Work finished (before or after the timeout result — either is
		// fine): hand the output back as finished text.
		textID := h.nextID("text")
		h.textDecisions[textID] = true
		output := job.output
		h.mu.Unlock()
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: textID,
			Text: &protocol.TextResp{
				Content:  output,
				Finished: true,
			},
		}, nil
	}
	h.mu.Unlock()

	// Result arrived before the background work finished: ask Hermes to
	// wait a little longer rather than blocking this handler.
	return &protocol.Decision{
		Decision:   protocol.DecisionWait,
		DecisionID: req.DecisionID,
		Wait: &protocol.Wait{
			Reason:          "background work still running",
			DurationSeconds: intPtr(1),
			PollEndpoint:    "/v1/sessions/" + req.SessionID,
		},
	}, nil
}

// OnCancel is a no-op; the background goroutine's output is simply discarded.
func (h *WaitResumeHarness) OnCancel(req *protocol.CancelRequest) error {
	return nil
}

// OnSessionTerminate is a no-op.
func (h *WaitResumeHarness) OnSessionTerminate(sessionID string) error {
	return nil
}

// Health reports the harness healthy with the wait/resume capability set.
func (h *WaitResumeHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities:    []protocol.DecisionType{protocol.DecisionWait, protocol.DecisionText, protocol.DecisionEnd},
	}
}

func intPtr(n int) *int { return &n }

func main() {
	addr := harness.ListenAddr()
	h := harness.NewHTTPServer(&WaitResumeHarness{
		jobs:          map[string]*jobState{},
		textDecisions: map[string]bool{},
	})
	log.Printf("h3 wait-resume harness listening on %s (set %s to override)", addr, harness.PortEnv)
	// Serve never returns: it reports a bind collision with the shared hint
	// naming the PORT override, and every other listen error is fatal.
	harness.Serve(addr, h)
}
