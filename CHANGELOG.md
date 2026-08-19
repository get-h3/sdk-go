# Changelog

All notable changes to the H3 Go SDK.

## [0.1.4] — 2026-08-19

### Fixed
- `ProcessRequest.Validate()` enforces `message.role == "user"` per the protocol schema (`Message.role`: "Always user for /v1/process messages"). A request with any other role now gets `400` with `{error: {code: INVALID_REQUEST, message: "message.role must be user"}}` instead of a `200` echo. (GAP-032)
- Harness panic delivery in `withMiddlewareTimeout` is deterministic: the panic channel is drained before flushing in the `done` case, eliminating a select race that occasionally flushed an empty `200` instead of the `500` JSON `INTERNAL_ERROR` response (CI-only flake in `TestPanicRecovery_JSONErrorResponse`). (INT-CI-001)

## [0.1.3] — 2026-08-18

### Fixed
- Panic recovery honors the error contract: a panicking harness now gets `500` with a JSON `ErrorResponse` (`code: INTERNAL_ERROR`) instead of `text/plain "internal server error"`. (GAP-027)
- Cancelled sessions are terminal: a late `POST /v1/result` on a cancelled session no longer flips status back to `completed` or increments `turn_count`; the lifecycle state stays `cancelled`. (GAP-028)
- `testbed.MockHermes` recovers harness panics in all four driving methods (`SendMessage`/`SendResult`/`SendCancel`/`TerminateSession`) and surfaces them as errors instead of crashing the test binary with a raw goroutine dump. (GAP-029)

### Docs
- `docs/dogfood/diagnostics.md` §3.8 no longer instructs workarounds for bugs fixed since 2026-08-08 (session status, `current_decision*`, session validation, timeout shape) — it documents the current behavior. (GAP-030)

## [0.1.2] — 2026-08-15

### Fixed
- `DELETE /v1/sessions/{id}` now removes the session entry from the store (real map deletion); a subsequent `GET /v1/sessions/{id}` returns `404` instead of a stale record, stopping unbounded session retention. (GAP-014)
- Echo example harness guards shared state (`responseCount`, `streaming`) with a `sync.Mutex`, making it race-free under concurrent sessions. (GAP-021)
- README quickstart and integration-guide snippets guard shared harness state with `sync.Mutex`, matching the race-safe pattern now in the echo example. (GAP-023)

### Added
- CI: h3-test compliance battery job runs all 44 tests against the conformance example on `:9191` as a release gate. (GAP-019)
- CI: sync-protocol workflow copies schemas to `protocol/schemas/v1` (the `go:generate` path) instead of `sdk-go/schemas`. (GAP-018)
- `.gitignore` for local tool artifacts. (GAP-024)

### Docs
- `DELETE /v1/sessions/{id}` documented as session removal: subsequent `GET` returns `404`. (GAP-017)
- `cancelled_decision_id` documented as populated when a decision is in-flight at cancel time. (GAP-016)
- `skills/h3-sdk-go-usage` refreshed: battery count corrected to 44/44; traps section matches the fixed session lifecycle (GAP-009/DOG-002/DOG-003/014). (GAP-015)
- Dogfood battery count corrected from 43 to 44 across docs. (GAP-022)
- Compliance count aligned 43/43→44/44 (live h3-test verified), quickstart naming aligned to `EchoHarness`, stale `CRON_PAUSE_REQUESTED` removed. (GAP-011, GAP-012, GAP-013)

## [0.1.1] — 2026-08-09

### Fixed
- Cancel and session-delete responses now match the OpenAPI contract: `POST /v1/cancel` returns `{cancelled, cancelled_decision_id}`, `DELETE /v1/sessions/{id}` returns `200 {terminated, session_id}` (was `204` empty). (GAP-003)
- `POST /v1/cancel` and `POST /v1/result` return `404 SESSION_NOT_FOUND` for unknown sessions per h3-protocol.yaml. (GAP-DOG-002)
- Harness request timeout now returns `504` with a JSON `ErrorResponse` (`code: HARNESS_TIMEOUT`) instead of `503` text/plain. (GAP-008)
- Session lifecycle/observability: session status transitions to `completed` when a decision ends the session; `GET /v1/sessions/{id}` populates `current_decision`/`current_decision_type`; `POST /v1/cancel` returns the interrupted `cancelled_decision_id`. (GAP-009, GAP-DOG-003)
- `cmd/h3-consensus-adapter` imports `protocol` types from this module instead of duplicating them locally, restoring History preservation. (GAP-007)
- `protocol/schemas/v1/` JSON schemas shipped so `go generate ./protocol/` works. (GAP-005)

### Added
- `docs/`: integration-guide.md, api-reference.md, examples.md — new-user path to an h3-test-compliant harness. (GAP-004)
- GOVERNANCE.md.

### Docs
- AGENTS.md quickstart implements the full `Harness` interface and compiles; README quickstart is h3-test compliant. (GAP-001, GAP-002)
- Timeout behavior documented as `504` JSON `HARNESS_TIMEOUT` (was `503` text/plain). (GAP-DOG-001)

## [0.1.0] — 2026-07-19

### Added
- `protocol` package: Go types generated from H3 JSON Schema
- `harness` package: Harness interface + HTTP handler + middleware
- `testbed` package: MockHermes for unit testing harness logic
- Echo example harness (examples/echo/)
- Structured access logging (slog)
- GitReins quality gate
- Hilo code graph
