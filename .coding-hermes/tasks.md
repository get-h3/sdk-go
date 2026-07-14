# Task Board — H3 Go SDK (`github.com/get-h3/sdk-go`)

## [x] INIT — Verify project structure, dependencies, and DuckBrain namespace (commit: TBD)
- [x] Verify repo structure: go.mod ✓, Makefile ✓, .gitignore ✓, AGENTS.md ✓, GitReins ✓
- [x] Verify package structure matches spec (S04 §2.1): protocol/ ✓, harness/ ✓, testbed/ ✓, examples/ ✓
- [x] Audit dependencies: no external deps needed (pure stdlib types); uuid per spec examples
- [x] Audit implementation state: ALL 8 .go files are package-level stubs — zero code
- [x] Audit DuckBrain namespace: `/project/sdk-go/` empty — needs seeding
- [x] Gap: README.md missing, .github/ missing (no CI)
- [x] Gap: Module path is `github.com/get-h3/sdk-go`; spec (S04 §2) uses `github.com/coding-herms/h3-sdk-go` — verify which is correct
- [x] Sister SDKs: Python (scaffold), TypeScript (partial) — Go SDK on par

## [x] SPEC — Audit API surface vs H3 spec, confirm 1:1 alignment (PHASE 1) (commit: TBD)
- [x] ✅ Protocol types vs JSON Schema v1: 100% coverage confirmed — 13 schema files → 22 Go types (ProcessRequest, Decision + 6 DecisionType enum values, ToolCall, LLMCall, TextResp, Wait, Delegate, End, ResultRequest, CancelRequest, HealthResponse, Message, Attachment, Identity, HistoryEntry, Tool, Model, SessionState, Config, Context, SessionResponse, ErrorResponse). All required fields, types, enums map 1:1.
- [x] ✅ Harness interface vs S04 §2.3: 5 methods confirmed — OnProcess, OnResult, OnCancel, OnSessionTerminate, Health. Return types and signatures match spec exactly.
- [x] ✅ HTTP endpoints: 5 handler registrations → 6 endpoints (GET /v1/health, POST /v1/process, POST /v1/result, POST /v1/cancel, GET+DELETE /v1/sessions/:id). sessionHandler covers both GET and DELETE on /v1/sessions/.
- [x] ✅ AGENTS.md quickstart: matches spec (NewHTTPServer → http.Handler, Decision types). Minor bug: OnProcess uses `req *protocol.ProcessRequest` but `protocol` package not imported in the quickstart snippet.
- [x] ⚠️ Module path: go.mod is `github.com/get-h3/sdk-go` (correct — matches actual repo). Spec S04 §2 uses `github.com/coding-herms/h3-sdk-go`. Repo name is canonical — spec needs update. Non-blocking for implementation.
- [x] ✅ DuckBrain seeded: 3 entries (protocol audit, type mapping, module path).

## [x] CORE-S01 — Implement protocol types from JSON Schema (PHASE 2) (commit: f295056)
- [x] ProcessRequest, Message, Attachment, Identity, Context (per schemas/v1/*.json ↔ S04 §2.2)
- [x] Decision + DecisionType enum (6 types: tool_call, llm_call, text, wait, delegate, end)
- [x] All sub-types: ToolCall, LLMCall, TextResp, Wait, Delegate, End
- [x] ResultRequest, CancelRequest, HealthResponse (per S02 spec)
- [x] HistoryEntry, Tool, Model, SessionState, Config (per common.json)
- [x] JSON tags on ALL fields matching wire format (snake_case)
- [x] `protocol/validate.go` — Validate() methods on ProcessRequest and Decision
- [x] Tests: JSON marshal/unmarshal round-trips for each type (18 tests, all pass)

## [ ] CORE-S02 — Implement harness interface + HTTP handler (PHASE 3)
Files: `harness/harness.go`, `harness/middleware.go`, `harness/harness_test.go`
- [ ] Harness interface (5 methods per S04 §2.3)
- [ ] NewHTTPServer(h Harness) → http.Handler (S04 §2.4)
- [ ] Endpoints: /v1/health, /v1/process, /v1/result, /v1/cancel, /v1/sessions/{id}
- [ ] JSON request unmarshalling + Decision validation + JSON response
- [ ] Middleware: request logging, panic recovery, timeout (per S04 §2.4)
- [ ] Tests: HTTP handler test with mock harness

## [ ] CORE-S03 — Implement testbed (MockHermes + assertions) (PHASE 4)
Files: `testbed/mock_hermes.go`, `testbed/assertions.go`, `testbed/*_test.go`
- [ ] MockHermes with SendMessage() and SendResult() (per S04 §6)
- [ ] Default tools/models/context helpers
- [ ] Assertion helpers: AssertDecisionType, AssertTextContent, AssertEndReason
- [ ] Tests: verify MockHermes works with a simple harness

## [ ] DOC-S01 — Create README.md + flesh out examples (PHASE 5)
Files: `README.md`, `examples/echo/main.go`, `examples/minimal/main.go`
- [ ] README.md: badges, install, quickstart, package structure, development
- [ ] examples/minimal: EchoHarness (matching S04 §2.5 and AGENTS.md quickstart)
- [ ] examples/echo: Echo + return text from OnResult

## [ ] CI-S01 — Set up GitHub Actions (PHASE 6)
Files: `.github/workflows/ci.yml`
- [ ] go build + go vet + go test on push/PR
- [ ] golangci-lint or staticcheck
- [ ] GitReins guard in CI
- [ ] Matrix: go 1.22, 1.23
