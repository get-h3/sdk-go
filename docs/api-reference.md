# H3 Go SDK — API Reference

Interface contracts, HTTP endpoints, middleware behavior, error codes, and test
helpers for harness authors. Everything here is exported by the
`github.com/get-h3/sdk-go` module (Go 1.22+, zero external dependencies).

Packages:

| Package | Purpose |
|---|---|
| `protocol` | Wire-format types (generated from the get-h3/protocol JSON Schema), validation, error codes |
| `harness` | The `Harness` interface, HTTP server, middleware |
| `testbed` | `MockHermes` + assertion helpers for unit-testing harness logic |

---

## 1. The Harness interface (`harness`)

```go
type Harness interface {
    // OnProcess is called when a new user message arrives.
    // Returns the first Decision in the agent loop.
    OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error)

    // OnResult is called after Hermes executes a Decision.
    // Returns the next Decision. Return DecisionEnd to finish.
    OnResult(req *protocol.ResultRequest) (*protocol.Decision, error)

    // OnCancel is called when the user interrupts.
    OnCancel(req *protocol.CancelRequest) error

    // OnSessionTerminate is called on DELETE /v1/sessions/:id.
    OnSessionTerminate(sessionID string) error

    // Health returns harness health status.
    Health() *protocol.HealthResponse
}
```

Contract notes:

- Return `(decision, nil)` or `(nil, err)` — never both.
- A nil `Health()` result is replaced by the server with an `ok` default.
- Decision payloads are validated by the server; a decision without
  `decision_id` gets a generated UUIDv4.
- Three decision-level contracts the SDK server does *not* enforce (empty
  `context.models`, the streaming `finished` transition, non-shrinking
  `history`) are in
  [§4 → Decision contracts the battery enforces](#decision-contracts-the-battery-enforces).

### `NewHTTPServer`

```go
func NewHTTPServer(h Harness) http.Handler
```

Wraps a harness in a fully wired `http.Handler` (routing, JSON codec, request
validation, decision validation, in-memory session tracking, middleware). Ready
for `http.ListenAndServe`.

Exposed endpoints:

| Method | Path | Handler |
|---|---|---|
| `GET` | `/v1/health` | `Health()` |
| `POST` | `/v1/process` | `OnProcess` |
| `POST` | `/v1/result` | `OnResult` |
| `POST` | `/v1/cancel` | `OnCancel` |
| `GET` | `/v1/sessions/{id}` | session status |
| `DELETE` | `/v1/sessions/{id}` | `OnSessionTerminate` |

Routes use Go 1.22 pattern matching; the `{id}` wildcard is read with
`r.PathValue("id")`.

## 2. HTTP contract

All request/response bodies are `application/json`.

### `GET /v1/health`

Response `200`:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "transport": "rest",
  "protocol_version": "1.0",
  "uptime_seconds": 123,
  "active_sessions": 2,
  "capabilities": ["text", "tool_call"],
  "degraded_reason": "",
  "error": ""
}
```

`status` ∈ `ok` | `degraded` | `down`. `capabilities` lists the
`protocol.DecisionType` values the harness can emit.

### `POST /v1/process`

Request — required fields marked **bold** (the only ones the battery guarantees):

```json
{
  "session_id": "sess-abc",
  "message": {"role": "user", "content": "hello", "timestamp": "2026-08-05T10:00:00Z"},
  "identity": {"platform": "telegram", "chat_id": "c-1", "user_name": "u", "user_id": "uid-1"},
  "context": {
    "history": [{"role": "user", "content": "hi"}],
    "tools": [{"name": "read_file", "description": "…", "parameters": {}}],
    "models": [{"name": "m", "provider": "p", "context_window": 128000}],
    "memory": "",
    "skills": [],
    "config": {"max_iterations": 10, "timeout_seconds": 30},
    "session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0, "started_at": "…"}
  }
}
```

Validation (else `400 INVALID_REQUEST`): `session_id` non-empty,
`message.role` non-empty, `identity.platform` non-empty, `identity.chat_id`
non-empty. Everything else is optional.

Response `200` — a Decision (see [§4 Decision types](#4-decision-types)):

```json
{
  "decision": "text",
  "decision_id": "9f1c…",
  "history": [{"role": "user", "content": "hi"}],
  "text": {"content": "Echo: hello", "finished": true}
}
```

### `POST /v1/result`

Request:

```json
{
  "session_id": "sess-abc",
  "decision_id": "9f1c…",
  "result": {"type": "tool_result", "tool_name": "read_file", "data": {}, "duration_ms": 12.5, "success": true}
}
```

`result.type` ∈ `tool_result` | `llm_response` | `text_sent` |
`delegate_result` | `wait_timeout` | `error`.

Response `200` — the next Decision, exactly like `/v1/process`.

### `POST /v1/cancel`

Request:

```json
{"session_id": "sess-abc", "reason": "user_interrupt"}
```

`reason` ∈ `user_interrupt` | `timeout` | `system`.

Both fields are required: `session_id` must be non-empty and `reason` must be
one of the three values above. A request that fails this check is rejected with
`400 INVALID_REQUEST` **before** the session store is consulted, so a malformed
cancel is never misreported as a vanished session:

```json
{"error": {"code": "INVALID_REQUEST", "message": "session_id is required"}}
```

`404 SESSION_NOT_FOUND` is reserved for a **valid** cancel naming a session the
harness does not have.

Response `200` — **corrected contract (GAP-003)**:

```json
{"cancelled": true, "cancelled_decision_id": "9f1c…"}
```

`cancelled_decision_id` is the id of the decision that was in flight when the
cancel arrived (tracked from the latest `OnProcess`/`OnResult` response). It is
the empty string only when no decision was in flight at cancel time. Treat a
non-null `cancelled` as authoritative.

### `GET /v1/sessions/{id}`

Response `200`:

```json
{
  "session_id": "sess-abc",
  "started_at": "2026-08-05T10:00:00Z",
  "last_active": "2026-08-05T10:01:00Z",
  "turn_count": 2,
  "status": "active",
  "current_decision": "",
  "current_decision_type": ""
}
```

`status` ∈ `active` | `completed` | `expired` | `cancelled`.

Unknown session → `404`:

```json
{"error": {"code": "SESSION_NOT_FOUND", "message": "session not found: sess-abc"}}
```

### `DELETE /v1/sessions/{id}`

Runs `OnSessionTerminate(sessionID)`, then deletes the session from the
in-memory store. The session is removed, not merely marked cancelled;
a subsequent `GET /v1/sessions/{id}` for that session returns `404 SESSION_NOT_FOUND`.

Response `200` — **corrected contract (GAP-003)**:

```json
{"terminated": true, "session_id": "sess-abc"}
```

Unknown session → `404 SESSION_NOT_FOUND` (same body as above). If
`OnSessionTerminate` returns an error → `500 INTERNAL_ERROR`.

## 3. Middleware

`NewHTTPServer` applies three layers (inner → outer):

| Layer | Behavior |
|---|---|
| Request logging | `slog.Info("request completed", method, path, status, duration)` for every request; `slog.Error("harness: panic recovered", error, stack)` on panics |
| Panic recovery | Catches panics from harness methods, returns `500` with a JSON `ErrorResponse` body `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`; the process keeps serving |
| Timeout | Custom timeout writer — **fixed 30 seconds**, wired in `withMiddleware`. On expiry the client receives `504` with a JSON `ErrorResponse` body `{"error":{"code":"HARNESS_TIMEOUT","message":"harness did not respond within the timeout"}}` (supersedes the legacy `http.TimeoutHandler` text/plain path) |

There are currently **no configuration knobs** for the middleware: the 30s timeout
is a constant of the server. Harness methods that may run longer than 30s must do
so asynchronously (goroutine + `wait` decision with `poll_endpoint`).

## 4. Decision types (`protocol`)

```go
type DecisionType string

const (
    DecisionToolCall DecisionType = "tool_call"
    DecisionLLMCall  DecisionType = "llm_call"
    DecisionText     DecisionType = "text"
    DecisionWait     DecisionType = "wait"
    DecisionDelegate DecisionType = "delegate"
    DecisionEnd      DecisionType = "end"
)
```

A `Decision` is a discriminated union — exactly one payload field must be set,
matching `decision`:

```go
type Decision struct {
    Decision   DecisionType   `json:"decision"`
    DecisionID string         `json:"decision_id"`
    History    []HistoryEntry `json:"history,omitempty"`
    ToolCall   *ToolCall      `json:"tool_call,omitempty"`
    LLMCall    *LLMCall       `json:"llm_call,omitempty"`
    Text       *TextResp      `json:"text,omitempty"`
    Wait       *Wait          `json:"wait,omitempty"`
    Delegate   *Delegate      `json:"delegate,omitempty"`
    End        *End           `json:"end,omitempty"`
}
```

### Payload types

```go
type ToolCall struct {
    Name      string `json:"name"`                // required
    Params    any    `json:"params"`              // required (any JSON value)
    Reasoning string `json:"reasoning,omitempty"`
}

type LLMCall struct {
    Model        string       `json:"model"`              // required
    SystemPrompt string       `json:"system_prompt,omitempty"`
    Messages     []LLMMessage `json:"messages"`           // required, ≥1
    Temperature  *float64     `json:"temperature,omitempty"`
    MaxTokens    *int         `json:"max_tokens,omitempty"`
}

type TextResp struct {
    Content  string `json:"content"`  // required, non-empty
    Finished bool   `json:"finished"` // true = complete, false = streaming
}

type Wait struct {
    Reason          string `json:"reason"`                     // required
    DurationSeconds *int   `json:"duration_seconds,omitempty"`
    PollEndpoint    string `json:"poll_endpoint,omitempty"`
}

type Delegate struct {
    Agent    string `json:"agent,omitempty"`
    Task     string `json:"task"`        // required
    Context  string `json:"context,omitempty"`
    Model    string `json:"model,omitempty"`
    Provider string `json:"provider,omitempty"`
}

type End struct {
    Reason  EndReason `json:"reason"`             // required
    Summary string    `json:"summary,omitempty"`
}
```

### End reasons

```go
const (
    EndTaskComplete EndReason = "task_complete"
    EndUserRequest  EndReason = "user_requested"
    EndError        EndReason = "error"
    EndTimeout      EndReason = "timeout"
    EndRateLimited  EndReason = "rate_limited"
    EndCancelled    EndReason = "cancelled"
)
```

### Validation rules (`Decision.Validate()`)

- `decision_id` required (server auto-fills UUIDv4 when empty — so *in practice*
  the server never rejects for this).
- `tool_call` → `ToolCall` non-nil, `Name` non-empty.
- `llm_call` → `LLMCall` non-nil, `Model` non-empty, ≥1 message.
- `text` → `Text` non-nil, `Content` non-empty.
- `wait` → `Wait` non-nil, `Reason` non-empty.
- `delegate` → `Delegate` non-nil, `Task` non-empty.
- `end` → `End` non-nil, `Reason` non-empty.
- Unknown `decision` value → error.

### Constructors

```go
// NewDecision creates a Decision with a fresh UUIDv4 DecisionID.
func NewDecision(decisionType DecisionType) *Decision

// GenerateUUID returns a UUIDv4 string (crypto/rand, stdlib only).
func GenerateUUID() string
```

Prefer `NewDecision` over literal construction so every decision carries a
unique, traceable id.

### Decision contracts the battery enforces

Three properties of the decisions you return are contracts, not conveniences:
`h3-test` exercises each one by name. Server validation (§7) does not cover them,
the SDK cannot enforce them for you either — only the `Decision` you hand back
carries or breaks them.

#### a. An empty `context.models` forbids `llm_call`

`LLMCall.Model` must name a model advertised in `context.models`; a name the
runtime has no route for is `UNKNOWN_MODEL` (§6). So when `context.models == []`
there is no model to name, and *any* `llm_call` is a fabricated one — the battery
reports it as a "hallucinated model" and fails the run (`no_models_available`).
`context.tools == []` is the same rule for `tool_call` (`no_tools_available`):
never call a tool the request did not advertise.

Return something honest instead — a `text` decision saying no model is available,
or an `end` with `EndError`:

```go
// Not a streaming turn (contract b decides that first) and no routable model:
// say so; do not invent a model name.
if len(req.Context.Models) == 0 {
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: "No model available in this session.", Finished: true},
    }, nil
}
```

Branch on the list you were *sent*, not on your own configuration: Hermes decides
per request what is available.

One ordering trap, and it bites on the first build: the battery's streaming prompt
(contract b) arrives with `context.models: []`. A no-model fallback that hard-codes
`finished=true` therefore answers that prompt `finished=true` and fails
`process_text_finished_false` — decide the streaming flag (b) before this fallback,
exactly as the minimal harness below does.

#### b. `"do not finish"` requests streaming — and the stream must close

`text.finished` is the streaming marker (`TextResp` above): `false` = "partial
text, expect another decision for it", `true` = "turn complete".

The battery sends the phrase **"do not finish"** (e.g. *"Just start a thought, do
not finish it yet."*) and asserts the answer is `text` with `finished=false`
(`process_text_finished_false`); a message asking for a final answer must come
back `finished=true` (`process_text_finished_true`). The transition is two steps,
and the unfinished text is only the first of them:

| Step | Inbound | Your decision |
|---|---|---|
| 1 | `POST /v1/process` whose `message.content` contains `"do not finish"` | `text` with `finished=false` |
| 2 | `POST /v1/result` for that decision (`result.type: text_sent`) | `text` with `finished=true` — or `end` |

`finished=false` tells Hermes the text is partial, so it comes back with the
result of that text instead of ending the turn. A stream that never flips never
terminates. Close it on the follow-up decision rather than holding
`finished=false` for the rest of the session — `testbed/conformance.go`, the
harness the battery validates, does exactly that (`OnResult` returns
`Finished: true`), and that is the copy to imitate. (`examples/echo` deliberately
keeps one demo session in the stream and never closes it; don't copy that part
into a session that has to end.)

```go
// OnProcess — step 1: decide whether this turn streams.
streaming := strings.Contains(req.Message.Content, "do not finish")
return &protocol.Decision{
    Decision: protocol.DecisionText,
    Text:     &protocol.TextResp{Content: "Starting a thought…", Finished: !streaming},
}, nil

// OnResult — step 2: the decision returned for an unfinished text must flip
// finished to true, or Hermes keeps asking and the session never ends.
return &protocol.Decision{
    Decision: protocol.DecisionText,
    Text:     &protocol.TextResp{Content: "Thought complete.", Finished: true},
}, nil
```

#### c. `Decision.history` must never shrink

The battery replays a populated `context.history` and fails any decision whose
`history` is shorter than the history it was sent — including *absent*, because
`History` is `omitempty` and an omitted field reads as empty
(`process_preserves_history`: `history shrank: 4 -> 0`). Three habits satisfy it:

1. **Seed** per-session history once from `req.Context.History`.
2. **Append** every new turn (the incoming user message, and your own turn if you
   model one) — history only grows.
3. **Attach the snapshot to every decision you return**, including result-driven
   ones. `ResultRequest` carries no `context` (§5), so a harness that attaches
   history only from `OnProcess` has nothing to attach on a result turn.

Keep history in your own session state and return a *copy*, so a later append
cannot mutate a snapshot Hermes is still holding:

```go
// seed once, then append this turn
s.history = append(s.history, protocol.HistoryEntry{Role: protocol.RoleUser, Content: req.Message.Content})

// snapshot for the decision
history := make([]protocol.HistoryEntry, len(s.history))
copy(history, s.history)

return &protocol.Decision{
    Decision: protocol.DecisionText,
    History:  history, // same snapshot on process AND result decisions
    Text:     &protocol.TextResp{Content: "…", Finished: true},
}, nil
```

#### Minimal harness honouring all three

`testbed/conformance.go` implements the full pattern; this is the same shape,
reduced to the three contracts.

```go
type MyHarness struct {
    mu       sync.Mutex
    sessions map[string][]protocol.HistoryEntry // session id → history snapshot
}

// The map must be non-nil before the first OnProcess/OnResult.
func NewMyHarness() *MyHarness {
    return &MyHarness{sessions: map[string][]protocol.HistoryEntry{}}
}

func (h *MyHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
    h.mu.Lock()
    // (c) seed once from the context we were sent, append this turn, snapshot.
    hist := h.sessions[req.SessionID]
    if hist == nil {
        hist = append(hist, req.Context.History...)
    }
    hist = append(hist, protocol.HistoryEntry{Role: protocol.RoleUser, Content: req.Message.Content})
    h.sessions[req.SessionID] = hist
    history := make([]protocol.HistoryEntry, len(hist))
    copy(history, hist)
    models := req.Context.Models
    streaming := strings.Contains(req.Message.Content, "do not finish") // (b) step 1
    h.mu.Unlock()

    // (b) A streaming request stays unfinished, whatever else is true of it.
    // The battery's streaming prompt arrives with an EMPTY model list, so this
    // branch must come before the no-model fallback.
    if streaming {
        return &protocol.Decision{
            Decision: protocol.DecisionText,
            History:  history,
            Text:     &protocol.TextResp{Content: "Starting a thought…", Finished: false},
        }, nil
    }

    // (a) no advertised model ⇒ never llm_call.
    if len(models) == 0 {
        return &protocol.Decision{
            Decision: protocol.DecisionText,
            History:  history,
            Text:     &protocol.TextResp{Content: "No model available in this session.", Finished: true},
        }, nil
    }

    return &protocol.Decision{
        Decision: protocol.DecisionText,
        History:  history,
        Text:     &protocol.TextResp{Content: "Echo: " + req.Message.Content, Finished: true},
    }, nil
}

func (h *MyHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
    h.mu.Lock()
    // (c) ResultRequest has no context — the snapshot comes from session state.
    history := make([]protocol.HistoryEntry, len(h.sessions[req.SessionID]))
    copy(history, h.sessions[req.SessionID])
    h.mu.Unlock()

    return &protocol.Decision{
        Decision: protocol.DecisionText,
        History:  history,
        // (b) step 2 — close the stream.
        Text: &protocol.TextResp{Content: "Result received.", Finished: true},
    }, nil
}
```

`h3-test --endpoint http://localhost:9191` is the arbiter for all three; the
integration guide's
[Decision contracts the battery enforces](integration-guide.md#decision-contracts-the-battery-enforces)
walks the same rules from the consumer side. (`DecisionID` is omitted in the
snippet above — the server fills a UUIDv4 when it is empty; use
`protocol.NewDecision` when you want to own the id.)

## 5. Request/response types (`protocol`)

```go
type ProcessRequest struct {
    SessionID string   `json:"session_id"`   // required
    Message   Message  `json:"message"`      // required (role required)
    Identity  Identity `json:"identity"`     // required (platform, chat_id required)
    Context   Context  `json:"context"`
}

type Message struct {
    Role        string       `json:"role"`
    Content     string       `json:"content"`
    Attachments []Attachment `json:"attachments,omitempty"` // type ∈ image | file | audio | video (AttachmentType)
    Timestamp   string       `json:"timestamp"`
}

type Identity struct {
    Platform string `json:"platform"`
    ChatID   string `json:"chat_id"`
    ThreadID string `json:"thread_id,omitempty"`
    UserName string `json:"user_name"`
    UserID   string `json:"user_id"`
}

type HistoryEntry struct {
    Role    HistoryRole `json:"role"`    // user | assistant | system
    Content string      `json:"content"`
}

type Tool struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Parameters  map[string]any `json:"parameters"` // JSON Schema
}

type Model struct {
    Name                string  `json:"name"`
    Provider            string  `json:"provider"`
    CostPer1kInput      float64 `json:"cost_per_1k_input,omitempty"`
    CostPer1kOutput     float64 `json:"cost_per_1k_output,omitempty"`
    ContextWindow       int     `json:"context_window"`
    SupportsVision      bool    `json:"supports_vision,omitempty"`
    SupportsToolCalling bool    `json:"supports_tool_calling,omitempty"`
}

type Config struct {
    MaxIterations       int      `json:"max_iterations"`
    TimeoutSeconds      int      `json:"timeout_seconds"`
    ProjectDir          string   `json:"project_dir,omitempty"`
    MaxToolCallsPerTurn int      `json:"max_tool_calls_per_turn,omitempty"`
    Temperature         *float64 `json:"temperature,omitempty"`
}

type Context struct {
    History      []HistoryEntry `json:"history"`
    Tools        []Tool         `json:"tools"`
    Models       []Model        `json:"models"`
    Memory       string         `json:"memory,omitempty"`
    Skills       []string       `json:"skills,omitempty"`
    Config       Config         `json:"config"`
    SessionState SessionState   `json:"session_state"`
}

type SessionState struct {
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
    Type       ResultType `json:"type"`     // tool_result | llm_response | text_sent | delegate_result | wait_timeout | error
    ToolName   string     `json:"tool_name,omitempty"`
    Data       any        `json:"data,omitempty"`
    DurationMs float64    `json:"duration_ms,omitempty"`
    Success    bool       `json:"success"`
}

type CancelRequest struct {
    SessionID string       `json:"session_id"`
    Reason    CancelReason `json:"reason"` // user_interrupt | timeout | system
}

type HealthResponse struct {
    Status          HealthStatus   `json:"status"` // ok | degraded | down
    Version         string         `json:"version"`
    Transport       string         `json:"transport,omitempty"`
    ProtocolVersion string         `json:"protocol_version,omitempty"`
    UptimeSeconds   int            `json:"uptime_seconds,omitempty"`
    ActiveSessions  int            `json:"active_sessions,omitempty"`
    Capabilities    []DecisionType `json:"capabilities,omitempty"`
    DegradedReason  string         `json:"degraded_reason,omitempty"`
    Error           string         `json:"error,omitempty"`
}

type SessionResponse struct {
    SessionID           string        `json:"session_id"`
    StartedAt           string        `json:"started_at"`
    LastActive          string        `json:"last_active"`
    TurnCount           int           `json:"turn_count"`
    Status              SessionStatus `json:"status"` // active | completed | expired | cancelled
    CurrentDecision     string        `json:"current_decision,omitempty"`
    CurrentDecisionType DecisionType  `json:"current_decision_type,omitempty"`
}

// POST /v1/cancel — 200 response
type CancelResponse struct {
    Cancelled           bool   `json:"cancelled"`
    CancelledDecisionID string `json:"cancelled_decision_id"`
}

// DELETE /v1/sessions/{id} — 200 response
type SessionTerminateResponse struct {
    Terminated bool   `json:"terminated"`
    SessionID  string `json:"session_id"`
}
```

## 6. Error codes

```go
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
```

Wire shape — every error response, all endpoints:

```json
{"error": {"code": "…", "message": "…", "details": {}}}
```

```go
type ErrorDetail struct {
    Code    ErrorCode      `json:"code"`
    Message string         `json:"message"`
    Details map[string]any `json:"details,omitempty"`
}

type ErrorResponse struct {
    Error ErrorDetail `json:"error"`
}
```

| Code | Emitted by the SDK server | Meaning |
|---|---|---|
| `INVALID_REQUEST` | `400` on `/v1/process` and `/v1/cancel` (and any decode failure) | Malformed JSON or missing required field. `/v1/process`: `session_id`, `message.role`, `identity.platform`, `identity.chat_id`. `/v1/cancel`: `session_id`, or `reason` outside `user_interrupt` \| `timeout` \| `system` |
| `INVALID_DECISION` | `500` after `OnProcess`/`OnResult` | Decision failed `Validate()` — missing payload for its type |
| `INTERNAL_ERROR` | `500` | Your method returned a non-nil error |
| `SESSION_NOT_FOUND` | `404` on `GET`/`DELETE /v1/sessions/{id}` and on `POST /v1/cancel` with a **valid** body naming an unknown session | No session with that id in the store |
| `UNKNOWN_TOOL` / `UNKNOWN_MODEL` / `SESSION_EXPIRED` | — (defined for protocol completeness) | Return these from your own `ErrorResponse` if you build a custom server; the SDK server does not emit them |
| `HARNESS_TIMEOUT` | `504` (middleware timeout) | Harness method exceeded the 30s server timeout; emitted by the SDK server |

> Note: the SDK server maps *every* method error to `INTERNAL_ERROR`. If you need
> fine-grained codes on the wire, extend `NewHTTPServer`'s handler or implement
> your own handlers using `protocol.ErrorResponse` — the types are exported.

## 7. Validation (`protocol.Validate`)

```go
func (r *ProcessRequest) Validate() error   // required: session_id, message.role, identity.platform, identity.chat_id
func (r *CancelRequest) Validate() error    // required: session_id, reason ∈ the CancelReason enum
func (d *Decision) Validate() error         // decision_id + payload per type (see §4)

type ValidationError struct {
    Code    ErrorCode
    Message string
    Details map[string]any
}
```

`ValidationError` implements `error`; use `errors.As` to unwrap and inspect
`Code`/`Details` (the failing field is in `Details["field"]`).

## 8. Testbed helpers (`testbed`)

### MockHermes — drive a harness without HTTP

```go
func NewMockHermes(h harness.Harness) *MockHermes

func (m *MockHermes) SendMessage(sessionID, content, userName, userID string) (*protocol.Decision, error)
func (m *MockHermes) SendResult(sessionID, decisionID string, result protocol.Result) (*protocol.Decision, error)
func (m *MockHermes) SendCancel(sessionID string, reason protocol.CancelReason) error
func (m *MockHermes) TerminateSession(sessionID string) error
func (m *MockHermes) Health() *protocol.HealthResponse

// Tracking fields for assertions
m.LastDecision  *protocol.Decision
m.LastError     error
m.Decisions     []*protocol.Decision
m.SessionCount  int
```

`SendMessage` builds a full `ProcessRequest` (identity `platform: "test"`,
`DefaultContext()`) so tests exercise realistic input.

### Fixtures

```go
func DefaultTools() []protocol.Tool      // read_file, write_file, terminal
func DefaultModels() []protocol.Model    // test-model (tool-calling), test-vision-model
func DefaultContext() protocol.Context   // history+tools+models+config+session_state
func QuickIdentity(userName, userID string) protocol.Identity
func QuickMessage(content string) protocol.Message
```

### Conformance harness

```go
// NewConformanceHarness returns the S04 §6 conformance harness — the same
// behaviour served by examples/conformance — for reuse in tests and demos.
func NewConformanceHarness() harness.Harness
```

Keyword-driven full agent loop (`tool_call` → result → `text` → `end`) covering
all six decision types. Serves the same logic `h3-test` validates.

### Assertions

```go
func AssertDecisionType(t *testing.T, d *protocol.Decision, expected protocol.DecisionType)
func AssertTextContent(t *testing.T, d *protocol.Decision, content string, finished bool)
func AssertEndReason(t *testing.T, d *protocol.Decision, expected protocol.EndReason)
func AssertNoError(t *testing.T, err error)
func AssertDecisionValid(t *testing.T, d *protocol.Decision)
```

### Example test

```go
func TestEchoHarness(t *testing.T) {
    m := testbed.NewMockHermes(&EchoHarness{})
    dec, err := m.SendMessage("s1", "hello", "alice", "u1")
    testbed.AssertNoError(t, err)
    testbed.AssertDecisionType(t, dec, protocol.DecisionText)
    testbed.AssertTextContent(t, dec, "Echo: hello", true)
    testbed.AssertDecisionValid(t, dec)
}
```

## 9. Config options

The SDK server itself exposes **no configuration struct** — routing, JSON codec,
validation, and middleware are fixed by `NewHTTPServer`. The knobs that exist:

| Knob | Where | Default |
|---|---|---|
| Listen address/port | your `http.ListenAndServe(":9191", h)` call | yours |
| Request timeout | `harness` middleware (`http.TimeoutHandler`) | fixed 30s |
| Logging | default `slog` logger (stderr) | `slog.Default()` |
| Per-session runtime config | `protocol.Config` inside each `ProcessRequest.Context` (sent by Hermes) | client-controlled |
| Harness-specific env vars | your code — see `examples/consensus` (`CONSENSUS_URL`, `CONSENSUS_API_KEY`) | yours |
