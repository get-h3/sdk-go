# Dogfood Log

Real-use field tests of this project. Each run records date, verdict, the
promise tested, top findings, and time-to-first-success.

---

## 2026-08-08 — PROMISING-BUT-ROUGH

- **Verdict:** 🟡 PROMISING-BUT-ROUGH
- **Promise tested:** "A Go developer can build an H3-compliant agent harness
  in under 10 minutes: implement 5 methods, serve with
  `harness.NewHTTPServer`, pass `h3-test` 43/43."
- **What was done:** Built a units-converter assistant harness from scratch in
  `/tmp/dogfood-h3-sdk-go` (OUTSIDE the repo) following only
  `docs/integration-guide.md` + `docs/api-reference.md`. Full agent loop
  (tool_call → tool_result → text → end), streaming text, session lifecycle.
  Verified with `h3-test` (43/43, 0.24s, exit 0), `testbed.MockHermes` unit
  tests (3/3), README-quickstart regression build (OK), `go test ./...` (0.45s),
  `go vet` (clean).
- **Time-to-first-success:** ~15 min (docs read → 43/43 harness).
- **Friction count:** 4 (1 doc-drift trap, 1 false cancel ack, 1 status lie,
  1 ghost-session result; plus GAP-009 live-confirmed).
- **Top findings:**
  1. GAP-DOG-001 — timeout docs drift: docs say 503 text/plain, impl returns
     504 JSON HARNESS_TIMEOUT (GAP-008 fix never reached the docs).
  2. GAP-DOG-002 — POST /v1/cancel on unknown session returns 200, OpenAPI
     defines 404 SESSION_NOT_FOUND.
  3. GAP-DOG-003 — session status never becomes "completed" after an `end`
     decision; OpenAPI promises "active or completed".
- **Left behind:** docs/dogfood/2026-08-08-integration.md,
  docs/dogfood/diagnostics.md, skills/h3-sdk-go-usage/SKILL.md, board tasks
  GAP-DOG-001..003, events 50-52.
- **Foreman:** active (CooldownS 7200 < 14400) — no wake needed. Tasks picked
  up on next tick.

## 2026-08-18 — PROMISING-BUT-ROUGH (published-consumer path)

- **Verdict:** 🟡 PROMISING-BUT-ROUGH — one P1 wire-contract edge + 2 minor;
  all main-path promises held exactly.
- **Promise tested:** "A Go developer can build an H3-compliant agent harness
  in under 10 minutes from the PUBLISHED module: `go get
  github.com/get-h3/sdk-go@latest`, implement 5 methods, serve with
  `harness.NewHTTPServer`, pass `h3-test` 44/44."
- **What was done:** Fresh consumer module in /tmp/dogfood-h3-sdk-go-2026-08-18
  (NO replace directive — v0.1.2 tagged 2026-08-15). Built a reminders-assistant
  harness exercising all 6 decision types (text/tool_call/llm_call/wait/
  delegate/end), streaming, history passthrough, panic recovery, full session
  lifecycle. Verified: `go get` → v0.1.2; build/vet clean; testbed unit tests
  4/4; h3-test 44/44 (0.16s); 6 concurrent sessions under `go run -race` → 0
  races; all error paths (404 SESSION_NOT_FOUND ×3, 400 INVALID_REQUEST,
  DELETE→404, cancel→cancelled_decision_id) match OpenAPI; repo `go test
  -short` 0.35s; git clean. Prior fixes GAP-003..GAP-026 confirmed live.
- **Time-to-first-success:** ~15 min (docs read → 44/44 harness); the guide's
  "<10 min" claim is credible — the compliance-reference snippet is copy-paste.
- **Friction count:** 4 (1 P1, 1 P2, 2 P3).
- **Top findings:**
  1. GAP-027 (P1) — panic recovery returns 500 text/plain instead of JSON
     ErrorResponse; api-reference L478 "every error response" JSON contradicts
     L216/L245 plain-text; battery can't detect (never makes a harness panic).
  2. GAP-028 (P2) — late result after cancel flips status cancelled → completed
     (+turn_count); cancel isn't terminal in the status machine.
  3. GAP-029 (P3) — testbed.MockHermes has no recover(); panicking harness
     crashes `go test` raw.
  4. GAP-030 (P3) — diagnostics.md §3.8 lists fixed bugs as current ("Today").
- **Left behind:** docs/dogfood/2026-08-18-integration.md (full probe table +
  working example), docs/dogfood/diagnostics.md §5, skills/h3-sdk-go-usage
  SKILL.md v1.0.2 (3 new traps), board tasks GAP-027..GAP-030, board event 146.
- **Foreman:** cooldown 21600 ≥ 14400 — woken to 900 after adding work.
2026-09-01 | SHIPPABLE | 24s t2fs | friction 4 | 5 findings

## 2026-09-02 — SHIPPABLE (published-consumer re-check + first live async-pattern run)

- **Verdict:** ✅ SHIPPABLE
- **Promise tested:** build an H3-compliant harness from the PUBLISHED module
  (v0.1.5) and run the documented async pattern (goroutine + `wait` +
  `poll_endpoint`) end-to-end — never live-tested in the 08-08/08-18/09-01 runs.
- **What was done:** consumer module "slowjobs" in /tmp/dogfood-h3-sdk-go-2026-09-02:
  real background job (~4s) → wait decision with poll_endpoint → poll → report
  text; wait_timeout correlation + re-arm; panic/timeout/validation/route/method
  error-contract probes; subtree mux mount; MockHermes unit tests; go test -race
  (consumer race found + fixed; SDK race-free); battery 45/45 ×2 (0.51–0.54s).
- **Time-to-first-success:** ~6 min to a compiling, battery-passing harness
  (README-quickstart + NewDecision pattern); the curl workflow needed api-reference
  §2 (no request examples in README/quickstart — filed GAP-040).
- **Friction count:** 4 real frictions + 1 consumer-side race (mine, instructive).
- **Top findings:** GAP-038 release drift 5th recurrence (benign: docs/CI only);
  GAP-039 testbed history injection missing; GAP-040 no curl examples in
  README/quickstart; GAP-041 api-reference:117 stale role rule; GAP-042 no
  runnable wait/resume example.
- **Left behind:** docs/dogfood/2026-09-02-integration.md (full probe table +
  slowjobs source), docs/dogfood/diagnostics.md §6, skills/h3-sdk-go-usage
  SKILL.md v1.0.3 (async-pattern recipe + updated traps), board GAP-038..042.
- **Foreman:** cooldown 259200s — woken to 900 after filing work.
2026-09-05 | SHIPPABLE | ~5min t2fs | friction 4 | GAP-043..047 + SKIPPED-install-bunker | battery 45/45 | consumer=llm deliberation on published v0.1.5 | same-session race found (P1)

## 2026-09-18 — SHIPPABLE (real-work tool_call consumer + clean-room install leg)

2026-09-18 | SHIPPABLE | install_seconds=20 | bunker=las-bunker-03 spawn FAILED (GAP-052) → clean-room container golang:1.22-bookworm substitute | smoke=ok | t2fs ~25s | friction 6 | GAP-048..054

- **Verdict:** ✅ SHIPPABLE (4th consecutive)
- **Promise tested:** a consumer can do *real work* through the documented
  `tool_call` round trip (Hermes executes the tool, returns the result, the
  harness produces an artifact) on the currently published module `v0.1.6`,
  and a fresh user can install from the docs.
- **What was done:** consumer `sentry` in /tmp/dogfood-h3-2026-09-18/consumer
  (published module, no replace): `triage <path>` → `tool_call git_log` →
  `tool_call file_stats` → writes `TRIAGE-REPORT.md` → `end`, driven by
  `hermes-sim` (real `git log`, real `find|du`). Battery 46/46 on it (0.66s).
  Lifecycle + error probes: cancel in-flight / unknown / missing-session_id /
  bad reason, DELETE→GET, text-finished session status, duplicate + invented
  `decision_id` results, health shape, `role:"system"`.
- **Time-to-first-success:** ~25s (0.4s module fetch → 2.2s build → health 200
  on the first poll).
- **Friction count:** 6 (4 of them are board rows).
- **Top findings:** GAP-049 uncorrelated/non-idempotent `/v1/result` (double
  side effects); GAP-048 `/v1/cancel` missing session_id → 404 instead of 400;
  GAP-050 documented health fields never populated; GAP-052 bunker spawn
  deadline < rootless-docker install → leaked users (16 on the box).
- **Left behind:** docs/dogfood/2026-09-18-integration.md,
  docs/dogfood/diagnostics.md §8, skills/h3-sdk-go-usage/SKILL.md v1.0.5,
  board GAP-048..054.
- **Foreman:** NOT woken and cooldown NOT touched (fleet law: 21600s minimum).

## 2026-09-19 | PROMISING-BUT-ROUGH | t2fs ~10s (proxy) / bunker install ~27s build | friction 3 | 5 findings (DF-6..10, board commit 97be3ee)

Promise: fresh user runs `go get github.com/get-h3/sdk-go` and builds an H3-compliant harness
from the README quickstart in minutes.

Real use this run (first run via the PUBLISHED module path): consumer from the Go proxy
(v0.1.6, 0.5s to resolve), README quickstart builds+runs, full six-endpoint curl workflow
passed (health/process/session/result/cancel/delete; 400 INVALID_REQUEST on missing identity
and role!=user; 404 SESSION_NOT_FOUND on dead session; streaming finished=false held; cancel
marks status=cancelled but session stays usable), testbed.NewMockHermes roundtrip test green,
h3-test battery 46/46 PASSED 1.07s p95 180ms against the live quickstart. Bunker leg (agent
25366843, destroyed+verified): clean Debian 13, clone OK, toolchain had to be installed by
hand (README gap, DF-10), examples/minimal FAILS to build against published v0.1.6
(ListenAddr/Serve/PortEnv are HEAD-only — DF-6, the release-drift class now breaks real code),
after go mod init the quickstart binary passed the full smoke.

Top findings: DF-8 (P1) h3-test scores 36/46 FAIL on a dead server (100s, p95 10s) instead of
saying 'endpoint down'; DF-6 (P2) v0.1.6 cannot build examples/minimal; DF-7/9/10 docs gaps.
Verdict PROMISING-BUT-ROUGH: the library itself is excellent and compliant; the rough edge is
publishing discipline — 21 commits (7 wire-facing incl. GAP-048/049/050) unreleased past
v0.1.6 (GAP-056, 7th recurrence), and now the drift has broken an in-repo example for @latest
consumers.

- **Foreman:** NOT woken and cooldown NOT touched (fleet law: 21600s minimum; project at 43200s).
