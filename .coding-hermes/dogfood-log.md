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
