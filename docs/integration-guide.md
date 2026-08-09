# H3 Go SDK — Integration Guide

Zero-to-production for developers building an H3-compliant harness with the Go SDK.
H3 is the **brain-swap protocol**: your harness becomes the thinking brain of
[Hermes](https://github.com/get-h3/h3); Hermes is the body. Your harness implements
5 methods, serves HTTP, and the `h3-test` battery (44 tests, 6 categories) verifies
compliance against your running endpoint.

**Time to a verified 44/44 harness: under 10 minutes.**

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
passes the complete 44/44 battery. Copy it verbatim, then read the anatomy notes
underneath.

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "strings"

    "github.com/get-h3/sdk-go/harness"
    "github.com/get-h3/sdk-go/protocol"
)

// EchoHarness implements all 5 methods of harness.Harness and is H3-compliant
// (passes the full h3-test battery, 44/44).
type EchoHarness struct {
    responseCount int
    streaming     bool // true while streaming unfinished text
}

// OnProcess is called when a new user message arrives. Returns the first
// Decision in the agent loop.
func (h *EchoHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
    // Messages containing "do not finish" request unfinished (streaming) text.
    h.streaming = strings.Contains(req.Message.Content, "do not finish")

    // Echo conversation history back so it never shrinks.
    history := make([]protocol.HistoryEntry, len(req.Context.History))
    for i, entry := range req.Context.History {
        history[i] = protocol.HistoryEntry{Role: entry.Role, Content: entry.Content}
    }

    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: fmt.Sprintf("Echo: %s", req.Message.Content), Finished: !h.streaming},
        History:  history,
    }, nil
}

// OnResult is called after Hermes executes a Decision. Returns the next
// Decision. Return DecisionEnd to finish.
func (h *EchoHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
    h.responseCount++
    // End after 2 results in normal mode, stay in the stream while streaming.
    if !h.streaming && h.responseCount >= 2 {
        return &protocol.Decision{
            Decision: protocol.DecisionEnd,
            End:      &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Echo conversation complete"},
        }, nil
    }
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: fmt.Sprintf("Result received: %s", req.DecisionID), Finished: !h.streaming},
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

// Health returns harness health status.
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

### Anatomy of a compliant harness

| Piece | What it does |
|---|---|
| `OnProcess` | Entry point for a new user message. Return the **first decision** of the loop. |
| `OnResult` | Called after Hermes executes a decision. Return the **next decision**; return `DecisionEnd` to finish the session. |
| `OnCancel` | User interrupt. Clean up any in-flight work and return `nil`. |
| `OnSessionTerminate` | `DELETE /v1/sessions/{id}`. Release session-scoped resources. |
| `Health` | Liveness for load balancers / the battery's health category. Report `HealthOK` (or `HealthDegraded` with a reason). |
| `harness.NewHTTPServer` | Wraps your harness in the full HTTP layer: routing, JSON codec, validation, session tracking, middleware. |

Three rules keep you compliant:

1. **Never shrink history.** Echo `req.Context.History` back in every decision — the
   battery asserts that conversation history never loses entries.
2. **Every decision must carry its payload.** A `text` decision needs `Text` set, an
   `end` decision needs `End`, etc. (The server validates this and answers
   `500 INVALID_DECISION` otherwise.)
3. **A decision without a `DecisionID` gets a generated UUIDv4** — you may omit it,
   but prefer `protocol.NewDecision(protocol.DecisionText)` which sets it for you.

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
  Error & Edge Cases                  11/11  ✅ PASSED
  Stress & Performance                5/5  ✅ PASSED
  TOTAL                               44/44  PASSED
```

Exit code `0` means compliant (exact banner/format may vary slightly between shim
versions). If a category fails, see
[Troubleshooting](#7-troubleshooting) — most failures are history shrinkage or a
missing decision payload.

> The battery is **black-box**: it only speaks HTTP to your endpoint. The same
> `h3-test` run works against harnesses built with any SDK.

## 6. Session lifecycle and error handling

The HTTP server tracks sessions for you in an in-memory store; your methods
don't manage it.

| Event | What happens |
|---|---|
| `POST /v1/process` | Session created (`status: active`), turn counter incremented. |
| `POST /v1/result` | `last_active` refreshed, turn counter incremented. |
| `POST /v1/cancel` | Your `OnCancel` runs, session marked `cancelled`, responds `{"cancelled": true, "cancelled_decision_id": ""}`. |
| `GET /v1/sessions/{id}` | Returns status/started/last_active/turn_count; `404 SESSION_NOT_FOUND` for unknown sessions. |
| `DELETE /v1/sessions/{id}` | Your `OnSessionTerminate` runs, session marked `cancelled`, responds `{"terminated": true, "session_id": "<id>"}`; `404 SESSION_NOT_FOUND` for unknown sessions. |

Errors follow one JSON shape everywhere:

```json
{"error": {"code": "SESSION_NOT_FOUND", "message": "session not found: abc"}}
```

| Situation | Status | Code |
|---|---|---|
| Malformed JSON body | 400 | `INVALID_REQUEST` |
| Missing `session_id` / `message.role` / `identity.platform` / `identity.chat_id` | 400 | `INVALID_REQUEST` |
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
  process keeps serving. Fix panics anyway — the client sees a bare 500.

## 7. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `connection refused` | Server not running, or wrong port | Confirm `go run main.go` output; match `--endpoint` to the `ListenAndServe` port |
| `address already in use` | Port 9191 taken | Use another port in both `main.go` and `h3-test --endpoint http://localhost:9192` |
| Battery hangs on one test | Harness method blocked >30s | Server replies `504 JSON HARNESS_TIMEOUT` (`{"error":{"code":"HARNESS_TIMEOUT",...}}`); make the method return promptly or move work to a goroutine |
| `400 INVALID_REQUEST` | Battery sends minimal requests | Don't require optional fields; only `session_id`, `message.role`, `identity.platform`, `identity.chat_id` are guaranteed |
| `500 INVALID_DECISION` | Decision missing its payload | Every `text` decision needs `Text`; every `end` needs `End`; `text.content` must be non-empty |
| History tests fail | History shrank | Echo `req.Context.History` back verbatim in every decision |
| `h3-test` not found | Shim not installed | `pip install git+https://github.com/get-h3/shim` |

## 8. Deployment notes

- **Session store is in-memory.** Restarting the process forgets all sessions
  (GET/DELETE then 404). If you need durable sessions, key your own store by
  `req.SessionID` inside the harness.
- **Put it behind a reverse proxy** (Caddy, nginx) for TLS and request size limits;
  the harness itself is plain HTTP.
- **Wire up the health endpoint** (`GET /v1/health`) to your load balancer /
  orchestrator — it reports `status`, `version`, `uptime_seconds`, `active_sessions`,
  and `capabilities`.
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
