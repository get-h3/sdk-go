
## Dogfood Findings (2026-09-18)

Verdict: SHIPPABLE (4th consecutive) — first real-work consumer: `sentry`, a repo-triage harness on published v0.1.6 that drives a real `tool_call` → `tool_result` → `end` loop through a scripted Hermes (real `git log`, real file stats) and writes a real `TRIAGE-REPORT.md`. Battery 46/46 against it (0.66s). Clean-room install on Go 1.22.12 (documented minimum): clone → `go build ./...` → `make all` → `make test` = 20s, all green, README curl quickstart reproduced verbatim, `go list -m all` = module only (zero-deps claim verified). t2fs ~25s from empty dir to a live conformant harness.

- [P2] GAP-048: `POST /v1/cancel` validates nothing — missing `session_id` (valid JSON) → `404 SESSION_NOT_FOUND "session not found: "` instead of `400 INVALID_REQUEST`; out-of-enum `reason` accepted (200). Sibling endpoints 400 correctly; api-reference §2/§6 has no cancel error row.
- [P2] GAP-049: `POST /v1/result` is uncorrelated and non-idempotent — any `decision_id` accepted (invented ids drive the loop) and re-delivering one id calls `OnResult` again (measured: two identical `file_stats` tool_calls from one repeated result). Double side effects for any side-effecting consumer; docs silent. Note for clients: omitting `result.success` decodes as `false` (our third probe took the EndError path purely for that reason).
- [P2] GAP-050: health fields the docs promise are never filled by the SDK — `uptime_seconds` (api-reference §2 sample, integration-guide §8 LB advice) is absent from a live `GET /v1/health`; grep `harness/` shows no reference. `active_sessions` only exists if the harness sets it.
- [P3] GAP-051: session status semantics undocumented and `expired` unreachable — `completed` only after an `end` decision; a turn ending `text.finished=true` stays `active` indefinitely; no TTL. Monitoring per §8 cannot tell finished from abandoned.
- [P2] GAP-052 (infra): SKIPPED-install-bunker, now root-caused. 09-05's DNS blocker is resolved (github.com resolves from the box); the blocker is the spawn deadline: CLI gives up at ~45s while the rootless-docker install needs 60–90s → server `signal: killed` mid-install → `rollback userdel failed: context canceled` → every failed spawn leaks a `bunker-*` user (16 accumulated on las-bunker-03, two from today). No timeout flag in CLI 0.1.3. Substitute clean-room container install used for the installability evidence (labelled as substitute).
- [P3] GAP-053: `make verify-counts` false-greens in a fresh clone — `battery parity skipped (no ../shim/scripts/test-count.txt)` then `PASS`; the parity half of the guard does not run exactly where CI/fresh consumers run it.
- [P3] GAP-054: api-reference omits the Go const identifiers (`ResultTool`/`ResultTextSent`/`RoleUser`/`SessionStatus`/`HealthStatus`) — writing the `OnResult` switch required grepping `protocol/types.go`.
- Still-open repeats (evidence, not new rows): GAP-041 (api-reference §2/§6 still say `message.role` "non-empty"; reality is `"must be user"`) and the five 09-01 dogfood P2s — QA-H3-SDK-GO-FOREMAN-5 already flags the pending-row accumulation.

Left behind: `docs/dogfood/2026-09-18-integration.md` (full report + working recipes), `docs/dogfood/diagnostics.md` §8 (server internals, why results are uncorrelated, session lifecycle table, install-leg mechanics), `skills/h3-sdk-go-usage/SKILL.md` v1.0.5 (tool_call round-trip recipe + 6 new traps + corrections to 3 stale bullets: GAP-040 shipped, GAP-043 fixed, const-name gap).

## Dogfood Findings (2026-09-05)

Verdict: SHIPPABLE (3rd consecutive) — first llm_call consumer workflow + first concurrency probe. Consumer: 2-model deliberation harness on published v0.1.5; battery 45/45; scripted-Hermes loop WORKFLOW_OK; 6 parallel clients clean with unique sessions.

- [P1] GAP-043: same-session data race in SDK harness — resultHandler writes session fields (harness.go:291/292, 321/322 via sessionStore.update) race getSessionHandler reads (harness.go:382); `go build -race` fired 5 reports when concurrent clients shared one session id; unique ids → 0 races, 6/6 OK.
- [P2] GAP-044: three battery-enforced decision contracts documented nowhere (models=[] forbids llm_call; "do not finish" = streaming mode; Decision.history must never shrink) — consumer failed 3 different tests across 2 iterations purely on these; rules live only in test_battery.py + testbed/conformance.go.
- [P2] GAP-045: cmd/gen-types is a JSON-validating stub, yet //go:generate + docs present protocol/types.go as generated — `go generate ./protocol/` exits 0 and generates nothing.
- [P3] GAP-046: no consumer example exercises the llm_call round trip (process → llm_call → result → … → end); this run's deliberation harness is a donatable examples/llm-roundtrip/ candidate.
- [P3] GAP-047: release drift 6th recurrence — v0.1.5 is 22 commits behind HEAD (GAP-038 measured 16 on 09-02).
- [P2] SKIPPED-install-bunker: bunker-las-03 lost external DNS (get.docker.com/github.com unresolvable from box; 2 spawns failed at rootless-docker install; agents 372ce4e0/bb7d5c09 destroyed). Substitute: cold-cache `go get @v0.1.5` = 2s, zero deps, battery green from that module. Infra follow-up: restore resolver on 100.69.3.13.

## Dogfood Findings (2026-09-01)
Verdict: SHIPPABLE
Promise: {"entry_point":"Go library/SDK (module github.com/get-h3/sdk-go, packages protocol/, harness/, testbed/) consumed via go get; the runnable artifact is your own harness main.go or the example HTTP harness servers (examples/echo, minimal, conformance, consensus) that serve the H3 REST API on :9191; co

- [P2] make lint can silently false-green — Makefile lint target is `golangci-lint run ./... 2>/dev/null || staticcheck ./... 2>/dev/null || echo "lint: no linter available"` — with neither linter installed it prints a message but exits 0, so a user sees green without a real lint pass. With golangci-lint present it ran clean (0 issues) per the dogfood run.
- [P2] Battery count drifts across repos (44 vs 45) — umbrella /home/kara/get-h3/AGENTS.md (lines 24, 52) and shim/AGENTS.md (lines 19, 24) say 44 tests; sdk-python/AGENTS.md and the SDK README say 45/45. Reality: test_battery.py contains exactly 45 test functions (test_1_1..test_6_5, incl. 5_9b) and h3-test output reports 45/45 — the 44 references are stale.
- [P2] README's headline 45/45 claim is not self-verifiable — README line 31 claims 'passes the full h3-test battery, 45/45' and line 153 references get-h3/shim, but the install+run steps (git clone shim, pip install -e ., h3-test --endpoint http://localhost:9191) appear only in docs/examples.md and integration-guide.md — a fresh consumer following the quickstart cannot verify the claim from the README alone.
- [P2] All four examples hardcode :9191 with no collision hint — minimal/main.go:55, echo/main.go:100, conformance/main.go:21, consensus/main.go:367 all ListenAndServe(":9191") — starting a second example while one runs dies with 'address already in use' and nothing in the README warns about it (confirmed by grep across examples/).
- [P2] First-run bind delay undocumented — First `go run ./examples/echo/` compiles ~10-12s before :9191 binds; the dogfood run hit HTTP:000 connection refused on early curls and the README gives no startup-time/readiness hint (works:true — transient only).

## Dogfood Findings (2026-09-02)

Verdict: SHIPPABLE (published-consumer re-check + first live run of the async wait/resume pattern)
Promise: a Go developer can build an H3-compliant agent harness from the published module (`go get github.com/get-h3/sdk-go@latest`), serve it with `harness.NewHTTPServer`, and pass the h3-test battery — including the documented async pattern (goroutine + `wait` decision + `poll_endpoint`) for work that would exceed the fixed 30s timeout.

What was done: fresh consumer module in /tmp/dogfood-h3-sdk-go-2026-09-02 on published v0.1.5 (no replace directive). Built "slowjobs" — a report-generation harness that spawns a real background job (~4s), returns `wait` with `poll_endpoint`, re-arms on `wait_timeout`, and delivers the report text when done. Also probed: panic recovery, 31s-block timeout, malformed JSON, role:system, unknown route, wrong method, subtree mux mounting, `testbed.MockHermes` unit tests, `go test -race` (consumer race found and fixed; SDK race-free), battery 45/45 twice (0.51–0.54s).

Live evidence: wait→poll→report completed end-to-end; wait_timeout correlation + re-arm works; panic → 500 JSON INTERNAL_ERROR (GAP-027 fix verified on published module); 31s block → 504 JSON HARNESS_TIMEOUT at 30.05s (GAP-008); role:system → 400 (GAP-032); unknown route → 404 JSON NOT_FOUND (GAP-034); wrong method → 405 JSON METHOD_NOT_ALLOWED (GAP-035); malformed JSON → 400 JSON. NewHTTPServer mounts cleanly under a custom `/v1/` subtree of a consumer mux.

Tasks filed (board `.coding-hermes/board/tasks.jsonl`):
- GAP-038 (P2): release drift again — v0.1.5 is 16 commits behind HEAD (benign: docs/CI only, no wire changes); 5th occurrence of the tagging class.
- GAP-039 (P2): MockHermes cannot inject conversation history (SendMessage hardcodes DefaultContext) — history-passthrough untestable via the documented testbed API.
- GAP-040 (P2): no curl/request-body examples in README or integration-guide — first two live process calls 400'd on identity fields; full body exists only deep in api-reference §2.
- GAP-041 (P3): api-reference.md:117 still says message.role "non-empty"; GAP-032 tightened it to must equal "user" (L512 already correct).
- GAP-042 (P3): documented async wait/resume pattern has no runnable example in examples/ — consumers must invent it (and will likely mix mutex scopes; this run's own draft did).


## Dogfood Findings (2026-09-19)
Verdict: PROMISING-BUT-ROUGH — library compliant and pleasant; publishing discipline is the blocker.
Install leg: BUNKER RAN (agent 25366843, destroyed+verified) — clone OK, toolchain hand-install needed (DF-10), examples/minimal build FAILED against v0.1.6 (DF-6), README-quickstart path PASSED end-to-end incl. bunker smoke.

- [P1] DF-H3-SDK-GO-FOREMAN-8: h3-test battery reports misleading 36/46 FAIL (100s, p95 10s) when the server under test is dead — no 'endpoint unreachable' signal; live server scores 46/46 in 1.07s.
- [P2] DF-H3-SDK-GO-FOREMAN-6: examples/minimal does not compile against published v0.1.6 (harness.ListenAddr/Serve/PortEnv are HEAD-only) — GAP-056 release drift now breaks real consumer code.
- [P2] DF-H3-SDK-GO-FOREMAN-7: examples/minimal ships without go.mod; fresh user must invent `go mod init` (undocumented).
- [P3] DF-H3-SDK-GO-FOREMAN-9: testbed.NewMockHermes(harness.Harness) mis-use pattern — passing the http.Handler is the natural first guess; SendMessage returns *protocol.Decision not bool; needs a usage snippet.
- [P3] DF-H3-SDK-GO-FOREMAN-10: README install section omits the Go toolchain prerequisite for clean machines.
