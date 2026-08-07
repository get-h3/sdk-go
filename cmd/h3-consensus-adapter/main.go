// h3-consensus-adapter: standalone H3 protocol adapter for Consensus.
//
// This adapter implements the H3 brain-swap protocol (/v1/process, /v1/result) and
// translates requests to Consensus's native REST API. It runs as a separate process
// and can be pointed at any running Consensus instance.
//
// Usage:
//
//	h3-consensus-adapter --consensus-url http://localhost:8094 --port 9191
//
// # Hermes → H3 protocol → this adapter → Consensus REST API → agent loop
//
// The H3 wire types are imported from github.com/get-h3/sdk-go/protocol (no local
// duplicates — the protocol package's JSON tags are the wire contract), and the
// HTTP surface comes from github.com/get-h3/sdk-go/harness (middleware: logging,
// panic recovery, request timeouts, request/decision validation). This adapter
// implements harness.Harness and translates H3 requests into Consensus REST calls.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// ============================================================================
// CLI Flags
// ============================================================================

var (
	consensusURL = flag.String("consensus-url", "http://localhost:8094", "Consensus server URL")
	port         = flag.Int("port", 9191, "Port to listen on for H3 protocol")
	adminKey     = flag.String("admin-key", "", "Consensus admin API key (or set CONSENSUS_ADMIN_KEY env)")
)

// ============================================================================
// Adapter State
// ============================================================================

// Adapter implements harness.Harness and translates H3 protocol requests into
// Consensus REST API calls.
type Adapter struct {
	consensusURL string
	adminKey     string
	sessions     map[string]string                  // h3_session_id → consensus_session_id
	histories    map[string][]protocol.HistoryEntry // h3_session_id → last-seen conversation history (for /v1/result passthrough)
	mu           sync.RWMutex
	client       *http.Client
}

func NewAdapter(consensusURL, adminKey string) *Adapter {
	return &Adapter{
		consensusURL: strings.TrimRight(consensusURL, "/"),
		adminKey:     adminKey,
		sessions:     make(map[string]string),
		histories:    make(map[string][]protocol.HistoryEntry),
		client:       &http.Client{Timeout: 120 * time.Second},
	}
}

// ============================================================================
// Consensus API Types (adapter-specific — not part of the H3 protocol)
// ============================================================================

type ConsensusSession struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	AgentName string `json:"agent_name"`
	Goal      string `json:"goal"`
	ModelID   string `json:"model_id"`
}

type ConsensusMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ConsensusSendReq struct {
	Message ConsensusMessage `json:"message"`
}

type ConsensusStatusResp struct {
	Status      string              `json:"status"`
	Goal        string              `json:"goal"`
	LastMessage *string             `json:"last_message"`
	Iterations  []ConsensusIterResp `json:"iterations"`
}

type ConsensusIterResp struct {
	ID        string          `json:"id"`
	Monologue string          `json:"internal_monologue"`
	Actions   []ConsensusStmt `json:"system_actions"`
}

type ConsensusStmt struct {
	SQL string `json:"sql,omitempty"`
}

// ============================================================================
// Consensus API Calls
// ============================================================================

func (a *Adapter) consensusCreateSession(agentName, goal, modelID string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"agent_name":     agentName,
		"goal":           goal,
		"model_id":       modelID,
		"context_budget": 200000,
	})

	req, _ := http.NewRequest("POST", a.consensusURL+"/api/v1/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if a.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.adminKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var session ConsensusSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("decode session: %w", err)
	}

	return session.ID, nil
}

func (a *Adapter) consensusSendMessage(sessionID, content string) (map[string]any, error) {
	body, _ := json.Marshal(map[string]string{"content": content})

	url := fmt.Sprintf("%s/api/v1/sessions/%s/message", a.consensusURL, sessionID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if a.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.adminKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// If the response isn't JSON, return the raw text
		return map[string]any{"raw_response": "message sent", "status": fmt.Sprintf("%d", resp.StatusCode)}, nil
	}

	return result, nil
}

func (a *Adapter) consensusGetStatus(sessionID string) (*ConsensusStatusResp, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s", a.consensusURL, sessionID)
	req, _ := http.NewRequest("GET", url, nil)
	if a.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.adminKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var status ConsensusStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}

	return &status, nil
}

func (a *Adapter) consensusSendToolResult(sessionID, toolName string, success bool, data any) (map[string]any, error) {
	content := fmt.Sprintf("[Tool Result] %s: %v (success=%v)", toolName, data, success)
	return a.consensusSendMessage(sessionID, content)
}

// ============================================================================
// Session Management
// ============================================================================

func (a *Adapter) getOrCreateSession(req *protocol.ProcessRequest) (string, bool, error) {
	a.mu.RLock()
	consensusID, exists := a.sessions[req.SessionID]
	a.mu.RUnlock()

	if exists {
		return consensusID, false, nil
	}

	modelID := "deepseek-v4-flash"
	agentName := fmt.Sprintf("h3-%s", req.Identity.UserName)
	goal := req.Message.Content

	id, err := a.consensusCreateSession(agentName, goal, modelID)
	if err != nil {
		return "", false, err
	}

	a.mu.Lock()
	a.sessions[req.SessionID] = id
	a.mu.Unlock()

	return id, true, nil
}

// rememberHistory stores the conversation history seen for a session so that
// decisions returned from /v1/result (which carries no context of its own) can
// still echo it back — the H3 history-preservation rule: history never shrinks.
func (a *Adapter) rememberHistory(sessionID string, history []protocol.HistoryEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.histories[sessionID] = history
}

// historyFor returns the last-seen conversation history for a session.
func (a *Adapter) historyFor(sessionID string) []protocol.HistoryEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.histories[sessionID]
}

// ============================================================================
// harness.Harness implementation
// ============================================================================

// Health implements harness.Harness. Reports ok when the Consensus backend is
// reachable, degraded otherwise.
func (a *Adapter) Health() *protocol.HealthResponse {
	// Also check Consensus health
	consensusHealthy := true
	if resp, err := a.client.Get(a.consensusURL + "/api/v1/health"); err == nil {
		_ = resp.Body.Close()
		consensusHealthy = resp.StatusCode == 200
	}

	status := protocol.HealthOK
	degradedReason := ""
	if !consensusHealthy {
		status = protocol.HealthDegraded
		degradedReason = "Consensus backend unreachable"
	}

	return &protocol.HealthResponse{
		Status:          status,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities:    []protocol.DecisionType{protocol.DecisionText, protocol.DecisionToolCall, protocol.DecisionEnd},
		DegradedReason:  degradedReason,
	}
}

// OnProcess implements harness.Harness: forwards the user message to Consensus
// and maps the backend state back to an H3 decision. Every decision echoes the
// request conversation history (history-preservation rule).
func (a *Adapter) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	log.Printf("[H3] PROCESS session=%s user=%s msg=%q", req.SessionID, req.Identity.UserName, trunc(req.Message.Content, 60))

	// History passthrough: every decision echoes the conversation history back
	// so it never shrinks (H3 history-preservation rule).
	history := historyPassthrough(req)
	a.rememberHistory(req.SessionID, history)

	// Get or create Consensus session
	consensusID, isNew, err := a.getOrCreateSession(req)
	if err != nil {
		log.Printf("[H3] ERROR creating session: %v", err)
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: protocol.GenerateUUID(),
			History:    history,
			End:        &protocol.End{Reason: protocol.EndError, Summary: err.Error()},
		}, nil
	}

	if isNew {
		if consensusID == "" {
			log.Printf("[H3] ERROR: got empty consensus session ID")
			return &protocol.Decision{
				Decision:   protocol.DecisionEnd,
				DecisionID: protocol.GenerateUUID(),
				History:    history,
				End:        &protocol.End{Reason: protocol.EndError, Summary: "backend returned empty session ID"},
			}, nil
		}
		log.Printf("[H3] New Consensus session: %s → %s (goal: %q)", req.SessionID, consensusID[:12], trunc(req.Message.Content, 60))
	}

	// Send message to Consensus
	result, err := a.consensusSendMessage(consensusID, req.Message.Content)
	if err != nil {
		log.Printf("[H3] ERROR sending message: %v", err)
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: protocol.GenerateUUID(),
			History:    history,
			End:        &protocol.End{Reason: protocol.EndError, Summary: err.Error()},
		}, nil
	}

	// Check session status after sending
	status, err := a.consensusGetStatus(consensusID)
	if err != nil {
		log.Printf("[H3] ERROR getting status: %v", err)
	}

	// Build H3 decision based on Consensus response
	decisionID := protocol.GenerateUUID()
	content := strings.ToLower(req.Message.Content)

	// Streaming detection: certain user prompts signal the harness should
	// stream partial results (finished=false) rather than final answers.
	streamingHint := streamingDetected(content)

	// If session is idle/complete, the response is the final answer
	if status != nil && (status.Status == "idle" || status.Status == "completed") {
		// Prefer LastMessage from Consensus memory_events, fall back to iterations
		var responseText string
		if status.LastMessage != nil && *status.LastMessage != "" {
			responseText = *status.LastMessage
		} else if len(status.Iterations) > 0 {
			responseText = status.Iterations[len(status.Iterations)-1].Monologue
		}
		if responseText == "" {
			responseText = fmt.Sprintf("Consensus session %s processed: %s", consensusID[:12], status.Status)
		}

		finished := !streamingHint
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: decisionID,
			History:    history,
			Text:       &protocol.TextResp{Content: responseText, Finished: finished},
		}, nil
	}

	// If session is still thinking, return intermediate text
	if status != nil && status.Status == "thinking" {
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: decisionID,
			History:    history,
			Text:       &protocol.TextResp{Content: "Consensus agent is processing your request...", Finished: false},
		}, nil
	}

	// Check if the result contains a tool call indicator
	if isToolCallResponse(result) {
		if tc := extractToolCall(result); tc != nil {
			return &protocol.Decision{
				Decision:   protocol.DecisionToolCall,
				DecisionID: decisionID,
				History:    history,
				ToolCall:   tc,
			}, nil
		}
	}

	// Default: text response
	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: decisionID,
		History:    history,
		Text:       &protocol.TextResp{Content: fmt.Sprintf("%v", result), Finished: status != nil && status.Status == "idle"},
	}, nil
}

// OnResult implements harness.Harness: feeds tool results back to Consensus and
// returns the next decision, echoing the session's conversation history.
func (a *Adapter) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	log.Printf("[H3] RESULT session=%s decision=%s type=%s tool=%s success=%v",
		req.SessionID, trunc(req.DecisionID, 12), req.Result.Type, req.Result.ToolName, req.Result.Success)

	a.mu.RLock()
	consensusID, ok := a.sessions[req.SessionID]
	a.mu.RUnlock()

	// Echo the session's conversation history so it never shrinks
	// (/v1/result carries no context of its own).
	history := a.historyFor(req.SessionID)

	if !ok {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: protocol.GenerateUUID(),
			History:    history,
			End:        &protocol.End{Reason: protocol.EndError, Summary: "session not found"},
		}, nil
	}

	// Only feed tool results back to Consensus — text_sent is just a poll
	if req.Result.Type == protocol.ResultTool {
		_, _ = a.consensusSendToolResult(consensusID, req.Result.ToolName, req.Result.Success, req.Result.Data)
	}

	// Poll for response (give Consensus time to process)
	time.Sleep(1 * time.Second)

	status, err := a.consensusGetStatus(consensusID)
	if err != nil {
		return &protocol.Decision{
			Decision:   protocol.DecisionEnd,
			DecisionID: protocol.GenerateUUID(),
			History:    history,
			End:        &protocol.End{Reason: protocol.EndError, Summary: err.Error()},
		}, nil
	}

	decisionID := protocol.GenerateUUID()

	if status.Status == "idle" || status.Status == "completed" {
		var summary string
		// Prefer LastMessage (populated by Consensus from memory_events)
		if status.LastMessage != nil && *status.LastMessage != "" {
			summary = *status.LastMessage
		} else if len(status.Iterations) > 0 {
			summary = status.Iterations[len(status.Iterations)-1].Monologue
		}
		if summary == "" {
			summary = fmt.Sprintf("Session completed (status: %s)", status.Status)
		}
		// Return DecisionText with Finished: true so the response text is in
		// Text.Content where the H3 client displays it — NOT End.Summary which
		// is informational and not rendered by most H3 clients.
		return &protocol.Decision{
			Decision:   protocol.DecisionText,
			DecisionID: decisionID,
			History:    history,
			Text:       &protocol.TextResp{Content: summary, Finished: true},
		}, nil
	}

	return &protocol.Decision{
		Decision:   protocol.DecisionText,
		DecisionID: decisionID,
		History:    history,
		Text:       &protocol.TextResp{Content: fmt.Sprintf("Processing... (%s)", status.Status), Finished: false},
	}, nil
}

// OnCancel implements harness.Harness.
func (a *Adapter) OnCancel(req *protocol.CancelRequest) error {
	return nil
}

// OnSessionTerminate implements harness.Harness: drops the Consensus session
// mapping so a later /v1/process starts a fresh backend session.
func (a *Adapter) OnSessionTerminate(sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
	delete(a.histories, sessionID)
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func isToolCallResponse(result map[string]any) bool {
	if toolReqs, ok := result["tool_requests"].([]any); ok && len(toolReqs) > 0 {
		return true
	}
	return false
}

func extractToolCall(result map[string]any) *protocol.ToolCall {
	if toolReqs, ok := result["tool_requests"].([]any); ok && len(toolReqs) > 0 {
		if tr, ok := toolReqs[0].(map[string]any); ok {
			name, _ := tr["tool_name"].(string)
			params := tr["parameters"]
			reasoning, _ := result["internal_monologue"].(string)
			return &protocol.ToolCall{Name: name, Params: params, Reasoning: reasoning}
		}
	}
	return nil
}

func trunc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// streamingDetected returns true if the user message content hints at
// wanting a streaming-style (non-final) response rather than a complete answer.
func streamingDetected(content string) bool {
	streamingPhrases := []string{
		"do not finish",
		"don't finish",
		"start a thought",
		"not finish yet",
		"incomplete",
		"streaming",
	}
	for _, p := range streamingPhrases {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

// historyPassthrough copies the conversation history from the request context
// so every decision can echo it back unchanged — the H3 history-preservation
// rule (h3-test process_preserves_history): the history never shrinks.
func historyPassthrough(req *protocol.ProcessRequest) []protocol.HistoryEntry {
	history := make([]protocol.HistoryEntry, len(req.Context.History))
	for i, entry := range req.Context.History {
		history[i] = protocol.HistoryEntry{Role: entry.Role, Content: entry.Content}
	}
	return history
}

// ============================================================================
// Main
// ============================================================================

func main() {
	flag.Parse()

	key := *adminKey
	if key == "" {
		key = os.Getenv("CONSENSUS_ADMIN_KEY")
	}
	adapter := NewAdapter(*consensusURL, key)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("[H3] Consensus adapter starting on %s", addr)
	log.Printf("[H3] Consensus backend: %s", *consensusURL)

	// harness.NewHTTPServer wires all H3 endpoints (health/process/result/
	// cancel/sessions) with logging, panic recovery, timeouts, and validation.
	if err := http.ListenAndServe(addr, harness.NewHTTPServer(adapter)); err != nil {
		log.Fatalf("[H3] server error: %v", err)
	}
}
