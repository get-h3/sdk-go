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
