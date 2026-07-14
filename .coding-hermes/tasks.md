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

## [ ] SPEC — Audit API surface vs H3 spec, confirm 1:1 alignment (PHASE 1)
- [ ] Verify protocol types match JSON Schema v1 (schemas/v1/*.json) — 100% coverage
- [ ] Verify harness interface matches S04 §2.3 (Harness: 5 methods)
- [ ] Verify HTTP endpoints match S01 §4: /v1/health, /v1/process, /v1/result, /v1/cancel, /v1/sessions/:id
- [ ] Verify user-visible API from AGENTS.md quickstart matches spec
- [ ] Resolve module path: get-h3 vs coding-herms — confirm canonical name
- [ ] Seed DuckBrain `/project/sdk-go/` with spec decisions

## [ ] CORE-S01 — Implement protocol types from JSON Schema (PHASE 2)
Files: `protocol/types.go`, `protocol/types_test.go`
- [ ] ProcessRequest, Message, Attachment, Identity, Context (per schemas/v1/*.json ↔ S04 §2.2)
- [ ] Decision + DecisionType enum (6 types: tool_call, llm_call, text, wait, delegate, end)
- [ ] All sub-types: ToolCall, LLMCall, TextResp, Wait, Delegate, End
- [ ] ResultRequest, CancelRequest, HealthResponse (per S02 spec)
- [ ] HistoryEntry, Tool, Model, SessionState, Config (per common.json)
- [ ] JSON tags on ALL fields matching wire format (snake_case)
- [ ] `protocol/validate.go` — Validate() methods on ProcessRequest and Decision
- [ ] Tests: JSON marshal/unmarshal round-trips for each type

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
