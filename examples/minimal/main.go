// Package main — minimal H3 harness example.
// Demonstrates the simplest possible harness: respond with a fixed message
// on every process request, end on every result.
package main

import (
	"net/http"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// MinimalHarness responds "Hello from H3 Go SDK!" to every message.
type MinimalHarness struct{}

// OnProcess returns a text decision with a greeting.
func (h *MinimalHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	return &protocol.Decision{
		Decision: protocol.DecisionText,
		Text:     &protocol.TextResp{Content: "Hello from H3 Go SDK!", Finished: true},
	}, nil
}

// OnResult ends the session after any result is received.
func (h *MinimalHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	return &protocol.Decision{
		Decision: protocol.DecisionEnd,
		End:      &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Echo complete"},
	}, nil
}

// OnCancel is a no-op.
func (h *MinimalHarness) OnCancel(req *protocol.CancelRequest) error {
	return nil
}

// OnSessionTerminate is a no-op.
func (h *MinimalHarness) OnSessionTerminate(sessionID string) error {
	return nil
}

// Health reports the harness is healthy.
func (h *MinimalHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
	}
}

func main() {
	h := harness.NewHTTPServer(&MinimalHarness{})
	http.ListenAndServe(":9191", h)
}
