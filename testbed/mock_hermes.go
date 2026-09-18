// Package testbed provides MockHermes and assertion helpers
// for unit-testing H3 harness logic.
package testbed

import (
	"fmt"
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
func NewMockHermes(h harness.Harness) *MockHermes {
	return &MockHermes{
		Harness: h,
	}
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
