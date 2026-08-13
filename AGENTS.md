# AGENTS.md — H3 SDK for Go

Go SDK for building H3-compliant agent harnesses.

## Install

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

// EchoHarness implements all 5 methods of harness.Harness.
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

## Package Structure

- `protocol/` — Go types (generated from get-h3/protocol JSON Schema)
- `harness/` — Harness interface + HTTP handler + middleware
- `testbed/` — MockHermes for unit testing harness logic

## Development

- GitReins quality gate mandatory
- Must pass `h3-test` from get-h3/shim before release

## Reference

Spec: `get-h3/h3` → `specs/04-SDK-Libraries.md`
