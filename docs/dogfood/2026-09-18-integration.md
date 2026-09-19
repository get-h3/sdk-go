# H3 Go SDK — Dogfood Integration Report (2026-09-18)

**Verdict: ✅ SHIPPABLE** (4th consecutive) — first dogfood run driven by a consumer
harness that does **real work with real side effects through a real `tool_call`
round trip**, on the currently published module (`v0.1.6`), plus the first
clean-room install leg since the bunker leg broke on 2026-09-05.

| | |
|---|---|
| Module under test | `github.com/get-h3/sdk-go@v0.1.6` (published; HEAD was `v0.1.6-6-g2bda31c`) |
| Consumer | `sentry` — repo-triage harness, `/tmp/dogfood-h3-2026-09-18/consumer` |
| Driver | `hermes-sim` — scripted Hermes that executes the harness's `tool_call` decisions **for real** |
| Battery | **46/46 PASSED** against the real consumer (0.66s) and against a fresh-installed `examples/echo` (0.22s) |
| Clean-room install | 20s (clone → `go build ./...` → `make all` → `make test`) on **Go 1.22.12 = the documented minimum** |
| Time to first success | ~25s from an empty dir to a live conformant harness (0.4s module fetch, 2.2s build, health 200 on the first poll) |
| Bunker leg | **SKIPPED-install-bunker** — new, root-caused blocker (see §5); substituted with a clean-room container |

---

## 1. The promise (null hypothesis)

> A Go developer can `go get github.com/get-h3/sdk-go`, implement the 5-method
> `Harness` interface, serve it with `harness.NewHTTPServer`, drive it from a
> Hermes body through the documented REST contract — including `tool_call`
> decisions where Hermes executes the tool and returns the result — and prove
> compliance with `h3-test` (46/46). The SDK is stdlib-only, the session store is
> in-memory, and the batteries' three decision contracts are on the consumer.

Prior runs proved the echo/text path, the async `wait` pattern, the `llm_call`
round trip and same-session concurrency. **Never proven before today:** a
consumer whose *point* is real work (filesystem-touching tools, a real artifact
written as the outcome) and that does it through `tool_call` → `tool_result` →
`end` from a live client.

## 2. What was actually built and run

`sentry` is a triage harness: it takes `triage <path>`, asks Hermes for
`git_log`, then `file_stats`, then writes `<path>/TRIAGE-REPORT.md` from the two
real results and ends. The scripted Hermes (`hermes-sim`) really shells out.

```go
// OnProcess — first decision is a tool call, not text.
if !haveTools { /* contract: never call a tool the request did not advertise */ }
return toolCall(hist, "git_log", map[string]any{"path": target, "limit": 5},
    "triage step 1: read recent history"), nil

// OnResult — the tool_result carries what the client actually executed.
case "git_log":
    return toolCall(hist, "file_stats", map[string]any{"path": target},
        "triage step 2: measure the tree"), nil
case "file_stats":
    os.WriteFile(target+"/TRIAGE-REPORT.md", []byte(report), 0o644) // real side effect
    return end(snapshot(s.history), protocol.EndTaskComplete, "triage complete -> "+path), nil
```

Live transcript (abridged, verbatim wire bodies):

```text
step 1 <- {"decision":"tool_call","decision_id":"f0d469f5-…","history":[…2 entries…],
           "tool_call":{"name":"git_log","params":{"limit":5,"path":"…/scratch-repo"},"reasoning":"…"}}
   executed git_log for real in 6ms -> c646993 extend handler / c23661e fix handler / ab4e3c5 initial service skeleton
step 2 <- {"decision":"tool_call","decision_id":"ba1ed4a0-…","tool_call":{"name":"file_stats",…}}
   executed file_stats for real in 15ms -> 8 files, 324K
step 3 <- {"decision":"end","decision_id":"fb5dfea8-…","end":{"reason":"task_complete",
           "summary":"triage complete -> …/scratch-repo/TRIAGE-REPORT.md"},…}

WORKFLOW_OK: end reason=task_complete
GET session -> 200 {…,"status":"completed","turn_count":3,"current_decision_type":"end"}
```

The artifact the harness wrote is real (`scratch-repo/TRIAGE-REPORT.md`, content
= the two tool outputs). **Nothing here is exercised by `go test`** — which is
exactly the point: the battery and the unit suite were green before this run,
and this run still found six things worth fixing.

### Battery against the real consumer (not a reference-shaped echo)

```text
Health & Protocol 7/7 · Process Basic Flows 8/8 · Decision Types 6/6 ·
Result Handling 7/7 · Error & Edge Cases 13/13 · Stress & Performance 5/5
TOTAL 46/46 PASSED   Duration 0.66s   p50/p95 1.89ms/81.06ms
```

A consumer with real branching (tools-driven, artifact-writing, error-ending)
passes the battery without contorting itself into the conformance shape. That is
the strongest statement anyone has made about this SDK's contract surface.

## 3. Lifecycle probes (all new this run)

| Probe | Result | Verdict |
|---|---|---|
| `POST /v1/cancel` while a `tool_call` is in flight | `{"cancelled":true,"cancelled_decision_id":"33cadd90-…"}`; session `status:cancelled`, `current_decision_type:tool_call` | ✅ matches docs |
| `POST /v1/cancel` for an **unknown** session | `404 SESSION_NOT_FOUND` | ✅ sane, but **undocumented** (§GAP-048 area) |
| `POST /v1/cancel` with **no `session_id`** (valid JSON) | `404 SESSION_NOT_FOUND "session not found: "` | 🔴 should be `400 INVALID_REQUEST` — **GAP-048** |
| `POST /v1/cancel` with an out-of-enum `reason` | `200 {"cancelled":true,…}` | 🟡 enum unenforced — **GAP-048** |
| `POST /v1/cancel` malformed JSON / empty body | `400 INVALID_REQUEST` (decode error text) | ✅ |
| `DELETE` then `GET` same session | `{"terminated":true}` → `404 SESSION_NOT_FOUND` | ✅ matches docs |
| Turn ending in `text.finished=true` | session stays `status: active` **forever** | 🟡 **GAP-051** (status semantics undocumented) |
| Duplicate `POST /v1/result` for the same `decision_id` | `OnResult` runs **again**, emits a second `tool_call` | 🔴 **GAP-049** (no correlation/dedupe) |
| `POST /v1/result` with a made-up `decision_id` | `200`, drives the loop forward | 🔴 same family — **GAP-049** |
| `GET /v1/health` | `uptime_seconds` **absent**, `active_sessions` only if the harness fills it | 🟡 **GAP-050** (docs promise §2/§8 shape) |
| `message.role: "system"` | `400 {"code":"INVALID_REQUEST","message":"message.role must be user"}` | ✅ reality; api-reference §2/§6 **still says "non-empty"** (GAP-041, still pending) |

## 4. Where the friction was (customer's-eye view)

1. **`/v1/result` accepts anything.** Not an error, but a silent footgun: a
   retry or a duplicated delivery runs the harness's tool path twice. Any
   consumer with side effects (this one writes a file) has to build its own
   idempotency. Nothing in `docs/` says results are uncorrelated or that
   delivery may be repeated. → **GAP-049**
2. **Validation is inconsistent across endpoints.** `/v1/process` is strict and
   well-documented; `/v1/cancel` answers a *missing required field* with a 404
   about a session named `""`. That cost me one confused re-read of the docs. →
   **GAP-048**
3. **The health sample in the docs is an ideal, not the contract.** Wiring
   `uptime_seconds` into a load balancer per §8 produces an absent field; the
   SDK has the data (`time.Now()` at store creation) and simply never fills it.
   → **GAP-050**
4. **Const identifiers for `ResultType` / `HistoryRole` are not in the API
   reference.** The JSON strings are (`"tool_result"`, `"text_sent"`, `"user"`),
   so writing the `OnResult` switch meant grepping `protocol/types.go` for
   `ResultTool` / `ResultTextSent` / `RoleUser`. "Had to read source to proceed"
   = a docs gap by this skill's rules. → **GAP-054**
5. **Session status semantics are undocumented.** `completed` (only via an
   `end` decision), `active`-after-finished-text, and an `expired` status that
   nothing can reach. Monitoring code has to guess. → **GAP-051**
6. **`make verify-counts` false-greens in a fresh clone.** In the clean-room
   container it printed
   `NOTE — no shim canonical count at /app/../shim/scripts/test-count.txt; battery parity skipped`
   and still exited **0** (canonical battery=46 asserted against nothing). The
   guard's stated job is "battery parity against the sibling shim checkout" — so
   it is weakest exactly where a fresh consumer or CI runs it. → **GAP-053**

Environment artifacts (mine, not the project's — recorded so nobody mis-blames
the repo): running `bash -lc` inside the `golang:1.22-bookworm` image drops
`/usr/local/go/bin` from `PATH`, so `go build` → `command not found` and
`make all` → `make: gofmt: No such file or directory` (Error 127). With a sane
`PATH` everything is green (§5). The README does not claim otherwise.

## 5. Installability leg

### 5a. Bunker (ephemeral, `las-bunker-03`) — SKIPPED, root-caused

The 2026-09-05 blocker (box had no external DNS) is **resolved**: `getent hosts
github.com` on the box now answers `140.82.114.4`, `bunkerd` is active, Docker
26.1.5 responds. The leg still could not run — for a *different*, now-proven
reason:

```text
bunker spawn --server bunker-las-03 --ttl 2h   → (2 attempts)
bunker: spawn agent: deadline_exceeded: context deadline exceeded   [client gives up ~45s]
bunkerd journal:
  {"msg":"installing rootless docker","user":"bunker-302c1c67"}
  {"msg":"rollback userdel failed","error":"context canceled"}
  {"msg":"spawn agent failed","error":"install rootless docker for bunker-302c1c67:
      run rootless installer as bunker-302c1c67: signal: killed"}
  POST …/SpawnAgent … - 500 13162B in 1m25.7s
```

The rootless-docker install itself **succeeds** (the logs show
`Active: active (running)` for `docker.service` and a full
`docker version` server block) — but it takes 60–90s while the client deadline
fires at ~45s, the server's spawn context is cancelled, and the rollback
(`userdel`) fails with `context canceled`. Net effect: **every failed spawn
leaks a `bunker-*` user**. `las-bunker-03` currently carries **16** leaked
`/home/bunker-*` homes (two of them from today's attempts). No `bunker spawn`
timeout flag exists in CLI 0.1.3 to work around it.

Filed as **GAP-052** (P2, infra): raise/parameterise the spawn deadline, make
rollback non-cancellable, and reap the leaked agents. The hard rule was
respected — no repo visibility/permission/clone setting was touched, and no
credential was minted.

### 5b. Substitute: clean-room container install (labelled as such)

`golang:1.22-bookworm` (the **documented minimum**, not the dev box's 1.26.5),
empty filesystem, documented commands only:

```text
git clone --depth 1 https://github.com/get-h3/sdk-go.git /app   → 2bda31c   (anonymous: repo cloneable)
go build ./...                                                  → BUILD_OK  8.369s
make all    → verify-counts PASS (canonical battery=46, suite=130) · fmt · vet · build · test-short all ok
make test   → ok harness 0.164s · protocol 0.004s · countguard 3.150s · testbed 0.003s
go list -m all → only github.com/get-h3/sdk-go; no go.sum  → "zero external dependencies" VERIFIED
INSTALL_SECONDS=20
```

Smoke, documented example + README curl quickstart verbatim:

```text
PORT=9191 go run ./examples/echo/   → health 200 after 2s
POST /v1/process  → {"decision":"text","decision_id":"echo-001","text":{"content":"Echo: hello from curl","finished":true}}
GET  /v1/sessions/curl-demo-1 → {…,"turn_count":1,"status":"active","current_decision_type":"text"}
POST /v1/result   → {"decision":"text","decision_id":"echo-002","text":{"content":"Result received: echo-001",…}}
DELETE …/sessions/curl-demo-1 → {"terminated":true,"session_id":"curl-demo-1"}
h3-test → 46/46 PASSED (0.22s, p50/p95 0.82ms/27.08ms)
```

The README's curl quickstart reproduces its own documented response bodies
exactly, and the integration guide's headline — *"Time to a verified 46/46
harness: under 10 minutes"* — measured **~25 seconds** end to end. Claim holds.

## 6. Reproduce

```bash
/usr/local/bin/sentry-harness            # or: cd consumer && PORT=9393 go run .
cd consumer && go run ./cmd/hermes-sim -target <repo>   # real tool loop
h3-test --endpoint http://127.0.0.1:9393                # 46/46
```

Source of both programs: `/tmp/dogfood-h3-2026-09-18/consumer/` (ephemeral);
the `OnProcess`/`OnResult` shapes are mirrored in
`skills/h3-sdk-go-usage/SKILL.md` → "tool_call round-trip recipe".

## 7. What this run leaves behind

- Board rows **GAP-048 … GAP-054** (4×P2, 3×P3) on
  `.coding-hermes/board/tasks.jsonl`, each with repro + pass criteria.
- `skills/h3-sdk-go-usage/SKILL.md` → v1.0.5: the tool_call round-trip recipe
  (the one decision type the repo still ships no example for), the new traps,
  and corrections to three now-stale bullets.
- `docs/dogfood/diagnostics.md` §8 — how the server is built, why `/v1/result`
  is uncorrelated, and the right way to write a side-effecting consumer.
- Evidence that the 09-01 dogfood P2s (5 rows) and GAP-041/042/045/046 remain
  pending — the board's own QA row (QA-H3-SDK-GO-FOREMAN-5) flags the
  accumulation; the fix cadence, not the finding supply, is the bottleneck here.
