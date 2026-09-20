# H3 Go SDK — Integration Guide

Zero-to-production for developers building an H3-compliant harness with the Go SDK.
H3 is the **brain-swap protocol**: your harness becomes the thinking brain of
[Hermes](https://github.com/get-h3/h3); Hermes is the body. Your harness implements
5 methods, serves HTTP, and the `h3-test` battery (46 tests, 6 categories) verifies
compliance against your running endpoint.

**Time to a verified 46/46 harness: under 10 minutes.**

---

## 1. Prerequisites

| Requirement | Version | Check |
|---|---|---|
| Go toolchain | 1.22+ | `go version` |
| h3-test CLI | any | `h3-test --help` |

Install the compliance tester (Python 3.10+):

```bash
pip install git+https://github.com/get-h3/shim
h3-test --help   # confirms install
```

> The SDK itself has **zero external dependencies** — standard library only.

## 2. Get the SDK

Clone the repository (or use your existing checkout):

```bash
git clone https://github.com/get-h3/sdk-go.git
cd sdk-go
go build ./...   # sanity check: everything compiles
```

## 3. Scaffold your harness module

Create your harness as a **sibling of the SDK checkout** and point Go at the local
copy with a `replace` directive. This works offline and never touches the network,
because the SDK has no dependencies:

```bash
mkdir ../h3-harness && cd ../h3-harness
go mod init h3-harness
go mod edit -replace github.com/get-h3/sdk-go=../sdk-go
go get github.com/get-h3/sdk-go
```

> Once the module is published you can skip the `replace` and just
> `go get github.com/get-h3/sdk-go` — the code is identical either way.

## 4. Implement the Harness interface

Create `main.go`. The full reference harness below is the **compliance reference** —
it is the same logic that ships in `examples/echo` and the README quickstart, and it
passes the complete 46/46 battery. Copy it verbatim, then read the anatomy notes
underneath.

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "strings"
    "sync"

    "github.com/get-h3/sdk-go/harness"
    "github.com/get-h3/sdk-go/protocol"
)

// EchoHarness implements all 5 methods of harness.Harness and is H3-compliant
// (passes the full h3-test battery, 46/46).
type EchoHarness struct {
    mu            sync.Mutex
    responseCount int
    streaming     bool // true while streaming unfinished text
}

// OnProcess is called when a new user message arrives. Returns the first
// Decision in the agent loop.
func (h *EchoHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
    // Messages containing "do not finish" request unfinished (streaming) text.
    h.mu.Lock()
    h.streaming = strings.Contains(req.Message.Content, "do not finish")
    finished := !h.streaming
    h.mu.Unlock()

    // Echo conversation history back so it never shrinks.
    history := make([]protocol.HistoryEntry, len(req.Context.History))
    for i, entry := range req.Context.History {
        history[i] = protocol.HistoryEntry{Role: entry.Role, Content: entry.Content}
    }

    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: fmt.Sprintf("Echo: %s", req.Message.Content), Finished: finished},
        History:  history,
    }, nil
}

// OnResult is called after Hermes executes a Decision. Returns the next
// Decision. Return DecisionEnd to finish.
func (h *EchoHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
    h.mu.Lock()
    h.responseCount++
    count := h.responseCount
    streaming := h.streaming
    h.mu.Unlock()
    // End after 2 results in normal mode, stay in the stream while streaming.
    if !streaming && count >= 2 {
        return &protocol.Decision{
            Decision: protocol.DecisionEnd,
            End:      &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Echo conversation complete"},
        }, nil
    }
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: fmt.Sprintf("Result received: %s", req.DecisionID), Finished: !streaming},
    }, nil
}

// OnCancel is called when the user interrupts.
func (h *EchoHarness) OnCancel(req *protocol.CancelRequest) error {
    return nil
}

// OnSessionTerminate is called on DELETE /v1/sessions/{id}.
func (h *EchoHarness) OnSessionTerminate(sessionID string) error {
    return nil
}

// Health returns harness health status. Identity + capabilities are yours; the
// SDK server fills uptime_seconds and active_sessions on the way out (it owns
// the clock and the session store) — you cannot know them here.
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
```

Save as `main.go` and run:

```bash
go run main.go
```

You now have a live harness on `http://localhost:9191`.

**Every example honors the `PORT` environment variable** (default `9191`), so when
another harness already holds the default port you move the listener instead of
editing source. The same rule applies to your own harness if you build the address
with `harness.ListenAddr()` and serve it with `harness.Serve`:

```bash
PORT=9293 go run ./examples/conformance/
# in another terminal:
h3-test --endpoint http://127.0.0.1:9293
```

A port collision is reported with the address and a hint naming the override
(`address already in use on :9191 — is another harness running? set PORT to override`).

### Test it with curl

One complete turn against that live harness. `session_id`, `identity.platform`,
`identity.chat_id` and `message.role: "user"` are all required — omitting any of them
returns HTTP 400 `INVALID_REQUEST`.

```bash
# 1. Send a user message
curl -s -X POST http://127.0.0.1:9191/v1/process \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "curl-demo-1",
    "identity": {"platform": "telegram", "chat_id": "-1001234567890"},
    "message": {"role": "user", "content": "hello from curl"},
    "context": {"history": []}
  }'
# → {"decision":"text","decision_id":"echo-001","text":{"content":"Echo: hello from curl","finished":true}}

# 2. Report the result of that decision — decision_id comes from step 1
curl -s -X POST http://127.0.0.1:9191/v1/result \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "curl-demo-1",
    "decision_id": "echo-001",
    "result": {"type": "tool_result", "success": true}
  }'

# 3. Inspect the session, then terminate it
curl -s http://127.0.0.1:9191/v1/sessions/curl-demo-1
curl -s -X DELETE http://127.0.0.1:9191/v1/sessions/curl-demo-1
```

The README's [Test it with curl](../README.md#test-it-with-curl) shows the same
sequence with real response bodies; [api-reference](api-reference.md) section 2 has
the full HTTP contract for every decision type and error code.

### Anatomy of a compliant harness

| Piece | What it does |
|---|---|
| `OnProcess` | Entry point for a new user message. Return the **first decision** of the loop. |
| `OnResult` | Called after Hermes executes a decision. Return the **next decision**; return `DecisionEnd` to finish the session. |
| `OnCancel` | User interrupt. Clean up any in-flight work and return `nil`. |
| `OnSessionTerminate` | `DELETE /v1/sessions/{id}`. Release session-scoped resources. |
| `Health` | Liveness for load balancers / the battery's health category. Report `HealthOK` (or `HealthDegraded` with a reason) and your identity fields — the SDK server fills `uptime_seconds` and `active_sessions` for you. |
| `harness.NewHTTPServer` | Wraps your harness in the full HTTP layer: routing, JSON codec, validation, session tracking, middleware. |

Three rules keep you compliant:

1. **Never shrink history.** Echo `req.Context.History` back in every decision — the
   battery asserts that conversation history never loses entries. (See
   [Decision contracts the battery enforces](#decision-contracts-the-battery-enforces)
   for the seed → append → attach pattern.)
2. **Every decision must carry its payload.** A `text` decision needs `Text` set, an
   `end` decision needs `End`, etc. (The server validates this and answers
   `500 INVALID_DECISION` otherwise.)
3. **A decision without a `DecisionID` gets a generated UUIDv4** — you may omit it,
   but prefer `protocol.NewDecision(protocol.DecisionText)` which sets it for you.

### Decision contracts the battery enforces

Three decision-level contracts are checked by name. None of them is something the
SDK can do for you: the server validates a decision's payload, not these
properties, so they live or die in the decision you return.

| Contract | Exercised as | What it means |
|---|---|---|
| No `llm_call` without a model | `no_models_available` (`no_tools_available` for tools) | `context.models == []` → never return `llm_call`; `LLMCall.Model` must be a model Hermes sent |
| A stream must close | `process_text_finished_false` / `process_text_finished_true` | a message containing `"do not finish"` → `finished=false`; the next decision for that text → `finished=true` |
| History never shrinks | `process_preserves_history` | seed from `req.Context.History`, append each turn, attach the snapshot to *every* decision |

**1. An empty `context.models` forbids `llm_call`.** Hermes sends the models it can
actually route in `context.models`, and `llm_call.model` has to name one of them.
When that list is empty there is no model to name, so a decision that names one
regardless is a fabricated model: the battery fails the run ("hallucinated model")
and real Hermes would answer `UNKNOWN_MODEL`. Return a `text` decision saying no
model is available (or `end` with `EndError`), and branch on the list you were
sent — never on your own configuration. `context.tools == []` is the same rule for
`tool_call`. Decide the streaming flag (contract 2) *before* this fallback: the
battery sends its streaming prompt with `context.models: []`, and a fallback pinned
to `finished=true` fails `process_text_finished_false`.

```go
// Decide the streaming flag FIRST (see 2 below): the battery's streaming prompt
// arrives with context.models == [], so this fallback must not force
// finished=true on it.
streaming := strings.Contains(req.Message.Content, "do not finish")

// No routable model this turn — say so; do not invent a model name.
if len(req.Context.Models) == 0 {
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        History:  history, // (3) the same snapshot every decision carries
        Text:     &protocol.TextResp{Content: "No model available in this session.", Finished: !streaming},
    }, nil
}
```

**2. `"do not finish"` is a streaming request — and the stream has to close.** The
battery sends *"Just start a thought, do not finish it yet."* and expects
`text.finished=false`. That flag means "partial text — come back to me with this
text's result", so Hermes calls `OnResult` next; the decision you return **there**
must flip `finished=true` (or be an `end`) or the session streams forever. The
unfinished text is only step 1 of 2:

| Step | Inbound | Return |
|---|---|---|
| 1 | `POST /v1/process` with `"do not finish"` in `message.content` | `text`, `finished=false` |
| 2 | `POST /v1/result` for that decision (`result.type: text_sent`) | `text`, `finished=true` — or `end` |

```go
// Step 1 (OnProcess): open the stream only for a "do not finish" message.
streaming := strings.Contains(req.Message.Content, "do not finish")
return &protocol.Decision{
    Decision: protocol.DecisionText,
    History:  history,
    Text:     &protocol.TextResp{Content: "Starting a thought…", Finished: !streaming},
}, nil

// Step 2 (OnResult): close it — this decision answers the unfinished text.
return &protocol.Decision{
    Decision: protocol.DecisionText,
    History:  history,
    Text:     &protocol.TextResp{Content: "Thought complete.", Finished: true},
}, nil
```

**3. Attach the history snapshot to every decision.** `OnProcess` seeds per-session
history once from `req.Context.History` and appends the incoming turn; `OnResult`
receives no context at all (`ResultRequest` has no `context` field), so the
snapshot has to come from state you kept. A `history` array that comes back shorter
than the one you were sent — or absent, because `History` is `omitempty` — fails
`process_preserves_history` with `history shrank`:

```go
h.mu.Lock()
// Seed once from the context, then append this turn: history only grows.
if h.sessions[sid] == nil {
    h.sessions[sid] = append(h.sessions[sid], req.Context.History...)
}
h.sessions[sid] = append(h.sessions[sid], protocol.HistoryEntry{
    Role: protocol.RoleUser, Content: req.Message.Content,
})
// Snapshot (a copy) for the decision — never alias the live slice.
history := make([]protocol.HistoryEntry, len(h.sessions[sid]))
copy(history, h.sessions[sid])
h.mu.Unlock()
```

The reference for all three is [`testbed/conformance.go`](../testbed/conformance.go)
— the harness the battery validates — and
[api-reference §4 → Decision contracts the battery enforces](api-reference.md#decision-contracts-the-battery-enforces)
has the same rules with a minimal complete harness. Note that the echo harness in
§4 above satisfies (2) and (3) on the process side but deliberately holds its demo
session in the stream; when your harness has to end, copy the flip from
`testbed/conformance.go`.

## 5. Verify with the compliance battery

In a second terminal (or with the server running in the background):

```bash
h3-test --endpoint http://localhost:9191
```

Expected output tail — all six categories green:

```text
  Health & Protocol                   7/7  ✅ PASSED
  Process Basic Flows                 8/8  ✅ PASSED
  Decision Types                      6/6  ✅ PASSED
  Result Handling                     7/7  ✅ PASSED
  Error & Edge Cases                  13/13  ✅ PASSED
  Stress & Performance                5/5  ✅ PASSED
  TOTAL                               46/46  PASSED
```

Exit code `0` means compliant (exact banner/format may vary slightly between shim
versions). If a category fails, see
[Troubleshooting](#7-troubleshooting) — most failures are one of the three
[decision contracts](#decision-contracts-the-battery-enforces) or a missing
decision payload.

> The battery is **black-box**: it only speaks HTTP to your endpoint. The same
> `h3-test` run works against harnesses built with any SDK.

## 6. Session lifecycle and error handling

The HTTP server tracks sessions for you in an in-memory store; your methods
don't manage it.

| Event | What happens |
|---|---|
| `POST /v1/process` | Session created (`status: active`), turn counter incremented. |
| `POST /v1/result` | `last_active` refreshed, turn counter incremented. Answered with `text` → session stays `active`. Answered with `end` → session becomes `completed`. |
| Harness decision `end` (from `OnProcess` or `OnResult`) | Session marked `completed`. Not terminal: a later `POST /v1/process` re-opens the session (back to `active`, counters reset), and `POST /v1/cancel` still overrides it to `cancelled`. |
| A turn finishes while the session continues | A `text` decision with `finished: true` ends only the **turn** — the session is still `active` afterwards. Only an `end` decision ends the **session** (`completed`); `finished` and `end` are different signals. |
| `POST /v1/cancel` | Your `OnCancel` runs, session marked `cancelled`, responds `{"cancelled": true, "cancelled_decision_id": "<decision_id if in flight, else empty>"}`. |
| `GET /v1/sessions/{id}` | Returns status/started/last_active/turn_count; `404 SESSION_NOT_FOUND` for unknown sessions. |
| `DELETE /v1/sessions/{id}` | Your `OnSessionTerminate` runs, then the session is **deleted** (removed from the store); responds `{"terminated": true, "session_id": "<id>"}`; a subsequent `GET /v1/sessions/{id}` returns `404 SESSION_NOT_FOUND`; `404 SESSION_NOT_FOUND` for unknown sessions. |

Notes for monitoring:

- `cancelled` is terminal — a late `POST /v1/process` or `POST /v1/result`
  never rewrites it.
- The `expired` status exists in the wire enum but this SDK never sets it:
  there is no TTL and no expiry timer. An abandoned session stays `active`
  forever until deleted — build idle cleanup on `last_active`, not on
  `expired`.

Errors follow one JSON shape everywhere:

```json
{"error": {"code": "SESSION_NOT_FOUND", "message": "session not found: abc"}}
```

| Situation | Status | Code |
|---|---|---|
| Malformed JSON body | 400 | `INVALID_REQUEST` |
| Missing `session_id` / `message.role` / `identity.platform` / `identity.chat_id` | 400 | `INVALID_REQUEST` |
| A result's `decision_id` is not the session's in-flight decision (retry, stale or invented) | 400 | `INVALID_REQUEST` |
| Your method returns an error | 500 | `INTERNAL_ERROR` |
| Your decision fails validation (missing payload, empty content) | 500 | `INVALID_DECISION` |
| Unknown session on GET/DELETE | 404 | `SESSION_NOT_FOUND` |
| Panic inside your method | 500 | recovered — server stays up |
| Handler exceeds the 30s timeout | 504 | JSON `{"error":{"code":"HARNESS_TIMEOUT","message":"harness did not respond within the timeout"}}` |

Best practices:

- Return `nil` error and a **valid decision** — or an error. Never both.
- Do expensive work that can exceed 30s asynchronously (goroutine + `wait`/`poll`
  decision) — the server's request timeout is fixed at 30s.
- A panic in your code is caught and logged with a stack trace via `slog`; the
  process keeps serving. Fix panics anyway — the client sees a `500` JSON
  `ErrorResponse` with code `INTERNAL_ERROR`.

### Result correlation and at-least-once delivery

`POST /v1/result` is correlated, not fire-and-forget: `decision_id` MUST be the
**in-flight** decision id — the id from the immediately preceding
`/v1/process` or `/v1/result` response for that session. The server checks it
before your `OnResult` runs:

| You send | You get |
|---|---|
| the in-flight decision id | `200` + the next decision (normal path) |
| a decision id that was already resolved | `400 INVALID_REQUEST`, `decision_id "…" has already been resolved for session "…"` |
| an unknown / invented decision id | `400 INVALID_REQUEST`, `decision_id "…" does not match the session's in-flight decision "…"` |
| anything, on a session with no decision in flight | `200` (nothing to correlate against) |

Why you care: delivery is **at-least-once** — a client may retry after a
timeout, and two clients can share one chat id. Before this rule a replayed
result re-ran `OnResult`, which re-executed the harness's tool step and any side
effects it performs; an invented id drove the loop too. Now both are refused
before your code is called.

Client recipe, in full:

1. Keep the decision id you are answering with your in-flight request.
2. On retry, reuse **that same** in-flight decision id — do not re-`process` to
   get a fresh one, that is a new turn.
3. On `400` with `already been resolved`, treat the result as **applied** (it
   was) and call `GET /v1/sessions/{id}` to pick up the current
   `current_decision` instead of resending.
4. On `400` with `does not match …in-flight decision`, your id is stale: `GET`
   the session and answer the decision it reports.

```bash
# Retry-safe: re-send the SAME in-flight decision id, or read the session first.
curl -s -X POST http://127.0.0.1:9191/v1/result \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"sess-abc","decision_id":"<in-flight id>","result":{"type":"tool_result","success":true}}'
# 400 "already been resolved" → already applied; re-GET instead of resending:
curl -s http://127.0.0.1:9191/v1/sessions/sess-abc
```

## 7. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `connection refused` | Server not running, or wrong port | Confirm `go run main.go` output; match `--endpoint` to the `ListenAndServe` port |
| `address already in use` | Another harness (or an example) already holds the port | Don't edit `main.go` — set `PORT` when you start the server and point the battery at the same port: `PORT=9293 go run main.go` + `h3-test --endpoint http://127.0.0.1:9293`. Every example honors `PORT` (default `9191`) |
| Battery hangs on one test | Harness method blocked >30s | Server replies `504 JSON HARNESS_TIMEOUT` (`{"error":{"code":"HARNESS_TIMEOUT",...}}`); make the method return promptly or move work to a goroutine |
| `400 INVALID_REQUEST` | Battery sends minimal requests | Don't require optional fields; only `session_id`, `message.role`, `identity.platform`, `identity.chat_id` are guaranteed |
| `400 decision_id … already been resolved` | You retried a result that was already applied | Treat it as applied and re-`GET` the session instead of resending — see [result correlation](#result-correlation-and-at-least-once-delivery) |
| `500 INVALID_DECISION` | Decision missing its payload | Every `text` decision needs `Text`; every `end` needs `End`; `text.content` must be non-empty |
| `no_models_available` fails | An `llm_call` was returned while `context.models` was empty | Branch on `len(req.Context.Models)` and answer with `text`/`end` instead of a model name — see [decision contracts](#decision-contracts-the-battery-enforces) |
| `process_text_finished_false` fails | The `"do not finish"` prompt wasn't treated as a stream | Detect it with `strings.Contains(req.Message.Content, "do not finish")` and return `Finished: false` — see [decision contracts](#decision-contracts-the-battery-enforces) |
| History tests fail | History shrank, or `history` was omitted on a result decision | Seed from `req.Context.History`, append each turn, attach the snapshot to every decision — see [decision contracts](#decision-contracts-the-battery-enforces) |
| Stream never ends | `finished=false` returned for the rest of the session | Flip `finished=true` (or `end`) on the decision you return for the unfinished text's result |
| `h3-test` not found | Shim not installed | `pip install git+https://github.com/get-h3/shim` |

## 8. Deployment notes

- **Session store is in-memory.** Restarting the process forgets all sessions
  (GET/DELETE then 404). If you need durable sessions, key your own store by
  `req.SessionID` inside the harness.
- **Put it behind a reverse proxy** (Caddy, nginx) for TLS and request size limits;
  the harness itself is plain HTTP.
- **Wire up the health endpoint** (`GET /v1/health`) to your load balancer /
  orchestrator. Your `Health()` supplies the identity/capability fields —
  `status`, `version`, `transport`, `protocol_version`, `capabilities`,
  `degraded_reason`, `error` — and the SDK server fills the two runtime metrics:
  `uptime_seconds` (seconds since `NewHTTPServer`, filled on every response) and
  `active_sessions` (sessions currently in the server's in-memory store, i.e.
  created by `POST /v1/process` and dropped by `DELETE /v1/sessions/{id}`). Both
  are always present, including `0`, so an LB rule on either can never read an
  absent field; anything your `Health()` sets for them is overwritten.
- **Prefer graceful shutdown** so in-flight sessions can finish:

  ```go
  srv := &http.Server{Addr: ":9191", Handler: harness.NewHTTPServer(&EchoHarness{})}
  go func() { log.Fatal(srv.ListenAndServe()) }()

  stop := make(chan os.Signal, 1)
  signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
  <-stop
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  _ = srv.Shutdown(ctx)
  ```

- **Logging.** Request logs (method, path, status, duration) and panic stack traces
  go to the default `slog` logger (stderr). Configure a real slog handler in
  `main()` for production structured logs.
- **Before every release**, re-run `h3-test --endpoint <prod-url>` — the battery is
  the compliance gate.

## 9. Next steps

| You want to… | Go to |
|---|---|
| See the six decision types in action | [`examples.md`](examples.md) → conformance |
| Unit-test your harness without HTTP | [`api-reference.md`](api-reference.md) → testbed, or `testbed/` package docs |
| Integrate an external agent backend | `examples/consensus/main.go` (real REST integration) |
| Full type/endpoint reference | [`api-reference.md`](api-reference.md) |
