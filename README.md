# H3 SDK for Go

[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/get-h3/sdk-go.svg)](https://pkg.go.dev/github.com/get-h3/sdk-go)

Go SDK for building [H3](https://github.com/get-h3/h3)-compliant agent harnesses.

## Install

```bash
go get github.com/get-h3/sdk-go
```

## Quickstart

```go
package main

import (
    "net/http"

    "github.com/get-h3/sdk-go/harness"
    "github.com/get-h3/sdk-go/protocol"
)

type EchoHarness struct{}

func (h *EchoHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: "Hello from Go!", Finished: true},
    }, nil
}

func (h *EchoHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
    return &protocol.Decision{
        Decision: protocol.DecisionEnd,
        End:      &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Done"},
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

## Package Structure

| Package | Description |
|---------|-------------|
| [`protocol/`](./protocol/) | Go types generated from the [H3 protocol JSON Schema](https://github.com/get-h3/protocol) – `ProcessRequest`, `Decision` (6 decision types), `ResultRequest`, `CancelRequest`, `HealthResponse`, and supporting types. |
| [`harness/`](./harness/) | Harness interface (5 methods) + HTTP handler + middleware (request logging, panic recovery, timeout). The `NewHTTPServer` function returns an `http.Handler` ready to serve. |
| [`testbed/`](./testbed/) | `MockHermes` for unit testing harness logic — send messages, results, and cancel requests; assert decisions with helper methods. |

## Examples

- [`examples/minimal/`](./examples/minimal/) — Minimal harness: responds "Hello from H3 Go SDK!" on every message.
- [`examples/echo/`](./examples/echo/) — Echo harness: echoes back the user's message content.
- [`examples/conformance/`](./examples/conformance/) — Conformance harness: full agent loop (tool_call → result → text → end) for h3-test validation.
- [`examples/consensus/`](./examples/consensus/) — Consensus reference integration: demonstrates H3 + Consensus for multi-model deliberation.

## Development

```bash
make build        # go build ./...
make test         # go test ./... -count=1
make test-short   # go test ./... -count=1 -short
make vet          # go vet ./...
make lint         # golangci-lint run ./... or staticcheck
make fmt          # gofmt -w .
make clean        # go clean ./...
make all          # fmt + vet + build + test-short
```

- **Quality gate:** GitReins mandatory (secrets, build, lint, tests). Run `gitreins guard` before committing.
- **Pre-release check:** Must pass `h3-test` from [get-h3/shim](https://github.com/get-h3/shim).

## Reference

- Spec: [get-h3/h3 → specs/04-SDK-Libraries.md](https://github.com/get-h3/h3/blob/main/specs/04-SDK-Libraries.md)
- Protocol schema: [get-h3/protocol](https://github.com/get-h3/protocol)
