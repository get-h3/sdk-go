# GAP-062 — curl walkthroughs use real server-assigned decision_ids, verified live

Date: 2026-09-22 (rework pass; the first pass was the same day)
Repo: get-h3/sdk-go (branch `wt/README-ECHO-FIX`, base 700f512)
Subject: the README "Test it with curl" sequence and the integration-guide
"Test it with curl" section
Result: every documented step passes AS WRITTEN end-to-end
(process → result → result → end → session `completed` → DELETE → 404, plus the
`echo-001` negative proof), driven against the quickstart harness the walkthroughs
print — built VERBATIM, on the documented `PORT` override, with no edit to the block.

## Why this file was reworked

The first pass claimed an "AS WRITTEN" run but proved it with a copy of the quickstart
`main` that had been EDITED first: the bind address was changed from the printed
`":9191"` to `":9295"`, because the default port was already held in the evidence
environment. A Tier-2 judge rejected that, correctly: a reader following the docs
cannot edit the printed `main` either, and the docs' own answer to "port already in
use" is the `PORT` environment variable — so the quickstart `main` must read it.

This pass fixes both halves and re-runs the proof:

1. `README.md` quickstart `main` passes `harness.ListenAddr()` to `http.ListenAndServe`
   instead of the `":9191"` literal — the helper README L209-212 already tells readers
   to use, and the rule `harness/listen.go` implements (`PORT`, default `9191`). No
   other line of the block changed.
2. `docs/integration-guide.md` printed the same hardcoded block
   (`log.Fatal(http.ListenAndServe(":9191", h))`), so it was fixed the same way and its
   walkthrough was re-run verbatim too.
3. The evidence below is a VERBATIM run: the `main` block was extracted from README.md
   byte-for-byte (sha256 pinned in this file), built unmodified, and served on
   `PORT=9295`.

## The gap the walkthroughs had (why the ids were rewritten in the first pass)

The walkthroughs quoted a literal `decision_id` of `"echo-001"` (in both the README
sequence and `docs/integration-guide.md`). But the harness those docs print never sets
`DecisionID`, so the SDK server auto-generates one per decision (`harness/harness.go`
~L317/L422, H3 protocol §2.1). Following the docs verbatim, step 4's
`POST /v1/result` with `"decision_id":"echo-001"` therefore fails:

    {"error":{"code":"INVALID_REQUEST","message":"decision_id \"echo-001\" does not
    match the session's in-flight decision \"<uuid>\""}}   — HTTP 400

The docs now quote ids from one live run and say explicitly: `decision_id` is
**server-assigned** unless the harness sets `DecisionID` — copy the id from YOUR
own step response, never from the doc. The negative proof below re-confirms that 400
against the same live run.

Note: `examples/echo/main.go` DOES pin literal ids (`echo-001`/`echo-002`/`echo-end`)
— the printed quickstart harness does not. The walkthrough subject is the quickstart
harness, so that is what was driven.

## How it was driven

    # 1. extract the README Quickstart ```go block, byte-for-byte, NO edits
    python3 extract.py README.md main.go
    → extracted bytes: 2554  sha256: f8c0755ad1952fd49a82748675031ae10013615feee7104e7efb87149be5b475
    # and re-proved against the file on disk:
    python3 verify_verbatim.py README.md main.go
    → VERBATIM: main.go == README Quickstart go block, byte for byte

    # 2. the walkthrough's own two setup commands (README L107-109), with the module
    #    resolution pointed at this worktree instead of the published tag:
    #      go mod init my-harness ; go get github.com/get-h3/sdk-go
    #    go.mod → require github.com/get-h3/sdk-go v0.0.0
    #              replace github.com/get-h3/sdk-go => <this worktree>
    go build -o harness-bin . && PORT=9295 ./harness-bin

    # 3. the port override is the documented one, and it came from the ENVIRONMENT:
    ps -o pid,args -p 3572250          → ./harness-bin
    tr '\0' '\n' </proc/3572250/environ | grep PORT → PORT=9295
    ss -tlnp:
      LISTEN 0.0.0.0:9191  users:(("python",pid=1188437,fd=14))     # unrelated process
      LISTEN       *:9295  users:(("harness-bin",pid=3572250,fd=3)) # the README main

No scaffold beyond that: no code edit, no patch, no sed, no extra flag, no rebuild
between the transcript steps. `:9191` was held by an unrelated process in the evidence
environment, which is exactly the situation the documented `PORT` override exists for.

The integration-guide block was extracted the same way (sha256 `dd977c5a4758cc9e…`, quoted
below) and served on `PORT=9296`.

## The quickstart main that ran — verbatim from README.md (sha256 f8c0755ad1952fd4…)

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
    http.ListenAndServe(harness.ListenAddr(), h)
}
```

## Live transcript — README "Test it with curl", verbatim main, PORT=9295


    $ curl -s http://127.0.0.1:9295/v1/health
    {"status":"ok","version":"1.0.0","transport":"rest","protocol_version":"1.0","uptime_seconds":16,"active_sessions":0,"capabilities":["text"]}
    HTTP 200

    $ curl -s -X POST http://127.0.0.1:9295/v1/process -H 'Content-Type: application/json' -d '{session_id: curl-demo-1, identity: {platform: telegram, chat_id: -1001234567890}, message: {role: user, content: hello from curl}, context: {history: []}}'
    {"decision":"text","decision_id":"e766d816-46aa-47d1-90af-85ee65ea28c8","text":{"content":"Echo: hello from curl","finished":true}}

    HTTP 200
    copied decision_id from the step-2 response: e766d816-46aa-47d1-90af-85ee65ea28c8

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"session_id":"curl-demo-1","started_at":"2026-09-22T15:33:44-05:00","last_active":"2026-09-22T15:33:44-05:00","turn_count":1,"status":"active","current_decision":"e766d816-46aa-47d1-90af-85ee65ea28c8","current_decision_type":"text"}
    HTTP 200

    $ curl -s -X POST http://127.0.0.1:9295/v1/result -H 'Content-Type: application/json' -d '{session_id: curl-demo-1, decision_id: e766d816-46aa-47d1-90af-85ee65ea28c8, result: {type: tool_result, success: true}}'
    {"decision":"text","decision_id":"3ed9aa2a-8ac6-49d4-bba0-86c2dadf90c2","text":{"content":"Result received: e766d816-46aa-47d1-90af-85ee65ea28c8","finished":true}}

    HTTP 200

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1        (after the first result — turn 2)
    {"session_id":"curl-demo-1","started_at":"2026-09-22T15:33:44-05:00","last_active":"2026-09-22T15:33:44-05:00","turn_count":2,"status":"active","current_decision":"3ed9aa2a-8ac6-49d4-bba0-86c2dadf90c2","current_decision_type":"text"}
    HTTP 200

    $ curl -s -X POST http://127.0.0.1:9295/v1/result -H 'Content-Type: application/json' -d '{session_id: curl-demo-1, decision_id: 3ed9aa2a-8ac6-49d4-bba0-86c2dadf90c2, result: {type: tool_result, success: true}}'
    {"decision":"end","decision_id":"1f82a3bf-c470-4fba-b5ae-ba1929f0ae4c","end":{"reason":"task_complete","summary":"Echo conversation complete"}}

    HTTP 200

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1        (terminal decision -> completed)
    {"session_id":"curl-demo-1","started_at":"2026-09-22T15:33:44-05:00","last_active":"2026-09-22T15:33:44-05:00","turn_count":3,"status":"completed","current_decision":"1f82a3bf-c470-4fba-b5ae-ba1929f0ae4c","current_decision_type":"end"}
    HTTP 200

    $ curl -s -X DELETE http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"terminated":true,"session_id":"curl-demo-1"}
    HTTP 200

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1        (404 re-check, as the docs promise)
    {"error":{"code":"SESSION_NOT_FOUND","message":"session not found: curl-demo-1"}}
    HTTP 404

    $ curl -s -X POST http://127.0.0.1:9295/v1/process -H 'Content-Type: application/json' -d '{...session_id: curl-demo-neg...}'
    {"decision":"text","decision_id":"9790ef6e-3f2d-4c4e-b2f9-0b3dd2206a91","text":{"content":"Echo: hello from curl","finished":true}}

    HTTP 200
    server-assigned in-flight decision: 9790ef6e-3f2d-4c4e-b2f9-0b3dd2206a91

    $ curl -s -X POST http://127.0.0.1:9295/v1/result -H 'Content-Type: application/json' -d '{session_id: curl-demo-neg, decision_id: echo-001, ...}'   (pre-fix doc literal)
    {"error":{"code":"INVALID_REQUEST","message":"decision_id \"echo-001\" does not match the session's in-flight decision \"9790ef6e-3f2d-4c4e-b2f9-0b3dd2206a91\""}}

    HTTP 400
    cleaned up curl-demo-neg

Status codes observed, in order: 200 health · 200 process · 200 session view ·
200 result (text) · 200 session view (turn 2) · 200 result (`end`) · 200 session view
(`completed`) · 200 DELETE · 404 re-check · 400 `INVALID_REQUEST` for the pre-fix
`echo-001` literal. `started_at` / `last_active` are quoted above exactly as the server
returned them (RFC3339, server clock) — the first pass's session-view example omitted
them, which is fixed in README.md and in both transcripts here.

## Live transcript — integration-guide "Test it with curl", verbatim IG main, PORT=9296

The integration-guide block that ran (verbatim, sha256 `dd977c5a4758cc9e4b08fa24d2913d167471f6fa4edee5ce6f560d7caed4bffb`):

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
    log.Fatal(http.ListenAndServe(harness.ListenAddr(), h))
}
```

    $ curl -s http://127.0.0.1:9296/v1/health
    {"status":"ok","version":"1.0.0","transport":"rest","protocol_version":"1.0","uptime_seconds":6,"active_sessions":0,"capabilities":["text"]}
    HTTP 200

    $ curl -s -X POST http://127.0.0.1:9296/v1/process  (IG doc body, session curl-demo-1)
    {"decision":"text","decision_id":"3ba2d43b-d47e-430a-a946-781dd1e52907","text":{"content":"Echo: hello from curl","finished":true}}

    HTTP 200
    copied decision_id from the step-1 response: 3ba2d43b-d47e-430a-a946-781dd1e52907

    $ curl -s -X POST http://127.0.0.1:9296/v1/result  (IG step 2, decision_id from step 1)
    {"decision":"text","decision_id":"98d3902f-4cb6-494b-8a4f-ff0ab4ac5c7d","text":{"content":"Result received: 3ba2d43b-d47e-430a-a946-781dd1e52907","finished":true}}

    HTTP 200

    $ curl -s http://127.0.0.1:9296/v1/sessions/curl-demo-1   (IG step 3)
    {"session_id":"curl-demo-1","started_at":"2026-09-22T15:34:41-05:00","last_active":"2026-09-22T15:34:41-05:00","turn_count":2,"status":"active","current_decision":"98d3902f-4cb6-494b-8a4f-ff0ab4ac5c7d","current_decision_type":"text"}
    HTTP 200

    $ curl -s -X DELETE http://127.0.0.1:9296/v1/sessions/curl-demo-1   (IG step 3)
    {"terminated":true,"session_id":"curl-demo-1"}
    HTTP 200

    $ curl -s -X POST http://127.0.0.1:9296/v1/process  (IG claim: omitting identity -> 400 INVALID_REQUEST)
    {"error":{"code":"INVALID_REQUEST","message":"identity.platform is required"}}

    HTTP 400

    $ curl -s -X DELETE http://127.0.0.1:9296/v1/sessions/curl-demo-1   (cleanup)
    {"error":{"code":"SESSION_NOT_FOUND","message":"session not found: curl-demo-1"}}
    HTTP 404

Status codes observed, in order: 200 health · 200 process · 200 result ·
200 session view · 200 DELETE · 400 `INVALID_REQUEST` (the section's documented
"omitting a required field" claim). The ids quoted in `docs/integration-guide.md` are
this run's (`3ba2d43b-…`, one live run, already stale by design — the section tells the
reader to copy the id from their own response).

## What changed in this pass

- `README.md` — quickstart `main`: `http.ListenAndServe(":9191", h)` →
  `http.ListenAndServe(harness.ListenAddr(), h)`; the step-3 session-view example now
  shows the full server body (`started_at`/`last_active` included) and the whole
  walkthrough quotes one coherent live run; the `PORT` paragraph above the compliance
  section now names the quickstart `main` as an example of the `ListenAddr()` rule.
- `docs/integration-guide.md` — same hardcoded `main` fixed the same way
  (`log.Fatal(http.ListenAndServe(harness.ListenAddr(), h))`), the prose around it
  updated to match, and the walkthrough ids refreshed from the `:9296` run.
- `docs/verification/gap-062-echo-walkthrough.md` — this file.
- No harness, example, protocol or test source was touched.

## Gates (worktree, race-enabled)

    gofmt -l .                          → (empty)
    go build ./...                      → exit 0
    go vet ./...                        → exit 0
    go test -race -count=1 ./...        → exit 0
      6 packages "ok" (cmd/gen-types, harness, protocol, scripts/countguard,
      scripts/lintguard, testbed), 0 FAIL
    go test -count=1 -v ./...           → 168 PASS, 0 FAIL

## Cleanup

Both scratch listeners were stopped (`kill 3572250 <ig pid>`); `ss -tlnp` afterwards
shows only the unrelated `python` process on `:9191`, and nothing on `:9295`/`:9296`.
The extraction, harness binary and transcripts live under `/tmp/gap062-echo-verify/`
(scratch, not part of the repo); the quoted transcripts are reproduced in full above,
which is what the reader needs to check the claims.
