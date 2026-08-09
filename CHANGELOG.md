# Changelog

All notable changes to the H3 Go SDK.

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
