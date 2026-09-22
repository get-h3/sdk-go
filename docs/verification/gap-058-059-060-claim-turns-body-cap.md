# Verification — SDKGO-GAP-058 / 059 / 060 (result-delivery race, turn accounting, body cap)

**Date:** 2026-09-22 · **Host:** Linux (kara) · **Repo:** `get-h3/sdk-go` @ `wt/SDKGO-GAP058-060`
**Method:** every claim below was driven twice with the SAME script — once against a
binary built from `HEAD` before the fix, once against the fixed tree — plus the
in-repo regression tests re-run against reverted logic (RED proofs). Nothing here
is inferred from reading the diff.

| Finding | Before (pre-fix) | After |
|---|---|---|
| GAP-058 — 8 concurrent deliveries of ONE `decision_id` | `200 x8`, **8** `OnResult` invocations, `turn_count 9` | `200 x1` + `400 x7`, **1** `OnResult` invocation, `turn_count 2` |
| GAP-059 — two `POST /v1/process` on one session | `turn_count 1`, `started_at` moved `…:22 → …:23` | `turn_count 2`, `started_at` unchanged; a completed session still re-opens with a reset |
| GAP-060 — 11,000,143-byte body (~10.5 MiB) | `HTTP 200` in 0.107s, 11 MB echoed, session created | `HTTP 413` in 0.041s, JSON `ErrorResponse`, session never created |

## How it was driven

Two harnesses, both started the same way (`PORT=<port> <binary>`), each built
from the tree it was verifying:

- `examples/echo` — the repo's own example, for the turn-accounting and body-cap
  scenarios (sessions `before-*` / `after-*`).
- a probe harness with a **250 ms hold inside `OnResult`** (`/tmp/slowharness`,
  built with `replace github.com/get-h3/sdk-go => <tree>`), for the concurrency
  scenario. The hold widens the window in which a second delivery can reach
  `OnResult`, so the pre-fix race reproduces from curl instead of depending on
  scheduling luck — with `examples/echo` alone the pre-fix race fired in 1 of 2
  live runs (2 of 8 deliveries executed twice in the run that did fire), and the
  in-repo regression test is the deterministic gate.

### 1. Pre-fix baseline (`HEAD`, `PORT=9411` / `PORT=9421`)

    8 concurrent POST /v1/result for decision_id slow-001
        HTTP code per delivery: 200 x8
        decision_id in each response body: 8 x"decision_id":"slow-002"
    grep -c 'OnResult called'   ->  8
    curl -s /v1/sessions/race-before
    {"session_id":"race-before",…,"turn_count":9,"status":"active","current_decision":"slow-002",…}

One decision, eight harness side effects. The `turn_count` of 9 is 1 process +
8 deliveries — the deliveries that lost the race counted a turn each, and all of
them were told `200`.

    POST /v1/process (session before-b)  ->  {"…","turn_count":1,"started_at":"2026-09-22T14:03:22-05:00"}
    POST /v1/process (session before-b)  ->  {"…","turn_count":1,"started_at":"2026-09-22T14:03:23-05:00"}

A second `POST /v1/process` on the same active session restarted it: the counter
went back to 1 and `started_at` moved. (Note the RFC3339 second resolution — a
reset inside the same second is invisible here, which is why the regression test
compares the store's `time.Time`.)

    11,000,143-byte body POSTed to /v1/process
    HTTP 200 in 0.107048s
    response body (11,000,089 bytes): {"decision":"text","decision_id":"echo-001","text":{"content":"Echo: aaaa…
    /v1/health  active_sessions=3  ->  active_sessions=4

An unauthenticated caller's oversized body was decoded in full, echoed back, and
it created a session.

### 2. Post-fix (`wt/SDKGO-GAP058-060`, `PORT=9412` / `PORT=9422`)

    8 concurrent POST /v1/result for decision_id slow-001
        HTTP code per delivery: 200 x1   400 x7
        decision_id in each response body: 1 x"decision_id":"slow-002"
    grep -c 'OnResult called'   ->  1
    curl -s /v1/sessions/race-after
    {"session_id":"race-after",…,"turn_count":2,"status":"active","current_decision":"slow-002",…}

Losers are answered from the state the winner left behind:

    {"error":{"code":"INVALID_REQUEST","message":"decision_id \"echo-001\" has already been resolved for session \"after-a\""}}

Same 8-way race on `examples/echo` (`PORT=9412`, session `after-a`): `200 x1`,
`400 x7`, `turn_count 2`.

    POST /v1/process (session after-b)  ->  {"…","turn_count":1,"started_at":"2026-09-22T14:04:04-05:00"}
    POST /v1/process (session after-b)  ->  {"…","turn_count":2,"started_at":"2026-09-22T14:04:04-05:00"}   # unchanged
    POST /v1/result   (decision echo-001) -> {"decision":"end",…}   # session completed, turn_count 3
    POST /v1/process (session after-b)  ->  {"…","turn_count":1,"started_at":"2026-09-22T14:04:06-05:00"}   # re-open resets

    11,000,142-byte body POSTed to /v1/process
    HTTP 413 in 0.040981s
    response body (94 bytes): {"error":{"code":"INVALID_REQUEST","message":"request body exceeds the 10485760-byte limit"}}
    /v1/health  active_sessions=3  ->  active_sessions=3
    curl -s /v1/sessions/after-oversized  ->  404 SESSION_NOT_FOUND

The oversized request was refused before any harness method ran: no session was
created, `active_sessions` did not move, and the response came back in 41 ms
against an 11 MB body.

## RED proofs — the new regression tests against reverted logic

Each new test was run against a scratch copy of the fixed tree with exactly one
fix reverted (the worktree itself was never reverted).

| Revert | Test | Result |
|---|---|---|
| no delivery lock + claim released immediately after admission (pre-GAP-058 shape) | `TestResultEndpoint_ConcurrentDuplicateDeliveredOnce` | FAIL — `expected exactly 1 accepted delivery of "dec-claim-proc", got 8`; `expected 7 refused deliveries, got 0`; `OnResult ran 8 times for one decision_id delivered 8 times`; `expected turn_count 2 …, got 9` |
| unconditional reset in `beginProcessTurn` (pre-GAP-059 shape) | `TestProcessEndpoint_TurnAccountingMatchesDocumentedSemantics` | FAIL — `repeated POST /v1/process on an active session: turn_count 1, want 2`; `started_at moved … 14:00:42.589399535 -> 14:00:42.590062312`; `third POST /v1/process: turn_count 1, want 3` |
| `decodeBody` without `http.MaxBytesReader` (pre-GAP-060 shape) | `TestPostHandlersRejectOversizedBody` | FAIL — all four cases: `expected 413, got 400 ({"error":{"code":"INVALID_REQUEST","message":"failed to decode request body: unexpected EOF"}})` |

The memory half of the body-cap test, measured directly on the same 4 MiB body
against a 4 KiB cap (`runtime.MemStats.TotalAlloc` delta around the handler
call):

    pre-fix : status=400  allocated=16,806,168 bytes   (4x the body: decode buffers)
    post-fix: status=413  allocated=    36,440 bytes

## Suite + gates on the fixed tree

    go build ./...                     # OK
    go vet ./...                       # OK
    go test -p 1 ./...                 # all packages ok (165 tests)
    go test -race -p 1 ./harness/      # ok
    go test -p 1 -count=5 -run '<new + GAP-043 concurrency tests>' ./harness/   # ok, no flakes
    sh scripts/check-test-count.sh     # PASS — suite=165 agrees with the live count
    sh scripts/lint.sh                 # 7 issues (all pre-existing errcheck); HEAD has 8

## Scope note

`CHANGELOG.md` is a release-time artifact in this repo (the last entry is the
`v0.1.7` release cut; the GAP-061 fix that landed after it has no entry either),
so these three fixes are not written into it here — they belong in the next
release-notes pass.
