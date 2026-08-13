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
  TerminateSession), `ConformanceHarness` (the 44/44 reference harness),
  `DefaultContext()/DefaultTools()/DefaultModels()` for fast unit tests.
- **`cmd/` + `examples/`** — `gen-types` generator; minimal/echo/conformance/
  consensus examples; `h3-consensus-adapter` (external-agent bridge, refactored
  onto SDK types in GAP-007).

Compliance is gated by **`h3-test`** (44 tests, 6 categories) from
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
   Today that means knowing: status stays `active` after `end`
   (GAP-DOG-003), `current_decision*` is never populated (GAP-009),
   cancel/result don't validate sessions (GAP-DOG-002), and the timeout docs
   are stale (GAP-DOG-001).

## 4. Known limits (as of 2026-08-08)

- Sessions are in-memory only; restart forgets everything (documented).
- Timeout fixed at 30s; no middleware configuration knobs.
- Session observability is incomplete: no `completed` status, no current
  decision tracking, empty `cancelled_decision_id` (GAP-009 + GAP-DOG-003).
- `POST /v1/cancel` and `/v1/result` accept unknown sessions (GAP-DOG-002).
- Docs lag the timeout implementation (GAP-DOG-001).
