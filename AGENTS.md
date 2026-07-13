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
    "net/http"
    "github.com/get-h3/sdk-go/harness"
)

type MyHarness struct{}

func (h *MyHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
    return &protocol.Decision{
        Decision: protocol.DecisionText,
        Text: &protocol.TextResp{Content: "Hello from Go!", Finished: true},
    }, nil
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
