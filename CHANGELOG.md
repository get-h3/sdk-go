# Changelog

All notable changes to the H3 Go SDK.

## [0.1.7] — 2026-09-20

### Fixed
- `POST /v1/cancel` validates its request before touching the session store: a valid-JSON body missing `session_id` now returns `400` `{error: {code: INVALID_REQUEST, message: "session_id is required"}}`, and an out-of-enum or missing `reason` returns `400` naming the allowed values (`user_interrupt`, `timeout`, `system`) — instead of a phantom `404 SESSION_NOT_FOUND`. Validation happens before the lookup, so a malformed cancel can never read as a vanished session; a valid body with a live session still cancels (`200`), and an unknown session still gets `404 SESSION_NOT_FOUND`. (GAP-048)
- `POST /v1/result` correlates `decision_id` with the session's in-flight decision before `OnResult` runs: replaying the just-resolved decision id returns `400 INVALID_REQUEST` ("already been resolved for session"), and any other mismatched id returns `400 INVALID_REQUEST` ("does not match the session's in-flight decision") — closing the stale/duplicate/invented-decision-id hole. `ResultRequest.Validate()` now requires `session_id`, `decision_id`, and `result.type`. No new wire shapes were introduced. (GAP-049)
- `GET /v1/health` metrics are populated: `uptime_seconds` counts from process start and `active_sessions` reflects the live session-store size, instead of always reporting `0`. (GAP-050)

### Added
- Every example reads `PORT` and starts via `harness.ListenAddr()`/`harness.Serve()`: unset/empty/blank falls back to the default `9191`, and an already-bound port is reported with the address plus the `PORT` override hint instead of a bare listen error; conformance, consensus and minimal no longer hardcode `:9191`, and the consensus banner prints the resolved address. (`harness/listen.go` adds `PortFromEnv`, `ListenAddr`, `Serve`, `AddrInUseMessage`, `PortEnv`, `DefaultPort`.) (DF-H3-SDK-GO-FOREMAN-4)
- The count guard batches its per-file shell spawns into a single `grep` invocation, cutting `scripts/check-test-count.sh` runtime while preserving identical verdicts. (GAP-057)

### Docs
- AGENTS.md quickstart `main()` uses the `PORT`-aware `harness.ListenAddr()`/`harness.Serve()` pair, matching `examples/echo`.
- `POST /v1/result` decision-contract wording documented across the README, integration guide, and API reference. (GAP-044)
## [0.1.6] — 2026-09-18

### Fixed
- Concurrent requests that share a session id are serialized per session: `harness` returns a snapshot of the session entry taken under the read lock instead of handing callers the live pointer, and the unlocked read sites (`GET /v1/sessions/{id}`, the cancelled-decision read in `POST /v1/cancel`, the terminal-status check in `POST /v1/process`) go through it — removing the data race between a locked result write and an unlocked read on the same session. No response status, body, or header changed. (GAP-043)
- `testbed.MockHermes` can seed conversation history: `SendMessageWithHistory` and `ContextWithHistory` build the same request `SendMessage` builds with a populated `Context.History`, so the never-shrinking-history contract is testable through the documented testbed API instead of by hand-building a `ProcessRequest`. A nil or empty history behaves exactly like `SendMessage`. (GAP-039)
- `examples/echo` honors the `PORT` environment variable (default 9191) instead of hardcoding its listen address, and an already-bound port is reported with the address plus the `PORT` override hint instead of a bare listen error; every other listen error stays fatal with a non-zero exit. (DF-H3-9, DF-H3-15)

### Changed
- The CI battery gate asserts the battery's own `TOTAL … PASSED` summary plus a minimum-run strength floor instead of a pinned compliance-test total, so a shim that grows its suite no longer red-lines a build in which every test passed; the battery's exit code stays authoritative. (CI-SDKGO-001)
- The cross-language round-trip workflow fires on Go SDK source changes: `push`/`pull_request` triggers path-filtered to `harness/`, `protocol/`, `testbed/`, `cmd/`, `examples/`, `go.mod`, `Makefile` and the workflow file itself, so a wire-format-breaking source change can no longer ship with every workflow green; bookkeeping and prose paths are excluded. (GAP-073)

### Added
- `scripts/check-test-count.sh` (+ `scripts/test-count.txt`) and a `make verify-counts` target: the repo polices its own compliance/suite count prose — canonical counts → live suite parity → battery parity against the sibling shim checkout → stale-literal sweep with explicit historical exemptions — and CI runs it in the build job. The sweep previously lived only in the umbrella repo, where an SDK-only prose edit is never seen. (H3-GAP-087)
- `scripts/check-release-drift.sh` (+ `make release-drift`): reports how far `main` has moved past the published tag and how much of that is non-bookkeeping, so untagged wire-facing work is visible before the next `v0.1.x` tag is cut. Exits 0 unless an explicit `--fail-over` threshold is exceeded; CI runs it as an informational, non-blocking job. (GAP-038)

### Docs
- Compliance-count prose in `README.md`, `docs/` and CI comments swept to the current battery count, in two passes (the README/CI sweep and the `docs/` follow-up) so no surface still quoted the retired numbers. (GAP-045, GAP-037)

## [0.1.5] — 2026-08-27

### Fixed
- Unknown-route requests now return `404` with a JSON `ErrorResponse` (`code: NOT_FOUND`) instead of `text/plain "404 page not found"` — the mux default for unmatched paths previously bypassed the JSON error envelope, contradicting the documented "every error response, all endpoints" wire shape. (GAP-034)
- Wrong-method requests now return `405` with a JSON `ErrorResponse` (`code: METHOD_NOT_ALLOWED`) instead of `text/plain "Method Not Allowed"` — the not-found interceptor now traps `405` (and other text/plain mux defaults) in addition to `404`. (GAP-035)

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
