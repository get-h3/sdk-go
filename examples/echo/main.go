// Package main — echo H3 harness example.
// Demonstrates a harness that echoes back the user's message content
// and reports the received decision ID on each result.
package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// EchoHarness echoes the user message back and tracks results.
type EchoHarness struct {
	responseCount int
	streaming     bool // true when the current session is streaming text
}

// OnProcess echoes the user's message content.
func (h *EchoHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	content := fmt.Sprintf("Echo: %s", req.Message.Content)

	// Track streaming mode: messages containing "do not finish" trigger unfinished text
	h.streaming = strings.Contains(req.Message.Content, "do not finish")
	finished := !h.streaming

	// Echo conversation history from context
	history := make([]protocol.HistoryEntry, len(req.Context.History))
	for i, entry := range req.Context.History {
		history[i] = protocol.HistoryEntry{Role: entry.Role, Content: entry.Content}
	}

	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "echo-001",
		Text:       &protocol.TextResp{Content: content, Finished: finished},
		History:    history,
	}, nil
}

// OnResult reports the received decision ID, then ends the session.
func (h *EchoHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.responseCount++
	// End after 2 results for normal mode, stay in stream for streaming
	if !h.streaming && h.responseCount >= 2 {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "echo-end",
			End:        &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Echo conversation complete"},
		}, nil
	}
	content := fmt.Sprintf("Result received: %s", req.DecisionID)
	finished := !h.streaming
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "echo-002",
		Text:       &protocol.TextResp{Content: content, Finished: finished},
	}, nil
}

// OnCancel is a no-op.
func (h *EchoHarness) OnCancel(req *protocol.CancelRequest) error {
	return nil
}

// OnSessionTerminate is a no-op.
func (h *EchoHarness) OnSessionTerminate(sessionID string) error {
	return nil
}

// Health reports the harness is healthy.
func (h *EchoHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities:    []protocol.DecisionType{protocol.DecisionText},
	}
}

func main() {
	h := harness.NewHTTPServer(&EchoHarness{})
	log.Fatal(http.ListenAndServe(":9191", h))
}
