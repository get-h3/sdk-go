# H3 Go SDK — Diagnostics Trail

How this SDK is built, why it's shaped this way, the errors that have been
found along the way (by foremen, stand-ins, and dogfood runs), and the right
way to use it. This is the record for answering "does this project actually
work?" — written from a real-use run on 2026-08-08.

## 1. How the thing is built

Four packages, zero external dependencies (stdlib only — a deliberate design
constraint that makes `go get` offline-friendly):

- **`protocol/`** — wire types generated from `get-h3/protocol` JSON Schema
  (via `cmd/gen-types`, `//go:generate` reads `protocol/schemas/v1/*.json`).
  Key shapes: `Decision` (discriminated union of 6 types), `ProcessRequest`,
  `ResultRequest`, `SessionResponse`, `ErrorResponse` (`{error:{code,message}}`).
  Helpers: `NewDecision(type)` (sets UUIDv4), `GenerateUUID()`, `Validate()`.
- **`harness/`** — the `Harness` interface (5 methods) + `NewHTTPServer(h)`
  which wires: Go 1.22 pattern-mux routes, JSON codec, request validation
  (`session_id`/`message.role`/`identity.platform`/`identity.chat_id`
  required), decision validation, in-memory `sessionStore`, and middleware.
  Middleware order (outer→inner): timeout writer → panic recovery + slog
  logging. The **timeout writer** (added 2026-08-07, GAP-008) replaced
  `http.TimeoutHandler`: it runs the handler in a goroutine, buffers writes,
  and on deadline expiry discards the buffer and writes a clean JSON
  `HARNESS_TIMEOUT` 504 — the 30s deadline is a hardcoded constant, not
  configurable.
- **`testbed/`** — `MockHermes` (SendMessage/SendResult/SendCancel/
  TerminateSession), `ConformanceHarness` (the 45/45 reference harness),
  `DefaultContext()/DefaultTools()/DefaultModels()` for fast unit tests.
- **`cmd/` + `examples/`** — `gen-types` generator; minimal/echo/conformance/
  consensus examples; `h3-consensus-adapter` (external-agent bridge, refactored
  onto SDK types in GAP-007).

Compliance is gated by **`h3-test`** (45 tests, 6 categories) from
`get-h3/shim` — a black-box HTTP battery run against any running harness.

## 2. Error history (what was found and fixed — and what it teaches)

| When | Finding | Lesson |
|---|---|---|
| 2026-08-04 sweep | GAP-001: AGENTS.md quickstart didn't compile (missing 4 of 5 methods) | Docs rot independently of code; the quickstart is a test artifact, compile it in CI |
| 2026-08-04 sweep | GAP-002: README echo example failed 3/44 battery tests | A "minimal" example can quietly be non-compliant; the conformance example is the reference |
| 2026-08-04 sweep | GAP-003: cancel/delete response bodies didn't match OpenAPI | The battery checks status codes, not body shapes — curl the contract directly |
| 2026-08-04 sweep | GAP-004: no docs at all | → integration-guide + api-reference + examples.md |
| 2026-08-07 sweep | GAP-005: `go generate ./protocol/` broken (schemas missing) | Generated-code repos must ship their inputs |
| 2026-08-07 sweep | GAP-007: consensus adapter duplicated local protocol types, dropped History → battery history tests failed | Duplicated wire types drift; import the SDK types |
| 2026-08-08 hunter probe | GAP-008: timeout returned 503 text/plain (protocol requires JSON ErrorResponse); `ErrHarnessTimeout` defined but unused | Grep for defined-but-unused error codes — they mark unimplemented contract paths. Fixed: custom timeout writer, 504 JSON. **The docs were never updated → GAP-DOG-001** |
| 2026-08-08 hunter probe | GAP-009: `cancelled_decision_id` hardcoded `""`; `current_decision*` never populated | Battery passes on key presence, not value semantics |
| 2026-08-08 dogfood | GAP-DOG-001: docs still document the pre-GAP-008 timeout behavior | A fix that changes observable behavior must touch docs in the same commit |
| 2026-08-08 dogfood | GAP-DOG-002: cancel returns 200 for unknown sessions; OpenAPI defines 404 | Error-path consistency: GET/DELETE 404, POST doesn't — probe all verbs |
| 2026-08-08 dogfood | GAP-DOG-003: session status never becomes "completed" | The session store is a counter+timestamp box; lifecycle semantics are the harness's job today |

**Pattern:** every sweep found contract-vs-implementation gaps the green
battery missed. The battery is necessary but not sufficient — real use
(integrating, curling error paths, reading OpenAPI) is what finds these.

## 3. The right way to use it (verified 2026-08-08)

1. **Scaffold** as a sibling module with a `replace` directive
   (`integration-guide.md §3`) — offline, zero deps, works.
2. **Implement the 5 methods.** Keep a per-session history slice; seed from
   `req.Context.History`, append the user message, echo a snapshot back in
   EVERY decision (battery rule: never shrink history).
3. **Build decisions with `protocol.NewDecision(type)`** — you get a UUID for
   free; always set the payload matching the type (server 500s with
   `INVALID_DECISION` otherwise; `text.content` must be non-empty).
4. **The agent loop:** `OnProcess` returns the first decision; Hermes executes
   it and calls `OnResult` with the outcome; return the next decision; return
   `end` to finish. Tool results arrive in `req.Result.Data` as decoded JSON
   (`map[string]any`) or string — handle both.
5. **Never block >30s in a method** — the timeout is fixed and non-configurable;
   do long work in a goroutine and return a `wait` decision with
   `poll_endpoint`.
6. **Test without HTTP** using `testbed`: `NewMockHermes(h)` →
   `SendMessage/SendResult`, assert on `LastDecision`.
7. **Gate with `h3-test --endpoint http://localhost:9191`** — 0.24s, run it
   constantly.
8. **Then probe beyond the battery:** curl the error paths (unknown sessions,
   malformed bodies, hangs), and diff behavior against `h3-protocol.yaml`.
   Today that means verifying the fixes landed: session status transitions
   `active` → `completed` on an end decision and stays `cancelled` after a
   cancel, even when a late result arrives (GAP-DOG-003 fixed tick #114;
   GAP-028 cancelled-is-terminal tick #178), `current_decision*` and
   `cancelled_decision_id` are populated (GAP-009 fixed tick #112), cancel and
   result return 404 `SESSION_NOT_FOUND` for unknown sessions (GAP-DOG-002
   fixed tick #111), and the timeout path returns 504 JSON `HARNESS_TIMEOUT`
   (GAP-008 fixed tick #110, docs corrected tick #113).

## 4. Known limits (as of 2026-08-08)

- Sessions are in-memory only; restart forgets everything (documented).
- Timeout fixed at 30s; no middleware configuration knobs.
- Session observability is incomplete: no `completed` status, no current
  decision tracking, empty `cancelled_decision_id` (GAP-009 + GAP-DOG-003).
- `POST /v1/cancel` and `/v1/result` accept unknown sessions (GAP-DOG-002).
- Docs lag the timeout implementation (GAP-DOG-001).

## 5. Dogfood run 2026-08-18 (published-consumer path, v0.1.2)

**How this run differed from 08-08:** the earlier run scaffolded with a local
`replace` directive because the published module lagged HEAD (GAP-010, GAP-025).
v0.1.2 was tagged 2026-08-15, so this run was a plain `go get
github.com/get-h3/sdk-go@latest` — the honest consumer experience.

**What was built:** `h3-reminders`, a reminders-assistant harness in a scratch
module, deliberately exercising all six decision types, streaming, history
passthrough, panic recovery, lifecycle, and concurrency. Full source + probe
table: `docs/dogfood/2026-08-18-integration.md`.

**Everything held up.** `go get` → v0.1.2; build/vet/tests clean; all 6
endpoints behave per OpenAPI; all 6 decision types serialize correctly;
`h3-test` 45/45 in 0.16s; `go run -race` + 6 concurrent sessions → 0 races;
`go test -short` 0.35s. The fixes from GAP-003 through GAP-026 are real and
observable (404s, completed status, cancelled_decision_id, DELETE-as-removal,
504 JSON timeout).

**What was still wrong (new findings, board GAP-027..GAP-030):**

| Finding | Live evidence | Lesson |
|---|---|---|
| GAP-027 (P1): panic recovery returns `500 text/plain "internal server error"` (middleware `recover()` → `http.Error`), not the JSON `ErrorResponse` the protocol mandates; `api-reference.md:478` claims "every error response, all endpoints" is JSON while L216/L245 document the plain text | `curl POST /v1/process {"content":"panic now"}` → `HTTP/1.1 500`, `Content-Type: text/plain`, body `internal server error` | The battery can only drive compliant harnesses — it can never make a harness panic, so recovery-path contract breaks ship green. GAP-008 taught the same lesson for timeouts; the fix is the same: `writeError(500, INTERNAL_ERROR)`. **Defined-but-unused error codes (`ErrInternalError`) mark unimplemented contract paths.** |
| GAP-028 (P2): cancel sets status `cancelled`, but a late `result` executes `OnResult` and the end-transition overwrites status to `completed` (+turn_count) | process → cancel → late result → GET session: `status "completed"`, `turn_count 2` | Cancel must be terminal in the status machine; late in-flight results may run the harness but must not rewrite lifecycle state. |
| GAP-029 (P3): `testbed.MockHermes` has no `recover()` — a panicking harness crashes `go test` with a raw goroutine dump | had to wrap the call in `recover()` in my own test | The advertised "unit-test with MockHermes" workflow needs a guardrail; surface panics as errors. |
| GAP-030 (P3): §3.8 above says "Today that means knowing: status stays active after end…" — all four listed behaviors are FIXED (verified live this run) | status `completed`, `current_decision` populated, 404s, 504 JSON | Dated historical sections are fine, but "Today" claims must track the fixes; same class as GAP-015 (skill refresh). |

**The right way (updated 2026-08-18):** the published path works —
`go get github.com/get-h3/sdk-go@latest` resolves v0.1.2 and is fully
compliant. Use `protocol.NewDecision(type)` / `protocol.GenerateUUID()`, echo
history on every decision, guard shared state with `sync.Mutex`, gate with
`h3-test` (0.16s), then probe beyond the battery: panic (GAP-027), cancel-then-
result (GAP-028), and unit-test panics with a manual recover (GAP-029) until
the testbed grows one.

---

## 6. Dogfood run 2026-09-02 (published v0.1.5; first live run of the async pattern)

**How this run differed:** three prior runs (08-08, 08-18, 09-01) had beaten the
main path, error contract, and concurrency story into shape. The one documented
surface nobody had ever *executed* was the async pattern — the prescribed
answer to the fixed 30s timeout: spawn the slow work in a goroutine, return a
`wait` decision carrying `duration_seconds` + `poll_endpoint`, and let Hermes
either poll or fire `wait_timeout` at the decision id you handed it. This run
built a real harness around exactly that (`h3-slowjobs`, report generation with
a 4s background job) and probed the error contract on the published module
rather than HEAD.

**What worked, live (full table: `2026-09-02-integration.md`):**

- The full wait → poll → re-arm → report cycle completes end-to-end, from both
  the poll path and the `wait_timeout` correlation path. Correlation is by
  `req.DecisionID` == the id you put on the wait decision — which is why the
  "always set explicit decision ids" rule exists.
- The error contract finally matches the docs *on the published module*:
  panic → 500 JSON INTERNAL_ERROR (GAP-027), 31s block → 504 JSON
  HARNESS_TIMEOUT at 30.05s (GAP-008), role:'system' → 400 (GAP-032), unknown
  route → 404 JSON (GAP-034), wrong method → 405 JSON (GAP-035). The
  "text/plain mux default" bug class that produced GAP-008/027/034/035 is dead.
- `NewHTTPServer` mounts as a `/v1/` subtree of a consumer-owned mux and passes
  45/45 there — its 404/405 interceptor is path-agnostic. Consumers do not need
  to dedicate a port's route table to H3.

**What went wrong and why (the instructive part):**

1. *My own first draft had a data race*: the background goroutine locked a
   per-job mutex while handlers locked the harness mutex. Mixed lock scopes are
   a race by design; `-race` caught it immediately. Lesson: the docs' "guard
   shared harness state with `sync.Mutex`" means ONE mutex for everything the
   goroutine and the HTTP handlers touch — per-entity mutexes are how you lose.
   This is also the predictable failure mode of a pattern with no runnable
   example (GAP-042): consumers will invent it and make this exact mistake.
2. *Raw curls 400'd on `identity.platform`/`identity.chat_id`* — required
   fields, but no copy-pasteable curl exists above api-reference §2 (GAP-040).
3. *testbed cannot inject history* (`SendMessage` hardcodes `DefaultContext()`),
   so the history-passthrough pattern can only be unit-tested by driving
   `OnProcess` raw (GAP-039).
4. *Release drift, 5th recurrence* (GAP-038): v0.1.5 is 16 commits behind HEAD.
   Benign this time — the diff is docs/board/CI, zero wire changes — but the
   tag-on-every-green-tick rule keeps not sticking. The run's consumer-path
   check is what detects this class; it cost 5 minutes here and has cost a
   broken wire contract before (GAP-025/031/036).

**The right way for async work (proven):**

```go
// OnProcess: start the work, hand back a wait decision
j.waitID = protocol.GenerateUUID()
go func(j *job) { /* slow work; lock the HARNESS mutex when touching state */ }(j)
return &protocol.Decision{Decision: protocol.DecisionWait, DecisionID: j.waitID,
    Wait: &protocol.Wait{Reason: "...", DurationSeconds: intPtr(5), PollEndpoint: "/v1/process"}}, nil

// OnResult: Hermes fired the wait_timeout for that id
if req.Result.Type == protocol.ResultWaitTimeout {
    if j := byWaitID(req.DecisionID); j != nil {
        if j.done { return textDecision(j.report), nil }
        j.waitID = protocol.GenerateUUID()          // re-arm with a NEW id
        return waitDecision(j.waitID, j.reason), nil
    }
}
```

**Known limits (as of this run):** sessions remain in-memory (restart forgets
jobs — a real deployment persists its own job table); the timeout stays fixed
at 30s with no knob (by design, documented); the battery still cannot drive a
harness panic or a >30s block, so those probes stay manual.
