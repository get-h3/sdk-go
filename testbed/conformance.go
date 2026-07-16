package testbed

import (
	"strings"
	"sync"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// sessionState tracks per-session state for the conformance harness.
type sessionState struct {
	resultCount int
	status      string
	history     []protocol.HistoryEntry
}

// ConformanceHarness implements harness.Harness with decision logic for the
// h3-test conformance battery. It supports the full agent loop:
// tool_call → result → text → end.
type ConformanceHarness struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
}

// NewConformanceHarness creates a harness that implements the S04 §6
// conformance behaviour.
func NewConformanceHarness() harness.Harness {
	return &ConformanceHarness{
		sessions: make(map[string]*sessionState),
	}
}

func (h *ConformanceHarness) getSession(sessionID string) *sessionState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.sessions[sessionID]; ok {
		return s
	}
	s := &sessionState{status: "active"}
	h.sessions[sessionID] = s
	return s
}

func (h *ConformanceHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	content := strings.ToLower(req.Message.Content)
	s := h.getSession(req.SessionID)

	h.mu.Lock()
	s.history = append(s.history, protocol.HistoryEntry{
		Role:    protocol.RoleUser,
		Content: req.Message.Content,
	})
	h.mu.Unlock()

	id := "dec-process"

	if strings.Contains(content, "start a thought, do not finish") {
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: id,
			Text: &protocol.TextResp{
				Content:  "Starting a thought...",
				Finished: false,
			},
		}, nil
	}

	if strings.Contains(content, "final answer") || strings.Contains(content, "finished") {
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: id,
			Text: &protocol.TextResp{
				Content:  "The answer is 42.",
				Finished: true,
			},
		}, nil
	}

	if len(req.Context.Tools) > 0 && containsToolKeyword(content) {
		tool := req.Context.Tools[0]
		return &protocol.Decision{
			Decision:   protocol.DecisionToolCall,
			DecisionID: id,
			ToolCall: &protocol.ToolCall{
				Name:      tool.Name,
				Params:    map[string]any{"input": req.Message.Content},
				Reasoning: "tool requested by user",
			},
		}, nil
	}

	if len(req.Context.Models) > 0 && containsModelKeyword(content) {
		model := req.Context.Models[0]
		return &protocol.Decision{
			Decision:   protocol.DecisionLLMCall,
			DecisionID: id,
			LLMCall: &protocol.LLMCall{
				Model: model.Name,
				Messages: []protocol.LLMMessage{
					{Role: "user", Content: req.Message.Content},
				},
			},
		}, nil
	}

	if strings.Contains(content, "delegate") ||
		strings.Contains(content, "sub-agent") ||
		strings.Contains(content, "summarise") ||
		strings.Contains(content, "spawn") {
		return &protocol.Decision{
			Decision:   protocol.DecisionDelegate,
			DecisionID: id,
			Delegate: &protocol.Delegate{
				Task: "delegated task: " + req.Message.Content,
			},
		}, nil
	}

	if strings.Contains(content, "done") || strings.Contains(content, "end") {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: id,
			End: &protocol.End{
				Reason:  protocol.EndTaskComplete,
				Summary: "task complete",
			},
		}, nil
	}

	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: id,
		Text: &protocol.TextResp{
			Content:  "Echo: " + req.Message.Content,
			Finished: true,
		},
	}, nil
}

func containsToolKeyword(content string) bool {
	return strings.Contains(content, "echo") ||
		strings.Contains(content, "search") ||
		strings.Contains(content, "lookup") ||
		strings.Contains(content, "noop") ||
		strings.Contains(content, "use")
}

func containsModelKeyword(content string) bool {
	return strings.Contains(content, "model") || strings.Contains(content, "run")
}

func (h *ConformanceHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	s := h.getSession(req.SessionID)

	h.mu.Lock()
	defer h.mu.Unlock()

	s.resultCount++
	if s.resultCount >= 3 {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "dec-end-forced",
			End: &protocol.End{
				Reason:  protocol.EndTaskComplete,
				Summary: "forced end after 3 results",
			},
		}, nil
	}

	if !req.Result.Success || req.Result.Type == protocol.ResultError {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: "dec-end-error",
			End: &protocol.End{
				Reason:  protocol.EndError,
				Summary: "result indicated error",
			},
		}, nil
	}

	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: "dec-result",
		Text: &protocol.TextResp{
			Content:  "Result received: " + string(req.Result.Type),
			Finished: true,
		},
	}, nil
}

func (h *ConformanceHarness) OnCancel(req *protocol.CancelRequest) error {
	s := h.getSession(req.SessionID)
	h.mu.Lock()
	defer h.mu.Unlock()
	s.status = "cancelled"
	return nil
}

func (h *ConformanceHarness) OnSessionTerminate(sessionID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, sessionID)
	return nil
}

func (h *ConformanceHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities: []protocol.DecisionType{
			protocol.DecisionToolCall,
			protocol.DecisionLLMCall,
			protocol.DecisionText,
			protocol.DecisionWait,
			protocol.DecisionDelegate,
			protocol.DecisionEnd,
		},
	}
}

var _ harness.Harness = (*ConformanceHarness)(nil)
