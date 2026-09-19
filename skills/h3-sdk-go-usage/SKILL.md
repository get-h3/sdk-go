---
name: h3-sdk-go-usage
description: >-
  How to use the get-h3/sdk-go library for real: build an H3-compliant agent
  harness (brain-swap protocol) in Go, verify it with the h3-test battery,
  unit-test with testbed, and avoid the known contract/observability traps.
  Load this skill when working in this repo or building any harness with
  github.com/get-h3/sdk-go.
version: 1.0.5
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
| Docs (read these first) | `docs/integration-guide.md`, `docs/api-reference.md`, `docs/examples.md` | zero-to-46/46 path, contracts, example tour |
| Compliance gate | `h3-test --endpoint http://localhost:9191` (from get-h3/shim) | 46 tests / 6 categories, ~0.5s |

## Run commands

```bash
# Published module (preferred — verified 2026-09-18: v0.1.6 resolves in ~0.4s,
# consumer built from it in 2.2s and passed 46/46)
go mod init my-harness
go get github.com/get-h3/sdk-go@v0.1.6
go run main.go                 # serves :9191

# Offline/sibling-checkout variant (integration-guide §3)
go mod edit -replace github.com/get-h3/sdk-go=/path/to/sdk-go && go get github.com/get-h3/sdk-go

# Verify
h3-test --endpoint http://localhost:9191   # exit 0 = compliant
go test ./... -count=1                     # repo suite (143 Go tests), ~3s
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
   with `poll_endpoint` (proven recipe below).
6. **Unit-test without HTTP**: `testbed.NewMockHermes(h)` →
   `SendMessage(sessionID, content, user, uid)` / `SendResult(sessionID,
   decisionID, protocol.Result{...})`, assert on returned decisions.
   Caveat (GAP-039): there is no history-injecting entry point — to test
   history passthrough, drive `h.OnProcess(&protocol.ProcessRequest{...})`
   directly with your own `Context.History`.
7. **Health**: return `HealthOK`, version, transport `rest`,
   protocol_version `1.0`, and your real `Capabilities` list.

## Async work recipe (wait/resume — live-verified 2026-09-02)

For any work that could exceed the 30s server timeout:

```go
// OnProcess: start the work, hand back a wait decision
j.waitID = protocol.GenerateUUID()
go func(j *job) {
    /* slow work; when touching shared state use the HARNESS mutex, not a
       per-entity one — mixed lock scopes are a data race (`go test -race`
       catches it; this run's own first draft raced exactly that way) */
}(j)
return &protocol.Decision{Decision: protocol.DecisionWait, DecisionID: j.waitID,
    Wait: &protocol.Wait{Reason: "...", DurationSeconds: intPtr(5),
                         PollEndpoint: "/v1/process"}}, nil

// OnResult: Hermes fired wait_timeout for that id
if req.Result.Type == protocol.ResultWaitTimeout {
    if j := byWaitID(req.DecisionID); j != nil {      // correlate by decision id
        if j.done { return textDecision(j.report), nil }
        j.waitID = protocol.GenerateUUID()            // re-arm with a NEW id
        return waitDecision(j.waitID), nil
    }
}
```

**Mounting:** `NewHTTPServer` can be a subtree of your own mux —
`root.Handle("/v1/", harness.NewHTTPServer(h))` — and still passes 46/46
(the 404/405 JSON interceptor is path-agnostic; verified live).

## The llm_call round-trip recipe (live-verified 2026-09-05, published v0.1.5)

`llm_call` is the one decision type with no shipped example. The proven
pattern (battery green on 2026-09-05, scripted-Hermes client green — full source in
`docs/dogfood/2026-09-05-integration.md`):

```go
// OnProcess: guard the models contract (see traps), then ask model A.
if len(req.Context.Models) == 0 { /* text fallback, never llm_call */ }
hist := d.appendUser(req.SessionID, req.Message.Content) // snapshot incl. new turn
return &protocol.Decision{Decision: protocol.DecisionLLMCall,
    DecisionID: "llm-1", History: hist,
    LLMCall: &protocol.LLMCall{Model: req.Context.Models[0].Name,
        Messages: []protocol.LLMMessage{{Role: "user", Content: req.Message.Content}}}}, nil

// OnResult: collect answers per session; second round → critique call;
// after the final answer → synthesis text; after delivering it → end.
```

State rules that make it pass: keep `map[sessionID][]answer` AND
`map[sessionID][]HistoryEntry`; delete both in `OnSessionTerminate`; give the
struct a **constructor** — the README `&EchoHarness{}` zero-value pattern
panics (`assignment to entry in nil map`) the moment you add state maps.

## The tool_call round-trip recipe (live-verified 2026-09-18, published v0.1.6)

`tool_call` has no shipped example either (GAP-046). A real consumer whose tools
touch the filesystem, passes the full battery (46/46) and writes a real artifact
— full source in `docs/dogfood/2026-09-18-integration.md`. The loop:

```go
// OnProcess — branch on the tools the request ADVERTISED (contract), then ask
// for one. Params is `any`: send the JSON shape your tool expects.
if !hasTool(req.Context.Tools, "git_log") { /* text fallback, never tool_call */ }
return &protocol.Decision{Decision: protocol.DecisionToolCall, History: hist,
    ToolCall: &protocol.ToolCall{Name: "git_log",
        Params: map[string]any{"path": target, "limit": 5},
        Reasoning: "triage step 1: read recent history"}}, nil

// OnResult — switch on the tool that just ran. Read the payload from
// req.Result.Data (map[string]any after JSON round trip), always check
// req.Result.Success, and end with EndError on failure.
switch req.Result.ToolName {
case "git_log":
    return toolCall(hist, "file_stats", map[string]any{"path": target}, "…"), nil
case "file_stats":
    os.WriteFile(target+"/REPORT.md", []byte(report), 0o644)   // real side effect
    return end(...EndTaskComplete, "report written"), nil
}
```

Client side (what the body must send back): `result.type: "tool_result"`,
`result.tool_name` **echoing the tool that was called**, `result.data` = whatever
your tool produced, `result.success`, `result.duration_ms`. Omitting `success`
decodes as `false` — a harness that trusts it will take its error path.

**Do your own correlation if your tools have side effects.** `/v1/result` does
not check that `decision_id` matches the in-flight decision, and a duplicate
delivery re-invokes `OnResult` (GAP-049) — so a retry can run your tool step
twice. Track the last decision id per session and ignore repeats.

## Known traps (verified 2026-09-05 and 2026-09-18 — do not get bitten)

- **`/v1/result` is uncorrelated and not idempotent (GAP-049, 2026-09-18).** Any
  `decision_id` is accepted (a made-up one still drives the loop), and sending
  the same `decision_id` twice calls `OnResult` twice — verified: two identical
  `tool_call`s emitted from one repeated `tool_result`. Guard in your harness if
  your handler has side effects.
- **`/v1/cancel` does not validate its body like the other endpoints (GAP-048,
  2026-09-18).** `{"reason":"system"}` with no `session_id` returns
  `404 SESSION_NOT_FOUND "session not found: "` (not `400 INVALID_REQUEST`), and
  an out-of-enum `reason` is accepted with `200`. Malformed JSON still 400s.
  Don't read that 404 as "the session vanished" — check your own request first.
- **Health fields are the harness's job, not the SDK's (GAP-050, 2026-09-18).**
  `uptime_seconds` is never filled by `NewHTTPServer` (grep the package: the
  field does not appear), and `active_sessions` only exists if your `Health()`
  sets it — although `docs/api-reference.md` §2 shows both and
  `integration-guide.md` §8 tells operators to wire them to an LB. Fill them
  yourself (count your own sessions) or don't promise them.
- **Session status has three reachable values, not four (GAP-051, 2026-09-18).**
  `active` on create/result, `cancelled` via `/v1/cancel`, `completed` **only
  after an `end` decision**. A turn that ends with `text.finished=true` leaves
  the session `active` forever, and `expired` is unreachable (no TTL). Monitoring
  must not treat `active` as "in flight".
- **`make verify-counts` false-greens outside the monorepo checkout (GAP-053,
  2026-09-18).** In a fresh clone it prints
  `no shim canonical count at ../shim/scripts/test-count.txt; battery parity skipped`
  and still exits 0 — the battery-parity half of the guard does not run.
- **Const identifiers are not in the API reference (GAP-054, 2026-09-18).** The
  reference documents JSON strings (`"tool_result"`, `"text_sent"`, `"user"`) but
  not the Go names — `protocol.ResultTool` / `ResultTextSent` / `RoleUser` /
  `SessionStatus` come from `protocol/types.go`. Expect one grep when you write
  an `OnResult` switch.
- **Same-session concurrency race (GAP-043) — FIXED, do not code around it.**
  It was real in v0.1.5 (result POSTs vs session-status GETs on one session);
  closed 2026-09-18 (commit `f6fd890`), current on v0.1.6. Old advice "make every
  session id unique" is obsolete; unique ids remain good practice, not a fix.
- **Three battery contracts are documented only in the battery itself
  (GAP-044):** (1) `context.models=[]` → returning `llm_call` FAILS test 5_8
  ("hallucinated model") — `context.tools=[]` is the same rule for `tool_call`;
  (2) "do not finish" in the message = streaming mode →
  `text.finished=false`, and the next result must flip to `finished=true`;
  (3) `Decision.history` must NEVER shrink — seed once from context, append
  every user turn, attach the snapshot to every decision (result-driven ones
  too). When in doubt, mirror `testbed/conformance.go` — it is the compliant
  reference implementation.

- **Battery green ≠ contract clean.** The battery checks status codes and key
  presence, not value semantics. Probe error paths yourself with curl.
- **Session lifecycle is now observable — read it, don't work around it.**
  `GET /v1/sessions/{id}` reports the real status: `active` while running,
  `completed` after an `end` decision (GAP-DOG-003), `cancelled` after
  `POST /v1/cancel` (GAP-009 fills `current_decision`/`current_decision_type`,
  and cancel returns the in-flight `cancelled_decision_id`). Older traps
  (status always `active`, empty current-decision fields) are fixed — do not
  write workarounds for them.
- **Unknown sessions 404 everywhere**: `POST /v1/cancel`, `POST /v1/result`,
  `GET/DELETE /v1/sessions/{id}` all return `404 SESSION_NOT_FOUND` for
  unknown session ids (GAP-DOG-002). Guard against ghost sessions in your
  client.
- **Timeout response shape**: the server returns `504` + JSON
  `{"error":{"code":"HARNESS_TIMEOUT","message":"harness did not respond within the timeout"}}`
  (GAP-008 implementation). Parse for the JSON shape, not text/plain.
- **Sessions are in-memory and DELETE removes them**: restart forgets
  everything (documented; plan your own persistence if needed). `DELETE
  /v1/sessions/{id}` calls `OnSessionTerminate` then removes the session —
  a later `GET` returns 404 (GAP-014). `POST /v1/cancel` is the soft path:
  it keeps the session retrievable with status `cancelled`.
- **A panicking harness gets a JSON 500** `INTERNAL_ERROR` — the recover
  path returns `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`
  with `Content-Type: application/json` (GAP-027, fixed 2026-08-18). The
  process keeps serving after the panic.
- **Cancel IS terminal now (GAP-028, fixed 2026-08-18):** a late `result`
  after `cancel` no longer resurrects the session — status stays `cancelled`
  and `turn_count` is untouched (verified live on the 08-18 build). Older
  advice about reconcilers not trusting `completed` after a cancel is
  obsolete; do not re-add workarounds.
- **MockHermes DOES recover panics (GAP-029, fixed 2026-08-18):** a panicking
  harness surfaces as a returned error from SendMessage/SendResult/etc., not a
  crashed test binary (live-verified 2026-09-02). Remaining testbed gap:
  no history injection (GAP-039) — drive OnProcess raw for that.
- **Required request fields trip raw curls**: `identity.platform` and
  `identity.chat_id` are mandatory alongside `session_id` + `message.role`.
  Copy a full body from `docs/api-reference.md` §2 or the README quickstart —
  the README/quickstart curl sequence now exists and is verified verbatim
  (GAP-040 shipped; this bullet used to say those examples were missing).
- Strays in the repo (`.vfs/.dirty`, `dagger.db`, `gen-types`, `echo`,
  `minimal`, `h3-consensus-adapter` binaries) are intentional leftovers —
  leave them untracked.

## Verifying your harness end-to-end (L3 checklist)

1. `h3-test` → 46/46.
2. curl full loop: process (tool_call) → result (tool_result) → result
   (text_sent) → end; confirm history grows, never shrinks.
3. curl error paths: malformed JSON (400), missing session_id on `/v1/process`
   (400 — but on `/v1/cancel` it 404s, see traps), unknown session GET/DELETE/
   result (404), hang >30s (504 JSON HARNESS_TIMEOUT), DELETE then GET (404).
4. `go vet ./...`, `go test ./...` clean.
5. Installability from zero: clean container/box, `git clone` (anonymous, public
   repo) → `go build ./...` → `make all` → `make test` → run an example →
   battery. Measured 2026-09-18 on Go 1.22.12 (the documented minimum): **20s**,
   all green. Note: inside the `golang:*` image, use `bash -c` with
   `/usr/local/go/bin` on PATH — a login shell (`bash -lc`) drops it and
   `make all` dies at `gofmt` with Error 127.
