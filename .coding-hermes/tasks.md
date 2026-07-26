<!--
  ⚠️  BOARD FORMAT — coding-hermes-model-router v1.3 (2026-07-24)
  All tasks MUST use matrix format: | ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
  Before editing this file, load the skill: skill_view(name='coding-hermes-model-router')
  Validate: python3 ~/.hermes/scripts/validate-board-format.py .coding-hermes/tasks.md
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

**Routing Notes:** Project feature-complete. Board empty (active). Last active task PERF-ND-01 was already done (5 benchmarks existed). Scheduler: h3-sdk-go-foreman, CooldownS=1800, Enabled=true.

**Execution Order:** NEVER-DONE audit only (PERF-ND-01 already completed in prior tick).

|||**Escalation Conditions:** Project feature-complete, board empty. Scheduler CooldownS=1800 (30m). Last active task marked complete. Idle tick #17 (exceeded escalation threshold at tick #6). **ESCALATION ACTIVE — Tick #50 recommends Bane review for disable/archive.**||

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
| GITREINS-JUDGE | Configure LLM evaluator for commit quality review | Critical | 1 | — | DeepSeek V4 Flash |

||> Tick #38: NEVER-DONE 11-point audit — ALL PASS. Build/vet/tests/golangci-lint/gofmt/GitReins guard all clean. Go 1.26.5. Tests: 87/87 PASS (protocol 98.0%, harness 84.2%, testbed 81.0%). CI green (last cc4b66d ✅). GitReins dual-source check: all 4 tasks complete, 0 pending — no fabrication. Hilo: 94 edges, 18 files, useful. DuckBrain: namespace exists, list_keys connection error (intermittent, same as prior ticks). Git status: clean. 0 TODOs/FIXMEs. No outdated deps. Benchmarks present. Project feature-complete, board empty. **Scheduler API: CooldownS=1800** (NOT 43200 as prior board header claimed — board drift corrected). Idle tick #5. Escalation threshold: not yet exceeded (need >5).

|||| Tick #42 — 2026-07-26 08:45 UTC (DeepSeek V4 Flash)

|||| # | Gate | Result | Detail |
||||---|------|--------|--------|
|||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: board update Tick #41 |
|||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
|||| 3 | Tests | ✅ PASS | 87/87 PASS (protocol 98.0%, harness 84.2%, testbed 81.0%) |
|||| 4 | go vet | ✅ PASS | All packages clean |
|||| 5 | golangci-lint | ✅ PASS | 0 issues |
|||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (full suite) |
|||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful |
|||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
|||| 9 | Deps check | ✅ PASS | No outdated dependencies (only stdlib + uuid) |
|||| 10 | CI health | ✅ PASS | Last 3 runs all success ✅ (b3d352c Tick #41, 0d295fb Tick #40, 7a1d018 Tick #39). Last failure at f7939f9 (Jan 29 — historical, Hilo post-commit hook) |
|||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending. No board drift. |
|||| DB | DuckBrain | ✅ PASS | tick-42 key written (UUID `048a2067-bf03-49fe-aff2-237eeddf9a20`), list_keys now reachable — 23 keys in sdk-go namespace |

**Scheduler API** (GET /api/v1/projects): `h3-sdk-go-foreman`, Enabled=true, CooldownS=1800, Weight=10, Priority=8. Verified via curl.

**Verdict:** IDLE — Tick #42, Idle tick #9. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #9, threshold at 5). Recommend Bane review: disable foreman or archive project. Continuous idle ticks with no code changes are burning PAYG tokens on audit-only cycles. DuckBrain connectivity improved this tick (list_keys reachable).

||||| Tick #43 — 2026-07-26 14:19 UTC (DeepSeek V4 Flash)
|||||
||||| # | Gate | Result | Detail |
||||||---|------|--------|--------|
||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: db1a870 board update Tick #42 |
||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
||||| 3 | Tests | ✅ PASS | 87/87 PASS (protocol, harness, testbed). Examples: [no test files] (expected) |
||||| 4 | go vet | ✅ PASS | All packages clean |
||||| 5 | golangci-lint | ✅ PASS | 0 issues |
||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (no staged files) |
||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful |
||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
||||| 9 | Deps check | ✅ PASS | 0 outdated (only stdlib + uuid) |
||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (b3d352c→cc4b66d). No recent failures |
||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
||||| DB | DuckBrain | ⚠️ PARTIAL | tick-43 key written (UUID `dba93794-aaf4-40f0-bb99-0d298e8efb4a`). list_keys intermittent connection error persists (same as prior ticks) |
|||||
||**Scheduler API:** h3-sdk-go-foreman, Enabled=true, CooldownS=1800, Weight=10, Priority=8. Verified via script.

||||**Verdict:** IDLE — Tick #44, Idle tick #11. All 11 NEVER-DONE gates PASS. Project remains feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #11, threshold at 5). Recommending Bane review: disable foreman or archive project. 11 consecutive audit-only ticks burning PAYG tokens with no code changes. GitReins pipeline improved this tick (tier2 ai_eval stage added by INFRA-GR-05). DuckBrain: namespace write verified (tick-44 key saved).

|||||| Tick #45 — 2026-07-26 15:31 UTC (DeepSeek V4 Flash)
||||||
|||||| # | Gate | Result | Detail |
||||||---|------|--------|--------|
|||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: 09cac4b board update Tick #44 |
|||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
|||||| 3 | Tests | ✅ PASS | 87/87 PASS (harness, protocol, testbed). Examples: [no test files] (expected) |
|||||| 4 | go vet | ✅ PASS | All packages clean |
|||||| 5 | golangci-lint | ✅ PASS | 0 issues |
|||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (full suite) |
|||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful |
|||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
|||||| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
|||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (b3d352c→cc4b66d→tick #44). No failures |
|||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
|||||| DB | DuckBrain | ✅ PASS | tick-45 key written (UUID `efdf37fc-be31-4105-ac15-d30588a1edd9`). list_keys reachable after namespace switch|

|||||||| Tick #46 — 2026-07-26 11:04 UTC (DeepSeek V4 Flash)
||||||||
|||||||| # | Gate | Result | Detail |
|||||||---|------|--------|--------|
||||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: fc74574 board update Tick #45 |
||||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
||||||| 3 | Tests | ✅ PASS | 87/87 PASS (harness, protocol, testbed). All 3 packages clean |
||||||| 4 | go vet | ✅ PASS | All packages clean |
||||||| 5 | golangci-lint | ✅ PASS | 0 issues |
||||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (full suite) |
||||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful (flat SDK topology, all orphans expected) |
||||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
||||||| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
||||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (b3d352c→0d295fb→cc4b66d→Tick #41→Tick #37→Tick #35). No failures since Jan 29 |
||||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
||||||| DB | DuckBrain | ✅ PASS | tick-46 key written (UUID `8b562c7b-eefe-42a1-b86f-e8751b597639`). list_keys reachable — 10 keys in sdk-go namespace |
|||||||
||||||**Verdict:** IDLE — Tick #46, Idle tick #13. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #13, threshold at 5). 13 consecutive audit-only ticks burning PAYG tokens with no code changes. This tick confirms: nothing has changed, nothing is broken. Recommend Bane review: disable foreman or archive project. All quality gates remain green with zero effort.

|||||||||| Tick #47 — 2026-07-26 16:37 UTC (DeepSeek V4 Flash)
||||||||||
|||||||||| # | Gate | Result | Detail |
||||||||||---|------|--------|--------|
|||||||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: 9058759 board update Tick #46 |
|||||||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
|||||||||| 3 | Tests | ✅ PASS | 87/87 PASS (harness, protocol, testbed). All cached |
|||||||||| 4 | go vet | ✅ PASS | All packages clean |
|||||||||| 5 | golangci-lint | ✅ PASS | 0 issues |
|||||||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (no staged files) |
|||||||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful (flat SDK topology, all orphans expected) |
|||||||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
|||||||||| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
|||||||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (b3d352c→0d295fb→cc4b66d→bd5ca2a→05ee6e7f). No failures since Jan 29 |
|||||||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
|| DB | DuckBrain | ✅ PASS | tick-47 key written (UUID `5fefd3ca-d542-4f7d-bd77-77d9e2bfe3c0`). list_keys reachable — `/ticks/` prefix shows 2 entries |
||||||||||
|||||||||**Verdict:** IDLE — Tick #47, Idle tick #14. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #14, threshold at 5). 14 consecutive audit-only ticks burning PAYG tokens with no code changes. This tick confirms: nothing has changed, nothing is broken. Recommend Bane review: disable foreman or archive project. All quality gates remain green with zero effort.

|||||||||||| Tick #48 — 2026-07-26 18:00 UTC (DeepSeek V4 Flash)
||||||||||||
|||||||||||| # | Gate | Result | Detail |
||||||||||---|------|--------|--------|
|||||||||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: de40590 board update Tick #47 |
|||||||||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
|||||||||||| 3 | Tests | ✅ PASS | 87/87 PASS (harness, protocol, testbed). All 3 packages clean |
|||||||||||| 4 | go vet | ✅ PASS | All packages clean |
|||||||||||| 5 | golangci-lint | ✅ PASS | 0 issues |
|||||||||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (no staged files) |
|||||||||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful (flat SDK topology, all orphans expected) |
|||||||||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
|||||||||||| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
|||||||||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (de40590→b3d352c→0d295fb→cc4b66d→bd5ca2a). No failures since Jan 29 |
|||||||||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
|||||||||||| DB | DuckBrain | ✅ PASS | tick-48 key written (UUID `dcca158a-472d-4ade-b8c7-01736f16c22b`). list_keys reachable — 4 keys under `/ticks/` prefix |
||||||||||||
|||||||||**Verdict:** IDLE — Tick #48, Idle tick #15. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #15, threshold at 5). 15 consecutive audit-only ticks burning PAYG tokens with no code changes. This tick confirms: nothing has changed, nothing is broken. Recommend Bane review: disable foreman or archive project. All quality gates remain green with zero effort.

||||||||||||| Tick #49 — 2026-07-26 19:00 UTC (DeepSeek V4 Flash)
|||||||||||||
||||||||||||| # | Gate | Result | Detail |
|||||||||||||---|------|--------|--------|
||||||||||||| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: 7490d0d board update Tick #48 |
||||||||||||| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
||||||||||||| 3 | Tests | ✅ PASS | 87/87 PASS (harness, protocol, testbed). All 3 packages cached |
||||||||||||| 4 | go vet | ✅ PASS | All packages clean |
||||||||||||| 5 | golangci-lint | ✅ PASS | 0 issues |
||||||||||||| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (no staged files) |
||||||||||||| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful (flat SDK topology, all orphans expected) |
||||||||||||| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
||||||||||||| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
||||||||||||| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (7490d0d→de40590→b3d352c→0d295fb→cc4b66d). No failures since Jan 29 |
||||||||||||| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
||||||||||||| DB | DuckBrain | ✅ PASS | tick-49 key written (UUID `e9983267-9b5b-4f3d-952e-7c9c36c8b9cc`). list_keys reachable — namespace switch confirmed |
|||||||||||||
||||||||||||**Verdict:** IDLE — Tick #49, Idle tick #16. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #16, threshold at 5). 16 consecutive audit-only ticks burning PAYG tokens with no code changes. This tick confirms: nothing has changed, nothing is broken. Recommend Bane review: disable foreman or archive project. All quality gates remain green with zero effort.

| Tick #50 — 2026-07-26 19:XX UTC (DeepSeek V4 Flash)

| # | Gate | Result | Detail |
|---|------|--------|--------|
| 1 | Git status | ✅ PASS | Clean — no staged, untracked, or modified. Last commit: 98d647b board update Tick #49 |
| 2 | Build | ✅ PASS | `go build ./...` — 9 packages, 18 files, Go 1.26.5 |
| 3 | Tests | ✅ PASS | 87/87 PASS (harness 0.009s, protocol 0.006s, testbed 0.004s). All 3 packages cached |
| 4 | go vet | ✅ PASS | All packages clean |
| 5 | golangci-lint | ✅ PASS | 0 issues |
| 6 | GitReins guard | ✅ PASS | Secrets clean (gitleaks), guard PASS (full suite, no staged files) |
| 7 | Hilo graph | ✅ PASS | 94 edges, 18 files, useful (flat SDK topology, all orphans expected) |
| 8 | TODO/FIXME scan | ✅ PASS | 0 matches |
| 9 | Deps check | ✅ PASS | No outdated deps (only stdlib + uuid) |
| 10 | CI health | ✅ PASS | Last 5 runs all success ✅ (98d647b→7490d0d→de40590→b3d352c→0d295fb). No failures since Jan 29 |
| 11 | GitReins dual-source | ✅ PASS | 4 tasks all complete, 0 pending — no fabrication |
| DB | DuckBrain | ✅ PASS | tick-50 key written (UUID `4b8e634f-8a9e-4275-be85-690de2fd3ece`). list_keys intermittent connection error persists (write OK) |

**Scheduler API:** h3-sdk-go-foreman, Enabled=true, CooldownS=1800, Weight=10, Priority=8. Verified via curl.

**Verdict:** IDLE — Tick #50, Idle tick #17. All 11 NEVER-DONE gates PASS. Project feature-complete, board empty. **ESCALATION STILL ACTIVE** (idle tick #17, threshold at 5). 17 consecutive audit-only ticks burning PAYG tokens with no code changes. This tick confirms: nothing has changed, nothing is broken. Recommend Bane review: disable foreman or archive project. All quality gates remain green with zero effort.

> Tick #37: NEVER-DONE 11-point audit — ALL PASS.

> Tick #36: NEVER-DONE 11-point audit — ALL PASS. Build/vet/tests/lint all clean. Go 1.26.5. golangci-lint: 0 issues. CI green (last run cc4b66d ✅). GitReins guard PASS (full suite). Hilo: 94 edges across 18 Go files (useful). GITREINS-JUDGE verified configured (check-gitreins-judge.py PASS). DuckBrain: remember() succeeded (tick-36 key written), but read tools (list_keys/recall) show "Connection Error" — same intermittent transport issue as prior ticks. Board: GITREINS-JUDGE marked done. No gap tasks. Project feature-complete, board empty. Idle tick #3. Escalation counter: not yet at threshold (need >5). Scheduler: cooldown 43200s (12h).

> Tick #33: NEVER-DONE audit. All 11 gates pass. gofmt cleanup (3 files). Fixed: SECURITY.md + CODEOWNERS added, H3-ADAPTER-FIX committed (LastMessage field for consensus text relay). DuckBrain: 3 keys in sdk-go namespace. CI: 3 green runs. Project feature-complete, idle. Scheduler: cooldown 43200s.
> Committed: H3-ADAPTER-FIX (consensus adapter LastMessage), docs (SECURITY.md, CODEOWNERS), board update.
