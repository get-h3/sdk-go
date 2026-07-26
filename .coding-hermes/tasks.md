<!--
  ⚠️  BOARD FORMAT — coding-hermes-model-router v1.3 (2026-07-24)
  All tasks MUST use matrix format: | ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
  Before editing this file, load the skill: skill_view(name='coding-hermes-model-router')
  Validate: python3 ~/.hermes/scripts/validate-board-format.py .coding-hermes/tasks.md
- [ ] **GITREINS-JUDGE — Configure LLM evaluator for commit quality review**
  | 🔴 Critical | — | — | deepseek-v4-flash @ deepseek-foreman | GITREINS_LLM_API_KEY in ~/.hermes/.env | foreman-direct |

  Run: `python3 ~/.hermes/scripts/check-gitreins-judge.py .` to verify.
  Default limits (adjust per-project based on codebase size and task complexity):
  - Fast/small projects: `max_iterations: 50`, `max_time: 10m`, tokens: `0.2M/0.4M`
  - Large repos (Go monorepos, 100+ files): `max_iterations: 100`, `max_time: 30m`, tokens: `1M/2M`
  - C++/Rust (slow compiles): `max_time: 30m` minimum
  - Scheduler/production infra: `max_time: 30m`, tokens: `1M/2M`
  Supervisor auto-flags projects where limits are too low for codebase size.

| 🔴 Critical | — | — | deepseek-v4-flash @ deepseek-foreman | GITREINS_LLM_API_KEY in ~/.hermes/.env | foreman-direct |

  Run: `python3 ~/.hermes/scripts/check-gitreins-judge.py .` to verify.
  If missing, create/edit .gitreins/config.yaml with evaluator section using deepseek-v4-flash.
  This is CRITICAL for code quality — no automated review of worker output without it.

  NEVER remove the matrix header row or NEVER-DONE / E2E-001 fixtures.
-->

# H3 Go SDK — Model Router Task Matrix

**Core purpose:** Go SDK for the H3 protocol — protocol types, harness interface, HTTP middleware, test bed (MockHermes), examples. Package: `github.com/get-h3/sdk-go`.

## Active Tasks

- [ ] **E2E-001 — E2E Testing Tick (self-improving loop)** 🔁 Every 5-10 ticks
  Spawn Luna (browser/screenshots) or Step 3.7 Flash (CLI/API). Deploy/build, Playwright, screenshots, endpoints, console. → e2e-output/tasks.md → inject into board.

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| NEVER-DONE | 11-point audit sweep | High | 2 | — | ++code-review, +testing | DeepSeek V4 Pro | Audit runs every tick | GLM-5.2 |

**Assumptions:** Go 1.25+. Module path confirmed: `github.com/get-h3/sdk-go`. All 47 tests pass (27 round-trip + 12 validation + 5 UUID + 3 structured-error). Build clean. No external deps (pure stdlib + uuid).

**Routing Notes:** Project feature-complete. Board empty (active). Last active task PERF-ND-01 was already done (5 benchmarks existed).

**Execution Order:** NEVER-DONE audit only (PERF-ND-01 already completed in prior tick).

**Escalation Conditions:** Project feature-complete, board empty. Cooldown at 43200s (12h). Last active task marked complete. Consider escalating to Bane for project disable/archive if idle persists >5 ticks.

## Completed

| ID | Task | Pri | Cpx | Commit | Model |
|----|------|-----|-----|--------|-------|
| INIT | Verify project structure, dependencies, DuckBrain namespace | High | 1 | — | DeepSeek V4 Flash |
| SPEC | Audit API surface vs H3 spec, confirm 1:1 alignment | High | 2 | — | DeepSeek V4 Pro |
| CORE-S01 | Protocol types from JSON Schema — 22 Go types, JSON round-trips (18 tests) | Critical | 5 | f295056 | DeepSeek V4 Pro |
| CORE-S02 | Harness interface — OnProcess, OnResult, OnCancel, OnSessionTerminate, Health | Critical | 4 | 4fc3e5b | DeepSeek V4 Pro |
| CORE-S03 | HTTP server — 5 handler registrations → 6 endpoints | Critical | 4 | — | DeepSeek V4 Pro |
| CORE-S04 | Middleware — request logging, error handling | High | 2 | — | DeepSeek V4 Pro |
| TEST-S01 | Test bed MockHermes + assertions | High | 3 | c6aba84 | DeepSeek V4 Pro |
| QV-SDK-01 | Structured validation errors (ValidationError struct) | High | 3 | cf78c8d | DeepSeek V4 Pro |
| QV-SDK-02 | Auto-generate decision_id when empty (UUIDv4) | High | 2 | 0f4d384 | DeepSeek V4 Pro |
| QV-SDK-05 | Cross-language wire format consistency verified | High | 2 | 8f1ae87 | DeepSeek V4 Pro |
| EXAMPLES | minimal, echo, conformance, consensus | Medium | 2 | — | DeepSeek V4 Pro |
| CI | GitHub Actions: build + test + lint on PR | Medium | 2 | — | DeepSeek V4 Flash |
| DOC-05 | Missing CONTRIBUTING.md added | Low | 1 | — | DeepSeek V4 Flash |
| QUAL-01 | 0 TODO/FIXME/HACK markers confirmed | Low | 1 | — | DeepSeek V4 Flash |
| PERF-ND-01 | Zero Go benchmarks — 5 Benchmark* functions | Low | 2 | 478643e | DeepSeek V4 Pro |

> Tick #35: NEVER-DONE 11-point audit — ALL PASS. Build/vet/tests/lint all clean. CI green (last run cc4b66d ✅). No TODO/FIXME markers. No untracked source files. No outdated deps. GitReins guard PASS (full suite). DuckBrain MCP transport intermittent — namespace exists (sdk-go) but read tools unreachable this tick. Project feature-complete, board empty. Scheduler: cooldown 43200s (idle tick #2).

> Tick #33: NEVER-DONE audit. All 11 gates pass. gofmt cleanup (3 files). Fixed: SECURITY.md + CODEOWNERS added, H3-ADAPTER-FIX committed (LastMessage field for consensus text relay). DuckBrain: 3 keys in sdk-go namespace. CI: 3 green runs. Project feature-complete, idle. Scheduler: cooldown 43200s.
> Committed: H3-ADAPTER-FIX (consensus adapter LastMessage), docs (SECURITY.md, CODEOWNERS), board update.
