// Package main — Consensus reference integration example.
//
// Demonstrates using the H3 Go SDK together with the Consensus REST API for
// multi-model deliberation. This harness implements the full H3 agent loop:
//
//  1. OnProcess — parses the user message and creates a Consensus session, then
//     returns a tool_call decision to send the message to Consensus.
//  2. OnResult — receives the tool execution result (Consensus response),
//     returns a text decision summarising the deliberation outcome, then ends
//     the H3 session.
//
// The example uses only the Go standard library (net/http, encoding/json) to
// communicate with the Consensus REST API. No external HTTP client is required.
//
// Configuration via environment variables:
//
//	CONSENSUS_URL      — base URL of the Consensus API (default: http://localhost:8080)
//	CONSENSUS_API_KEY  — bearer token for the Consensus API (default: empty)
//
// Run with:
//
//	go run ./examples/consensus
//
// The H3 harness server listens on :9191.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

// ---------------------------------------------------------------------------
// Minimal Consensus REST API types (defined inline — just what we need).
// ---------------------------------------------------------------------------

// CreateSessionRequest is the body for POST /api/v1/sessions.
type CreateSessionRequest struct {
	Model   string `json:"model,omitempty"`
	Purpose string `json:"purpose,omitempty"`
}

// CreateSessionResponse is the response from POST /api/v1/sessions.
type CreateSessionResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// SendMessageRequest is the body for POST /api/v1/sessions/:id/messages.
type SendMessageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SendMessageResponse is the response from POST /api/v1/sessions/:id/messages.
type SendMessageResponse struct {
	Content   string `json:"content,omitempty"`
	Model     string `json:"model,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
}

// ExecuteToolRequest is the body for POST /api/v1/sessions/:id/tools/execute.
type ExecuteToolRequest struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params,omitempty"`
}

// ExecuteToolResponse is the response from POST /api/v1/sessions/:id/tools/execute.
type ExecuteToolResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// ConsensusHarness
// ---------------------------------------------------------------------------

// maxTurns limits the number of deliberation turns before the session ends.
const maxTurns = 3

// turnState tracks per-session state for the deliberation loop.
type turnState struct {
	consensusSessionID string
	turn               int
}

// ConsensusHarness is an H3 harness that delegates message processing to the
// Consensus multi-model deliberation API. It demonstrates the full agent loop:
// tool_call → result → text → end.
type ConsensusHarness struct {
	mu       sync.Mutex
	baseURL  string
	apiKey   string
	client   *http.Client
	sessions map[string]*turnState
}

// NewConsensusHarness creates a harness configured from environment variables.
func NewConsensusHarness() *ConsensusHarness {
	baseURL := os.Getenv("CONSENSUS_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &ConsensusHarness{
		baseURL: baseURL,
		apiKey:  os.Getenv("CONSENSUS_API_KEY"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		sessions: make(map[string]*turnState),
	}
}

// ---------------------------------------------------------------------------
// Consensus REST helpers (net/http only)
// ---------------------------------------------------------------------------

// consensusPost sends a JSON POST request to the Consensus API and decodes the
// response into dst. It returns an error on non-2xx status codes or transport
// failures.
func (h *ConsensusHarness) consensusPost(ctx context.Context, path string, body any, dst any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := h.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("consensus request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("consensus %s returned status %d", path, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode consensus response from %s: %w", path, err)
	}
	return nil
}

// createSession opens a new session on the Consensus API.
func (h *ConsensusHarness) createSession(ctx context.Context, userMessage string) (string, error) {
	var resp CreateSessionResponse
	if err := h.consensusPost(ctx, "/api/v1/sessions", &CreateSessionRequest{
		Purpose: userMessage,
	}, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// executeTool runs a tool within a Consensus session and returns the output.
func (h *ConsensusHarness) executeTool(ctx context.Context, sessionID, tool string, params map[string]any) (*ExecuteToolResponse, error) {
	var resp ExecuteToolResponse
	path := fmt.Sprintf("/api/v1/sessions/%s/tools/execute", sessionID)
	if err := h.consensusPost(ctx, path, &ExecuteToolRequest{
		Tool:   tool,
		Params: params,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// sendMessage sends a user message to a Consensus session.
func (h *ConsensusHarness) sendMessage(ctx context.Context, sessionID, content string) (*SendMessageResponse, error) {
	var resp SendMessageResponse
	path := fmt.Sprintf("/api/v1/sessions/%s/messages", sessionID)
	if err := h.consensusPost(ctx, path, &SendMessageRequest{
		Role:    "user",
		Content: content,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Harness interface implementation
// ---------------------------------------------------------------------------

// OnProcess is called when a new user message arrives. It creates a Consensus
// session, then returns a tool_call decision asking Hermes to execute the
// "consensus_deliberate" tool on the Consensus API. This is the first step in
// the agent loop.
func (h *ConsensusHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	userMessage := req.Message.Content

	// Create a Consensus session for this conversation.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	csid, err := h.createSession(ctx, userMessage)
	if err != nil {
		log.Printf("consensus: failed to create session: %v", err)
		// Fall back to a text response if Consensus is unavailable.
		return &protocol.Decision{
			Decision: protocol.DecisionText,
			Text: &protocol.TextResp{
				Content:  fmt.Sprintf("Consensus unavailable: %v. Falling back to echo: %s", err, userMessage),
				Finished: true,
			},
		}, nil
	}

	log.Printf("consensus: created session %s for h3 session %s", csid, req.SessionID)

	// Track session state.
	h.mu.Lock()
	h.sessions[req.SessionID] = &turnState{consensusSessionID: csid, turn: 0}
	h.mu.Unlock()

	// Return a tool_call decision — Hermes will execute it and send us the result.
	return &protocol.Decision{
		Decision:   protocol.DecisionToolCall,
		DecisionID: fmt.Sprintf("consensus-deliberate-%s", req.SessionID),
		ToolCall: &protocol.ToolCall{
			Name: "consensus_deliberate",
			Params: map[string]any{
				"session_id": csid,
				"message":    userMessage,
			},
			Reasoning: "Send the user message to Consensus for multi-model deliberation.",
		},
	}, nil
}

// OnResult is called after Hermes executes a decision. It receives the tool
// result, optionally advances the deliberation, and returns the next decision.
// After maxTurns deliberation rounds, it returns a text summary and ends the
// H3 session.
func (h *ConsensusHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	ts, ok := h.sessions[req.SessionID]
	h.mu.Unlock()
	if !ok {
		// Session state missing — end gracefully.
		return &protocol.Decision{
			Decision: protocol.DecisionEnd,
			End: &protocol.End{
				Reason:  protocol.EndError,
				Summary: "consensus session state not found",
			},
		}, nil
	}

	ts.turn++

	// If we haven't hit the turn limit, make another tool call to advance
	// the deliberation.
	if ts.turn < maxTurns {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := h.sendMessage(ctx, ts.consensusSessionID,
			fmt.Sprintf("Deliberation turn %d — please refine your analysis.", ts.turn))
		if err != nil {
			log.Printf("consensus: sendMessage error on turn %d: %v", ts.turn, err)
		}

		summary := "Deliberation in progress."
		if err == nil && resp.Content != "" {
			summary = resp.Content
		}

		return &protocol.Decision{
			Decision:   protocol.DecisionToolCall,
			DecisionID: fmt.Sprintf("consensus-refine-%d-%s", ts.turn, req.SessionID),
			ToolCall: &protocol.ToolCall{
				Name: "consensus_deliberate",
				Params: map[string]any{
					"session_id": ts.consensusSessionID,
					"message":    summary,
					"turn":       ts.turn,
				},
				Reasoning: fmt.Sprintf("Deliberation turn %d of %d.", ts.turn, maxTurns),
			},
		}, nil
	}

	// Final turn — summarise the result data and end the session.
	finalResult := "Deliberation complete."
	if req.Result.Type == protocol.ResultTool && req.Result.Success {
		if data, ok := req.Result.Data.(map[string]any); ok {
			if output, ok := data["output"].(string); ok && output != "" {
				finalResult = output
			}
		}
	}

	summary := fmt.Sprintf("Consensus deliberation complete after %d turns. Final result: %s",
		maxTurns, finalResult)

	return &protocol.Decision{
		Decision: protocol.DecisionEnd,
		End: &protocol.End{
			Reason:  protocol.EndTaskComplete,
			Summary: summary,
		},
	}, nil
}

// OnCancel is called when the user interrupts the session. It cleans up the
// Consensus session state.
func (h *ConsensusHarness) OnCancel(req *protocol.CancelRequest) error {
	h.mu.Lock()
	delete(h.sessions, req.SessionID)
	h.mu.Unlock()
	log.Printf("consensus: cancelled h3 session %s (reason: %s)", req.SessionID, req.Reason)
	return nil
}

// OnSessionTerminate is called on DELETE /v1/sessions/:id. It removes the
// associated Consensus session state.
func (h *ConsensusHarness) OnSessionTerminate(sessionID string) error {
	h.mu.Lock()
	delete(h.sessions, sessionID)
	h.mu.Unlock()
	log.Printf("consensus: terminated h3 session %s", sessionID)
	return nil
}

// Health reports the harness health status, including the Consensus base URL.
func (h *ConsensusHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status:          protocol.HealthOK,
		Version:         "1.0.0",
		Transport:       "rest",
		ProtocolVersion: "1.0",
		Capabilities: []protocol.DecisionType{
			protocol.DecisionToolCall,
			protocol.DecisionText,
			protocol.DecisionEnd,
		},
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	baseURL := os.Getenv("CONSENSUS_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	log.Printf("Starting H3 Consensus reference harness")
	log.Printf("  Consensus URL: %s", baseURL)
	log.Printf("  H3 listen:     :9191")
	log.Printf("  API key set:   %v", os.Getenv("CONSENSUS_API_KEY") != "")

	h := NewConsensusHarness()
	server := harness.NewHTTPServer(h)

	log.Fatal(http.ListenAndServe(":9191", server))
}
