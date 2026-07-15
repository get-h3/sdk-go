// Package testbed provides MockHermes and assertion helpers
// for unit-testing H3 harness logic.
package testbed

import (
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

// SendMessage simulates Hermes sending a user message to the harness.
// It constructs a ProcessRequest and calls h.OnProcess.
// Returns the Decision from OnProcess.
func (m *MockHermes) SendMessage(sessionID, content, userName, userID string) (*protocol.Decision, error) {
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
		Context: DefaultContext(),
	}

	dec, err := m.Harness.OnProcess(req)
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
func (m *MockHermes) SendResult(sessionID, decisionID string, result protocol.Result) (*protocol.Decision, error) {
	req := &protocol.ResultRequest{
		SessionID:  sessionID,
		DecisionID: decisionID,
		Result:     result,
	}

	dec, err := m.Harness.OnResult(req)
	m.LastDecision = dec
	m.LastError = err
	if dec != nil {
		m.Decisions = append(m.Decisions, dec)
	}
	return dec, err
}

// SendCancel simulates Hermes sending a cancel request.
func (m *MockHermes) SendCancel(sessionID string, reason protocol.CancelReason) error {
	req := &protocol.CancelRequest{
		SessionID: sessionID,
		Reason:    reason,
	}

	err := m.Harness.OnCancel(req)
	m.LastError = err
	return err
}

// Health calls the harness Health() method.
func (m *MockHermes) Health() *protocol.HealthResponse {
	return m.Harness.Health()
}

// TerminateSession calls OnSessionTerminate.
func (m *MockHermes) TerminateSession(sessionID string) error {
	err := m.Harness.OnSessionTerminate(sessionID)
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
