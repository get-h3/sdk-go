// Package protocol defines the H3 wire-format types.
// Generated from get-h3/protocol JSON Schema v1.
//
//go:generate go run github.com/get-h3/sdk-go/cmd/gen-types schemas/v1/*.json
package protocol

// DecisionType enumerates the six H3 decision types.
type DecisionType string

const (
	DecisionToolCall DecisionType = "tool_call"
	DecisionLLMCall  DecisionType = "llm_call"
	DecisionText     DecisionType = "text"
	DecisionWait     DecisionType = "wait"
	DecisionDelegate DecisionType = "delegate"
	DecisionEnd      DecisionType = "end"
)

// Decision is the discriminated union of all possible H3 decision types.
// The Decision field determines which sub-type is populated.
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

// ToolCall is a decision to execute a Hermes tool.
type ToolCall struct {
	Name      string `json:"name"`
	Params    any    `json:"params"`
	Reasoning string `json:"reasoning,omitempty"`
}

// LLMCall is a decision to run an LLM prompt.
type LLMCall struct {
	Model        string       `json:"model"`
	SystemPrompt string       `json:"system_prompt,omitempty"`
	Messages     []LLMMessage `json:"messages"`
	Temperature  *float64     `json:"temperature,omitempty"`
	MaxTokens    *int         `json:"max_tokens,omitempty"`
}

// LLMMessage is a single chat message within an LLMCall.
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TextResp is a decision to send text to the user.
type TextResp struct {
	Content  string `json:"content"`
	Finished bool   `json:"finished"`
}

// Wait is a decision to pause for an external signal.
type Wait struct {
	Reason          string `json:"reason"`
	DurationSeconds *int   `json:"duration_seconds,omitempty"`
	PollEndpoint    string `json:"poll_endpoint,omitempty"`
}

// Delegate is a decision to spawn a sub-agent.
type Delegate struct {
	Agent    string `json:"agent,omitempty"`
	Task     string `json:"task"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// EndReason enumerates session termination reasons.
type EndReason string

const (
	EndTaskComplete EndReason = "task_complete"
	EndUserRequest  EndReason = "user_requested"
	EndError        EndReason = "error"
	EndTimeout      EndReason = "timeout"
	EndRateLimited  EndReason = "rate_limited"
	EndCancelled    EndReason = "cancelled"
)

// End is a decision to terminate the session.
type End struct {
	Reason  EndReason `json:"reason"`
	Summary string    `json:"summary,omitempty"`
}

// ProcessRequest is the request body for POST /v1/process.
type ProcessRequest struct {
	SessionID string  `json:"session_id"`
	Message   Message `json:"message"`
	Identity  Identity `json:"identity"`
	Context   Context `json:"context"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Timestamp   string       `json:"timestamp"`
}

// AttachmentType enumerates possible attachment types.
type AttachmentType string

const (
	AttachmentImage AttachmentType = "image"
	AttachmentFile  AttachmentType = "file"
	AttachmentAudio AttachmentType = "audio"
	AttachmentVideo AttachmentType = "video"
)

// Attachment represents a file or media attachment.
type Attachment struct {
	Type     AttachmentType `json:"type"`
	URL      string         `json:"url"`
	MimeType string         `json:"mime_type"`
}

// Identity identifies the user and platform.
type Identity struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	ThreadID string `json:"thread_id,omitempty"`
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
}

// HistoryRole enumerates message roles in conversation history.
type HistoryRole string

const (
	RoleUser      HistoryRole = "user"
	RoleAssistant HistoryRole = "assistant"
	RoleSystem    HistoryRole = "system"
)

// HistoryEntry is a single entry in the conversation history.
type HistoryEntry struct {
	Role    HistoryRole `json:"role"`
	Content string      `json:"content"`
}

// Tool represents an available Hermes tool with its JSON Schema parameters.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Model represents an available LLM with pricing and capabilities.
type Model struct {
	Name               string  `json:"name"`
	Provider           string  `json:"provider"`
	CostPer1kInput     float64 `json:"cost_per_1k_input,omitempty"`
	CostPer1kOutput    float64 `json:"cost_per_1k_output,omitempty"`
	ContextWindow      int     `json:"context_window"`
	SupportsVision     bool    `json:"supports_vision,omitempty"`
	SupportsToolCalling bool   `json:"supports_tool_calling,omitempty"`
}

// SessionState tracks session-level metrics.
type SessionState struct {
	TurnCount      int     `json:"turn_count"`
	TotalToolCalls int     `json:"total_tool_calls"`
	TotalLLMCalls  int     `json:"total_llm_calls"`
	CostSoFar      float64 `json:"cost_so_far"`
	StartedAt      string  `json:"started_at"`
}

// Config provides session configuration parameters.
type Config struct {
	MaxIterations       int      `json:"max_iterations"`
	TimeoutSeconds      int      `json:"timeout_seconds"`
	ProjectDir          string   `json:"project_dir,omitempty"`
	MaxToolCallsPerTurn int      `json:"max_tool_calls_per_turn,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
}

// Context bundles all session context for a process request.
type Context struct {
	History      []HistoryEntry `json:"history"`
	Tools        []Tool         `json:"tools"`
	Models       []Model        `json:"models"`
	Memory       string         `json:"memory,omitempty"`
	Skills       []string       `json:"skills,omitempty"`
	Config       Config         `json:"config"`
	SessionState SessionState   `json:"session_state"`
}

// ResultType enumerates the types of execution results.
type ResultType string

const (
	ResultTool         ResultType = "tool_result"
	ResultLLMResponse  ResultType = "llm_response"
	ResultTextSent     ResultType = "text_sent"
	ResultDelegate     ResultType = "delegate_result"
	ResultWaitTimeout  ResultType = "wait_timeout"
	ResultError        ResultType = "error"
)

// Result is the execution result payload within a ResultRequest.
type Result struct {
	Type        ResultType `json:"type"`
	ToolName    string     `json:"tool_name,omitempty"`
	Data        any        `json:"data,omitempty"`
	DurationMs  float64    `json:"duration_ms,omitempty"`
	Success     bool       `json:"success"`
}

// ResultRequest is the request body for POST /v1/result.
type ResultRequest struct {
	SessionID  string `json:"session_id"`
	DecisionID string `json:"decision_id"`
	Result     Result `json:"result"`
}

// CancelReason enumerates cancellation reasons.
type CancelReason string

const (
	CancelUserInterrupt CancelReason = "user_interrupt"
	CancelTimeout       CancelReason = "timeout"
	CancelSystem        CancelReason = "system"
)

// CancelRequest is the request body for POST /v1/cancel.
type CancelRequest struct {
	SessionID string       `json:"session_id"`
	Reason    CancelReason `json:"reason"`
}

// HealthStatus enumerates harness health states.
type HealthStatus string

const (
	HealthOK       HealthStatus = "ok"
	HealthDegraded HealthStatus = "degraded"
	HealthDown     HealthStatus = "down"
)

// HealthResponse is the response from GET /v1/health.
type HealthResponse struct {
	Status          HealthStatus  `json:"status"`
	Version         string        `json:"version"`
	Transport       string        `json:"transport,omitempty"`
	ProtocolVersion string        `json:"protocol_version,omitempty"`
	UptimeSeconds   int           `json:"uptime_seconds,omitempty"`
	ActiveSessions  int           `json:"active_sessions,omitempty"`
	Capabilities    []DecisionType `json:"capabilities,omitempty"`
	DegradedReason  string        `json:"degraded_reason,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// SessionStatus enumerates session lifecycle states.
type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionExpired   SessionStatus = "expired"
	SessionCancelled SessionStatus = "cancelled"
)

// SessionResponse is the response from GET /v1/sessions/{session_id}.
type SessionResponse struct {
	SessionID           string        `json:"session_id"`
	StartedAt           string        `json:"started_at"`
	LastActive          string        `json:"last_active"`
	TurnCount           int           `json:"turn_count"`
	Status              SessionStatus `json:"status"`
	CurrentDecision     string        `json:"current_decision,omitempty"`
	CurrentDecisionType DecisionType  `json:"current_decision_type,omitempty"`
}

// ErrorCode enumerates machine-readable error codes.
type ErrorCode string

const (
	ErrInvalidRequest  ErrorCode = "INVALID_REQUEST"
	ErrInvalidDecision ErrorCode = "INVALID_DECISION"
	ErrUnknownTool     ErrorCode = "UNKNOWN_TOOL"
	ErrUnknownModel    ErrorCode = "UNKNOWN_MODEL"
	ErrSessionNotFound ErrorCode = "SESSION_NOT_FOUND"
	ErrSessionExpired  ErrorCode = "SESSION_EXPIRED"
	ErrHarnessTimeout  ErrorCode = "HARNESS_TIMEOUT"
	ErrInternalError   ErrorCode = "INTERNAL_ERROR"
)

// ErrorDetail contains the standard H3 error payload.
type ErrorDetail struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorResponse is the standard error response for all H3 endpoints.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
