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

    "github.com/get-h3/sdk-go/harness"
    "github.com/get-h3/sdk-go/protocol"
)

// MyHarness implements all 5 methods of harness.Harness.
type MyHarness struct {
    responseCount int
    streaming     bool // true while streaming unfinished text
}

func (h *MyHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
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

func (h *MyHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
    h.responseCount++
    if !h.streaming && h.responseCount >= 2 {
        return &protocol.Decision{
            Decision: protocol.DecisionEnd,
            End:      &protocol.End{Reason: protocol.EndTaskComplete, Summary: "Done"},
        }, nil
    }
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text:     &protocol.TextResp{Content: fmt.Sprintf("Result received: %s", req.DecisionID), Finished: !h.streaming},
    }, nil
}

func (h *MyHarness) OnCancel(req *protocol.CancelRequest) error {
    return nil
}

func (h *MyHarness) OnSessionTerminate(sessionID string) error {
    return nil
}

func (h *MyHarness) Health() *protocol.HealthResponse {
    return &protocol.HealthResponse{
        Status:          protocol.HealthOK,
        Version:         "1.0.0",
        Transport:       "rest",
        ProtocolVersion: "1.0",
        Capabilities:    []protocol.DecisionType{protocol.DecisionText},
    }
}

func main() {
    h := harness.NewHTTPServer(&MyHarness{})
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
