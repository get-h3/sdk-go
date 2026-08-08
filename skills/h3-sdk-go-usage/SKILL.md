---
name: h3-sdk-go-usage
description: >-
  How to use the get-h3/sdk-go library for real: build an H3-compliant agent
  harness (brain-swap protocol) in Go, verify it with the h3-test battery,
  unit-test with testbed, and avoid the known contract/observability traps.
  Load this skill when working in this repo or building any harness with
  github.com/get-h3/sdk-go.
version: 1.0.0
category: software-development
---

# H3 Go SDK Usage

H3 = **brain-swap protocol**: an external agent system (your Go harness)
becomes the thinking brain of Hermes; Hermes is the body. This SDK is the Go
side. A harness = 5 methods + an HTTP server + a passing `h3-test` battery.

## What it is / entry points

| Piece | Where | What |
|---|---|---|
| Wire types | `protocol/` | `Decision` (6 types), `ProcessRequest`, `ResultRequest`, `ErrorResponse`, `SessionResponse` |
| Server + interface | `harness/` | `Harness` interface (5 methods), `NewHTTPServer(h) http.Handler` |
| Test helpers | `testbed/` | `MockHermes`, `ConformanceHarness`, `DefaultContext()` |
| Docs (read these first) | `docs/integration-guide.md`, `docs/api-reference.md`, `docs/examples.md` | zero-to-43/43 path, contracts, example tour |
| Compliance gate | `h3-test --endpoint http://localhost:9191` (from get-h3/shim) | 43 tests / 6 categories, ~0.25s |

## Run commands

```bash
# Consumer scaffold (offline-friendly; SDK has zero deps)
go mod init my-harness
go mod edit -replace github.com/get-h3/sdk-go=/path/to/sdk-go
go get github.com/get-h3/sdk-go
go run main.go                 # serves :9191

# Verify
h3-test --endpoint http://localhost:9191   # exit 0 = compliant
go test ./... -count=1                     # repo suite, ~0.5s
```

## The right way (proven patterns)

1. **Implement all 5 methods** — `OnProcess`, `OnResult`, `OnCancel`,
   `OnSessionTerminate`, `Health`. Missing one = compile error (the interface
   enforces it).
2. **Preserve history in every decision**: seed a per-session slice from
   `req.Context.History`, append the user message, echo the snapshot back via
   `Decision.History`. Shrinking history fails battery tests.
3. **`protocol.NewDecision(type)`** for every decision — auto UUIDv4. Always
   set the matching payload (`Text` for text, `End` for end, `ToolCall` for
   tool_call...); the server 500s with `INVALID_DECISION` otherwise, and
   `text.content` must be non-empty.
4. **Agent loop:** `OnProcess` → first decision; Hermes executes it →
   `OnResult(result)` → next decision → ... → `end`. Tool results arrive in
   `req.Result.Data` as `map[string]any` (JSON-decoded) or `string` — handle
   both. Check `req.Result.Success`/`ResultError` and end with
   `EndError` on failure.
5. **Never block >30s in a method** — fixed, non-configurable timeout; 504
   JSON `HARNESS_TIMEOUT` on expiry. Long work → goroutine + `wait` decision
   with `poll_endpoint`.
6. **Unit-test without HTTP**: `testbed.NewMockHermes(h)` →
   `SendMessage(sessionID, content, user, uid)` / `SendResult(sessionID,
   decisionID, protocol.Result{...})`, assert on returned decisions.
7. **Health**: return `HealthOK`, version, transport `rest`,
   protocol_version `1.0`, and your real `Capabilities` list.

## Known traps (verified 2026-08-08 — do not get bitten)

- **Battery green ≠ contract clean.** The battery checks status codes and key
  presence, not value semantics. Probe error paths yourself with curl.
- **Session status lies**: `GET /v1/sessions/{id}` stays `"active"` forever —
  never `"completed"`, even after an `end` decision (GAP-DOG-003). Don't trust
  it for lifecycle decisions; key your own store by `req.SessionID`.
- **No current-decision observability**: `current_decision`/
  `current_decision_type` are never populated; `cancelled_decision_id` is
  always `""` (GAP-009). A streaming decision you cancel looks identical to
  cancelling nothing.
- **Cancel/result don't validate sessions**: `POST /v1/cancel` and
  `POST /v1/result` return 200 for unknown session ids (GAP-DOG-002) while
  GET/DELETE 404. Guard against ghost sessions in your client.
- **Timeout docs are stale**: the server returns `504` + JSON
  `HARNESS_TIMEOUT`, but integration-guide.md L229/L245 and api-reference.md
  L214/L499 still describe `503 text/plain "request timeout"` (GAP-DOG-001).
  Parse for the JSON shape.
- **Sessions are in-memory**: restart forgets everything (documented; plan
  your own persistence if needed).
- **`DELETE /v1/sessions/{id}`** marks the session `cancelled` but keeps it
  retrievable (200 on later GET) — expected per docs, but surprising.
- Strays in the repo (`.vfs/.dirty`, `dagger.db`, `gen-types`, `echo`,
  `minimal`, `h3-consensus-adapter` binaries) are intentional leftovers —
  leave them untracked.

## Verifying your harness end-to-end (L3 checklist)

1. `h3-test` → 43/43.
2. curl full loop: process (tool_call) → result (tool_result) → result
   (text_sent) → end; confirm history grows, never shrinks.
3. curl error paths: malformed JSON (400), missing session_id (400), unknown
   session GET/DELETE (404), hang >30s (504 JSON HARNESS_TIMEOUT).
4. `go vet ./...`, `go test ./...` clean.
