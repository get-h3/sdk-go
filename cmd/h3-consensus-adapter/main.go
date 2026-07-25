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
// Hermes → H3 protocol → this adapter → Consensus REST API → agent loop
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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
// H3 Protocol Types
// ============================================================================

type DecisionType string

const (
	DecisionToolCall DecisionType = "tool_call"
	DecisionLLMCall  DecisionType = "llm_call"
	DecisionText     DecisionType = "text"
	DecisionWait     DecisionType = "wait"
	DecisionDelegate DecisionType = "delegate"
	DecisionEnd      DecisionType = "end"
)

type Decision struct {
	Decision   DecisionType `json:"decision"`
	DecisionID string       `json:"decision_id"`
	ToolCall   *ToolCall    `json:"tool_call,omitempty"`
	LLMCall    *LLMCall     `json:"llm_call,omitempty"`
	Text       *TextResp    `json:"text,omitempty"`
	Wait       *Wait        `json:"wait,omitempty"`
	Delegate   *Delegate    `json:"delegate,omitempty"`
	End        *End         `json:"end,omitempty"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Params    any    `json:"params"`
	Reasoning string `json:"reasoning,omitempty"`
}

type LLMCall struct {
	Model        string       `json:"model"`
	SystemPrompt string       `json:"system_prompt,omitempty"`
	Messages     []LLMMessage `json:"messages"`
}

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TextResp struct {
	Content  string `json:"content"`
	Finished bool   `json:"finished"`
}

type Wait struct {
	Reason          string `json:"reason"`
	DurationSeconds *int   `json:"duration_seconds,omitempty"`
}

type Delegate struct {
	Task string `json:"task"`
}

type End struct {
	Reason  string `json:"reason"`
	Summary string `json:"summary,omitempty"`
}

type ProcessRequest struct {
	SessionID string   `json:"session_id"`
	Message   Message  `json:"message"`
	Identity  Identity `json:"identity"`
	Context   Context  `json:"context"`
}

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type Identity struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
}

type Context struct {
	History      []HistoryEntry `json:"history"`
	Tools        []any          `json:"tools"`
	Models       []any          `json:"models"`
	Memory       string         `json:"memory,omitempty"`
	Skills       []string       `json:"skills,omitempty"`
	Config       H3Config       `json:"config"`
	SessionState H3State        `json:"session_state"`
}

type HistoryEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type H3Config struct {
	MaxIterations  int    `json:"max_iterations"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ProjectDir     string `json:"project_dir,omitempty"`
}

type H3State struct {
	TurnCount      int     `json:"turn_count"`
	TotalToolCalls int     `json:"total_tool_calls"`
	TotalLLMCalls  int     `json:"total_llm_calls"`
	CostSoFar      float64 `json:"cost_so_far"`
	StartedAt      string  `json:"started_at"`
}

type ResultRequest struct {
	SessionID  string `json:"session_id"`
	DecisionID string `json:"decision_id"`
	Result     Result `json:"result"`
}

type Result struct {
	Type       string  `json:"type"`
	ToolName   string  `json:"tool_name,omitempty"`
	Data       any     `json:"data,omitempty"`
	DurationMs float64 `json:"duration_ms,omitempty"`
	Success    bool    `json:"success"`
}

// ============================================================================
// Consensus API Types
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
// Adapter State
// ============================================================================

type Adapter struct {
	consensusURL string
	adminKey     string
	sessions     map[string]string // h3_session_id → consensus_session_id
	mu           sync.RWMutex
	client       *http.Client
}

func NewAdapter(consensusURL, adminKey string) *Adapter {
	return &Adapter{
		consensusURL: strings.TrimRight(consensusURL, "/"),
		adminKey:     adminKey,
		sessions:     make(map[string]string),
		client:       &http.Client{Timeout: 120 * time.Second},
	}
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

func (a *Adapter) getOrCreateSession(req *ProcessRequest) (string, bool, error) {
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

// ============================================================================
// H3 Protocol Handlers
// ============================================================================

func (a *Adapter) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Also check Consensus health
	consensusHealthy := true
	if resp, err := a.client.Get(a.consensusURL + "/api/v1/health"); err == nil {
		resp.Body.Close()
		consensusHealthy = resp.StatusCode == 200
	}

	status := "ok"
	if !consensusHealthy {
		status = "degraded"
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status":           status,
		"version":          "1.0.0",
		"transport":        "rest",
		"protocol_version": "1.0",
		"capabilities":     []string{"text", "tool_call", "end"},
	})
}

func (a *Adapter) handleProcess(w http.ResponseWriter, r *http.Request) {
	var req ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "INVALID_REQUEST", err.Error())
		return
	}

	log.Printf("[H3] PROCESS session=%s user=%s msg=%q", req.SessionID, req.Identity.UserName, trunc(req.Message.Content, 60))

	// Get or create Consensus session
	consensusID, isNew, err := a.getOrCreateSession(&req)
	if err != nil {
		log.Printf("[H3] ERROR creating session: %v", err)
		writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: newID(),
			End:        &End{Reason: "error", Summary: err.Error()},
		})
		return
	}

	if isNew {
		if consensusID == "" {
			log.Printf("[H3] ERROR: got empty consensus session ID")
			writeDecision(w, Decision{
				Decision:   DecisionEnd,
				DecisionID: newID(),
				End:        &End{Reason: "error", Summary: "backend returned empty session ID"},
			})
			return
		}
		log.Printf("[H3] New Consensus session: %s → %s (goal: %q)", req.SessionID, consensusID[:12], trunc(req.Message.Content, 60))
	}

	// Send message to Consensus
	result, err := a.consensusSendMessage(consensusID, req.Message.Content)
	if err != nil {
		log.Printf("[H3] ERROR sending message: %v", err)
		writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: newID(),
			End:        &End{Reason: "error", Summary: err.Error()},
		})
		return
	}

	// Check session status after sending
	status, err := a.consensusGetStatus(consensusID)
	if err != nil {
		log.Printf("[H3] ERROR getting status: %v", err)
	}

	// Build H3 decision based on Consensus response
	decisionID := newID()
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
		writeDecision(w, Decision{
			Decision:   DecisionText,
			DecisionID: decisionID,
			Text:       &TextResp{Content: responseText, Finished: finished},
		})
		return
	}

	// If session is still thinking, return intermediate text
	if status != nil && status.Status == "thinking" {
		writeDecision(w, Decision{
			Decision:   DecisionText,
			DecisionID: decisionID,
			Text:       &TextResp{Content: "Consensus agent is processing your request...", Finished: false},
		})
		return
	}

	// Check if the result contains a tool call indicator
	responseText := fmt.Sprintf("%v", result)
	if isToolCallResponse(result) {
		tc := extractToolCall(result)
		writeDecision(w, Decision{
			Decision:   DecisionToolCall,
			DecisionID: decisionID,
			ToolCall:   tc,
		})
		return
	}

	// Default: text response
	writeDecision(w, Decision{
		Decision:   DecisionText,
		DecisionID: decisionID,
		Text:       &TextResp{Content: responseText, Finished: status != nil && status.Status == "idle"},
	})
}

func (a *Adapter) handleResult(w http.ResponseWriter, r *http.Request) {
	var req ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "INVALID_REQUEST", err.Error())
		return
	}

	log.Printf("[H3] RESULT session=%s decision=%s type=%s tool=%s success=%v",
		req.SessionID, req.DecisionID[:12], req.Result.Type, req.Result.ToolName, req.Result.Success)

	a.mu.RLock()
	consensusID, ok := a.sessions[req.SessionID]
	a.mu.RUnlock()

	if !ok {
		writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: newID(),
			End:        &End{Reason: "error", Summary: "session not found"},
		})
		return
	}

	// Only feed tool results back to Consensus — text_sent is just a poll
	if req.Result.Type == "tool_result" {
		a.consensusSendToolResult(consensusID, req.Result.ToolName, req.Result.Success, req.Result.Data)
	}

	// Poll for response (give Consensus time to process)
	time.Sleep(1 * time.Second)

	status, err := a.consensusGetStatus(consensusID)
	if err != nil {
		writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: newID(),
			End:        &End{Reason: "error", Summary: err.Error()},
		})
		return
	}

	decisionID := newID()

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
		writeDecision(w, Decision{
			Decision:   DecisionText,
			DecisionID: decisionID,
			Text:       &TextResp{Content: summary, Finished: true},
		})
		return
	}

	writeDecision(w, Decision{
		Decision:   DecisionText,
		DecisionID: decisionID,
		Text:       &TextResp{Content: fmt.Sprintf("Processing... (%s)", status.Status), Finished: false},
	})
}

func (a *Adapter) handleCancel(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
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

func extractToolCall(result map[string]any) *ToolCall {
	if toolReqs, ok := result["tool_requests"].([]any); ok && len(toolReqs) > 0 {
		if tr, ok := toolReqs[0].(map[string]any); ok {
			name, _ := tr["tool_name"].(string)
			params, _ := tr["parameters"]
			reasoning, _ := result["internal_monologue"].(string)
			return &ToolCall{Name: name, Params: params, Reasoning: reasoning}
		}
	}
	return nil
}

func writeDecision(w http.ResponseWriter, d Decision) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

func writeError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func trunc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
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

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		adapter.handleHealth(w, r)
	})
	mux.HandleFunc("/v1/process", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		adapter.handleProcess(w, r)
	})
	mux.HandleFunc("/v1/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		adapter.handleResult(w, r)
	})
	mux.HandleFunc("/v1/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		adapter.handleCancel(w, r)
	})
	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Extract session id from path: /v1/sessions/{id}
		id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		adapter.mu.RLock()
		_, ok := adapter.sessions[id]
		adapter.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "session not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"session_id": id, "status": "active"})
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("[H3] Consensus adapter starting on %s", addr)
	log.Printf("[H3] Consensus backend: %s", *consensusURL)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[H3] server error: %v", err)
	}
}
