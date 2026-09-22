# H3 SDK for Go

[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/get-h3/sdk-go.svg)](https://pkg.go.dev/github.com/get-h3/sdk-go)

Go SDK for building [H3](https://github.com/get-h3/h3)-compliant agent harnesses.

## Install

Requires a Go toolchain — **1.22+**, matching the badge above. If you do not have
one yet, install it first: <https://go.dev/dl/>.

```bash
go get github.com/get-h3/sdk-go
```

## Quickstart

```go
package main

import (
    "fmt"
    "net/http"
    "strings"
    "sync"

    "github.com/get-h3/sdk-go/harness"
    "github.com/get-h3/sdk-go/protocol"
)

// EchoHarness implements all 5 Harness methods and is H3-compliant
// (passes the full h3-test battery, 46/46).
type EchoHarness struct {
    mu            sync.Mutex
    responseCount int
    streaming     bool // true while streaming unfinished text
}

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

func (h *EchoHarness) OnCancel(req *protocol.CancelRequest) error {
    return nil
}

func (h *EchoHarness) OnSessionTerminate(sessionID string) error {
    return nil
}

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
    http.ListenAndServe(":9191", h)
}
```

Save as `main.go`, then:

```bash
go mod init my-harness
go get github.com/get-h3/sdk-go
go run main.go
```

The harness exposes six REST endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/health` | Harness health check |
| `POST` | `/v1/process` | New user message |
| `POST` | `/v1/result` | Result of a prior decision |
| `POST` | `/v1/cancel` | Cancel a running session |
| `GET` | `/v1/sessions/{id}` | Get session status |
| `DELETE` | `/v1/sessions/{id}` | Terminate a session |

## Test it with curl

With the harness above listening on `:9191`, this sequence drives one complete turn.
Copy it as-is: `identity.platform` and `identity.chat_id` are required by the wire
contract, and `message.role` must be `user` — omitting any of the three returns HTTP
400 `INVALID_REQUEST`.

**First run compiles before it serves.** `go run main.go` builds the binary before the
listener binds, so the first `curl`s can report `HTTP:000` / `connection refused` while
the compile runs — seconds on a warm build cache, longer cold. Let the compile finish, or
poll `curl -sf http://127.0.0.1:9191/v1/health` until it answers 200, before step 1.
(The bundled examples additionally log their listen address on startup — this quickstart's
`main` does not.)

```bash
# 1. Health check (HTTP 200)
curl -s http://127.0.0.1:9191/v1/health
```

```json
{"status":"ok","version":"1.0.0","transport":"rest","protocol_version":"1.0","uptime_seconds":0,"active_sessions":0,"capabilities":["text"]}
```

`uptime_seconds` and `active_sessions` are filled by the SDK server (it owns the
clock and the session store), not by your `Health()` — your `Health()` supplies
the identity fields. See
[`docs/api-reference.md` §2](docs/api-reference.md#get-v1health).

```bash
# 2. Send a user message (HTTP 200)
curl -s -X POST http://127.0.0.1:9191/v1/process \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "curl-demo-1",
    "identity": {"platform": "telegram", "chat_id": "-1001234567890"},
    "message": {"role": "user", "content": "hello from curl"},
    "context": {"history": []}
  }'
```

```json
{"decision":"text","decision_id":"f4be6275-852e-465c-9829-8047d713704c","text":{"content":"Echo: hello from curl","finished":true}}
```

`decision_id` is **server-assigned** here: the quickstart harness never sets
`DecisionID`, so the SDK server stamps a fresh UUID onto every decision (H3
protocol §2.1) — any harness that leaves it unset gets the same treatment. The
id above is from one live run and is already stale; **copy the `decision_id`
from your own step-2 response** into step 4, never from this page.

```bash
# 3. Inspect the session (HTTP 200)
curl -s http://127.0.0.1:9191/v1/sessions/curl-demo-1

# 4. Report the result of that decision (HTTP 200) — decision_id comes from step 2
curl -s -X POST http://127.0.0.1:9191/v1/result \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "curl-demo-1",
    "decision_id": "f4be6275-852e-465c-9829-8047d713704c",
    "result": {"type": "tool_result", "success": true}
  }'

# 5. Terminate the session (HTTP 200) — a following GET returns 404 SESSION_NOT_FOUND
curl -s -X DELETE http://127.0.0.1:9191/v1/sessions/curl-demo-1
```

Step 3 returns the live session state, for example:

```json
{"session_id":"curl-demo-1","turn_count":1,"status":"active","current_decision":"f4be6275-852e-465c-9829-8047d713704c","current_decision_type":"text"}
```

Every decision type, every error code and the full field reference:
[docs/api-reference.md](docs/api-reference.md).

Running on another port? **Every example honors the `PORT` environment variable**
(default `9191`), so a second harness can run next to one that already holds the
default port:

```bash
PORT=9293 go run ./examples/conformance/
# in another terminal:
h3-test --endpoint http://127.0.0.1:9293
```

Your own `main` gets the same behaviour from the shared helpers — `harness.ListenAddr()`
resolves `PORT` (default `9191`) for the address you pass to `http.ListenAndServe`, and
`harness.Serve(addr, h)` reports a bind collision with a hint naming the `PORT` override
instead of a bare `address already in use`.

## Verify compliance (46/46)

The quickstart harness above is the compliance reference — [`examples/echo`](./examples/echo/)
is the same logic. To reproduce the number yourself, install the battery and run it
against the live endpoint:

```bash
pip install git+https://github.com/get-h3/shim   # provides the `h3-test` CLI
h3-test --endpoint http://127.0.0.1:9191         # -> TOTAL 46/46 PASSED, exit 0
```

The battery is black-box (HTTP only), so the same command validates a harness built
with any SDK. Zero-to-production walkthrough: [docs/integration-guide.md](docs/integration-guide.md).

## Package Structure

| Package | Description |
|---------|-------------|
| [`protocol/`](./protocol/) | Go wire types maintained against the [H3 protocol JSON Schema](https://github.com/get-h3/protocol) – `ProcessRequest`, `Decision` (6 decision types), `ResultRequest`, `CancelRequest`, `HealthResponse`, and supporting types. `go generate ./protocol/` runs `cmd/gen-types`, a schema validator (it does not emit Go code). |
| [`harness/`](./harness/) | Harness interface (5 methods) + HTTP handler + middleware (request logging, panic recovery, timeout). The `NewHTTPServer` function returns an `http.Handler` ready to serve. |
| [`testbed/`](./testbed/) | `MockHermes` for unit testing harness logic — send messages, results, and cancel requests; assert decisions with helper methods. |

## Examples

- [`examples/minimal/`](./examples/minimal/) — Minimal harness: responds "Hello from H3 Go SDK!" on every message.
- [`examples/echo/`](./examples/echo/) — Echo harness: echoes back the user's message content.
- [`examples/conformance/`](./examples/conformance/) — Conformance harness: full agent loop (tool_call → result → text → end) for h3-test validation.
- [`examples/llm-roundtrip/`](./examples/llm-roundtrip/) — Deliberator harness: the `llm_call` round trip (process → llm_call → result → llm_call → result → text VERDICT → end), with a scripted fake-Hermes client.
- [`examples/consensus/`](./examples/consensus/) — Consensus reference integration: demonstrates H3 + Consensus for multi-model deliberation.

### Running an example

Where you build an example from decides whether it needs its own module:

- **Inside a clone** — the repository's root module (`module github.com/get-h3/sdk-go`)
  covers every example directory, so it builds as-is with no `go mod init`:

  ```bash
  cd examples/minimal && go build .   # exit 0
  ```

- **Copied out of the clone** into your own directory — the copy sits outside that
  module, so a bare build fails until you give it one:

  ```bash
  go build .                          # exit 1: go: go.mod file not found in current directory or any parent directory
  go mod init my-module
  go get github.com/get-h3/sdk-go
  go build .                          # exit 0
  ```

Either path needs a Go toolchain ([1.22+](https://go.dev/dl/)) installed first.

## Documentation

See [docs/](docs/): [integration-guide](docs/integration-guide.md) (zero-to-production), [api-reference](docs/api-reference.md) (contracts & errors), [examples](docs/examples.md) (example tour).

## Development

```bash
make build        # go build ./...
make test         # go test ./... -count=1
make test-short   # go test ./... -count=1 -short
make vet          # go vet ./...
make lint         # scripts/lint.sh — golangci-lint, staticcheck fallback, fails if neither is installed
make fmt          # gofmt -w .
make clean        # go clean ./...
make all          # fmt + vet + build + test-short
```

- **Quality gate:** GitReins mandatory (secrets, build, lint, tests). Run `gitreins guard` before committing.
- **Pre-release check:** Must pass `h3-test` from [get-h3/shim](https://github.com/get-h3/shim) — install and run commands under [Verify compliance](#verify-compliance-4646) above.

## Reference

- Spec: [get-h3/h3 → specs/04-SDK-Libraries.md](https://github.com/get-h3/h3/blob/main/specs/04-SDK-Libraries.md)
- Protocol schema: [get-h3/protocol](https://github.com/get-h3/protocol)
