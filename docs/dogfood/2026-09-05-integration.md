# Dogfood Integration Report — 2026-09-05

**Target:** `github.com/get-h3/sdk-go` **published v0.1.5** (no replace directive)
**Consumer:** multi-model deliberation harness — a workflow no shipped example covers
**Battery:** h3-test 45/45 (0.33s) · **Workflow:** live llm_call round trip WORKFLOW_OK
**Time-to-first-success:** ~5 min to a compiling harness (README quickstart pattern);
2 battery iterations to 45/45 — both failures were undocumented contract violations (GAP-044)

## Promise tested

> A Go developer can build an H3-compliant agent harness from the published
> module (`go get github.com/get-h3/sdk-go@latest`), including workflows where
> the harness asks *Hermes* for LLM completions (`llm_call`), and pass the
> h3-test battery.

## What was built (real consumer, outside the repo)

`/tmp/dogfood-h3-sdk-go-2026-09-05/`:

| File | What |
|---|---|
| `main.go` | `deliberator` harness: process → `llm_call` model A → result → `llm_call` model B (critique) → result → synthesis `text` → delivered → `end`. Plus streaming mode ("do not finish" → `finished=false`), plain-text fallback mode, per-session persistent history, cancel/terminate cleanup. |
| `mini_hermes.py` | Scripted fake-Hermes client: drives the full loop over REST, asserts the VERDICT text, checks session status + terminate → 404. |
| `concurrent_check.py` | 6 parallel clients, distinct chat ids, one harness — concurrency probe. |

Module wiring: `go mod init dogfood-consensus && go get github.com/get-h3/sdk-go@v0.1.5`
— cold `GOMODCACHE`, **2 seconds**, zero transitive dependencies (stdlib-only SDK holds).

## Errors hit (this is the valuable part)

1. **`no_models_available` battery failure (44/45 on first run).**
   My first draft returned `llm_call` with a hardcoded model regardless of
   `context.models`. Battery test 5_8 sends `models=[]` and calls that a
   *hallucinated model*. This contract is written **nowhere** in README or docs/
   — only in `test_battery.py` and (by example) `testbed/conformance.go`. → GAP-044.
2. **`process_preserves_history` + `process_text_finished_false` failures (43/45).**
   Returning `History: nil` on some decisions made history "shrink"; one-shot
   `finished=true` text broke the streaming expectation. Fix (mirroring
   `testbed/conformance.go`): seed per-session history once from context, append
   every user turn, attach a snapshot to **every** decision including
   result-driven ones; treat "do not finish" as unfinished-text trigger.
   Same undocumented-contract class. → GAP-044.
3. **`assignment to entry in nil map` panic → 500.**
   The README pattern `&EchoHarness{}` (zero-value struct) does not survive
   adding state maps — forgot the constructor on the second iteration. The
   **panic-recovery middleware behaved exactly as documented**: clean JSON
   `INTERNAL_ERROR` 500 + log with stack, battery stayed green on error tests.
   Consumer-side lesson: stateful harnesses need a constructor.
4. **Same-session data race in the SDK (the headline finding).**
   The accidental first concurrent run shared one session id across clients
   (all launched in the same second → `delib-<unix>`): **5 `WARNING: DATA RACE`**
   reports inside SDK code — `resultHandler` closures write session fields
   (`harness.go:291/292` turn/llm counters, `harness.go:321/322` status) via
   `sessionStore.update()` while `getSessionHandler` reads them
   (`harness.go:382`) with no shared lock. With unique session ids: 6/6
   clients OK, **0 races**. So: distinct sessions are safe; concurrent
   requests targeting the *same* session are not. → GAP-043 (P1).

## Live evidence

- Battery on final consumer: `45/45 PASSED, 0.33s, p50 1.18ms` (ran twice).
- Workflow (`mini_hermes.py`): `process 200 llm_call → result → llm_call →
  result → text VERDICT finished=true → result → end task_complete`;
  `GET /v1/sessions/{id}` → `status: completed` (old GAP-DOG-003 is fixed);
  `DELETE` → 200; second `GET` → 404 SESSION_NOT_FOUND.
- Concurrency: 6 parallel clients `CONCURRENT_OK=6/6`, 0 race warnings.
- `go vet` clean; `go build -race` binary used for the concurrency leg.
- Repo gates: `go test ./... -short` ok (harness 0.135s, protocol, testbed);
  `go generate ./protocol/` exits 0 but is a **stub** (validates 15 schemas,
  generates nothing). → GAP-045.

## Install leg

- **SKIPPED-install-bunker:** bunker-las-03 (100.69.3.13) has lost external DNS
  — `getent hosts get.docker.com` and `github.com` both fail on the box; two
  spawn attempts died identically at rootless-docker install (`curl: (6) Could
  not resolve host: get.docker.com`); half-spawned agents 372ce4e0/bb7d5c09
  destroyed. Boarded as `SKIPPED-install-bunker` row.
- Substitute (control host, cold cache): `go get @v0.1.5` = 2s, battery 45/45
  from that module — the published artifact itself installs and works.

## Verdict

**SHIPPABLE** for the third consecutive run — with one new P1 (same-session
race), which matters for any harness whose Hermes client retries or shares
session ids across connections.
