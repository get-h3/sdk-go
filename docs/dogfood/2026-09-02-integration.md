# H3 Go SDK — Integration Report (2026-09-02)

**Run:** dogfood field test #4 of `github.com/get-h3/sdk-go` — published-consumer
path (`go get @latest` → v0.1.5, no `replace` directive) with a focus no prior
run had covered: the **async wait/resume pattern** (goroutine + `wait` decision
+ `poll_endpoint`), which `docs/api-reference.md` §3 and
`docs/integration-guide.md` prescribe for any work that would exceed the fixed
30s server timeout but which had never been executed live.

**Verdict:** ✅ SHIPPABLE — every main-path and error-path promise held on the
published module; new findings are docs/testbed ergonomics (GAP-038..042), none
block real use.

## What was built

**`h3-slowjobs`** — a report-generation harness that does a *genuine* slow job:
`report: <topic>` spawns a background goroutine (~4s of "analysis"), returns a
`wait` decision with `duration_seconds` + `poll_endpoint`; polls re-arm the
wait until the job completes, then the report is delivered as text — from the
poll path *and* from the `wait_timeout` correlation path. It also keeps the
battery-required intents (llm / delegate / remind-me / do-not-finish), a
`panic` trigger, and a `slow` trigger (31s block) so the timeout and panic
error contracts can be probed live. Mounting question answered too:
`NewHTTPServer(h)` is mounted as the `/v1/` subtree of a consumer-owned mux.

Full source (single file, works against v0.1.5 as-is):

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/protocol"
)

type job struct {
	mu      sync.Mutex
	topic   string
	done    bool
	report  string
	waitID  string // latest decision_id Hermes can correlate a wait_timeout to
	created time.Time
}

type ReportHarness struct {
	mu        sync.Mutex
	jobs      map[string]*job
	lastJobID string
	seq       int
	streaming bool
}

func (h *ReportHarness) OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error) {
	content := req.Message.Content
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.Contains(content, "panic") {
		panic("intentional panic for middleware probe")
	}
	if strings.Contains(content, "slow") {
		time.Sleep(31 * time.Second) // live-probe the fixed 30s timeout
	}
	h.streaming = strings.Contains(content, "do not finish")

	history := make([]protocol.HistoryEntry, len(req.Context.History))
	for i, e := range req.Context.History {
		history[i] = protocol.HistoryEntry{Role: e.Role, Content: e.Content}
	}

	switch {
	case strings.HasPrefix(content, "report:"):
		topic := strings.TrimSpace(strings.TrimPrefix(content, "report:"))
		h.seq++
		id := fmt.Sprintf("job-%d", h.seq)
		j := &job{topic: topic, waitID: protocol.GenerateUUID(), created: time.Now()}
		h.jobs[id] = j
		h.lastJobID = id
		go func(j *job) {
			time.Sleep(4 * time.Second) // the slow work
			h.mu.Lock()                 // ONE mutex for all shared state
			j.done = true
			j.report = fmt.Sprintf("Report on %q: [simulated 4s analysis]", j.topic)
			h.mu.Unlock()
		}(j)
		return &protocol.Decision{
			Decision:   protocol.DecisionWait,
			DecisionID: j.waitID,
			History:    history,
			Wait: &protocol.Wait{
				Reason:          "generating " + topic,
				DurationSeconds: intPtr(5),
				PollEndpoint:    "/v1/process",
			},
		}, nil
	// ... battery intents omitted for brevity (llm / delegate / remind-me /
	// do-not-finish exactly as in docs/examples.md conformance harness) ...
	case strings.Contains(content, "wait") || strings.Contains(content, "status"):
		if j, ok := h.jobs[h.lastJobID]; ok && j.done {
			return &protocol.Decision{
				Decision: protocol.DecisionText, DecisionID: protocol.GenerateUUID(),
				History: history,
				Text:    &protocol.TextResp{Content: j.report, Finished: !h.streaming},
			}, nil
		}
		return &protocol.Decision{
			Decision: protocol.DecisionWait, DecisionID: protocol.GenerateUUID(),
			History: history,
			Wait:    &protocol.Wait{Reason: "still working", DurationSeconds: intPtr(5), PollEndpoint: "/v1/process"},
		}, nil
	default:
		return &protocol.Decision{
			Decision: protocol.DecisionText, DecisionID: protocol.GenerateUUID(),
			History: history,
			Text:    &protocol.TextResp{Content: "say 'report: <topic>'", Finished: !h.streaming},
		}, nil
	}
}

func (h *ReportHarness) OnResult(req *protocol.ResultRequest) (*protocol.Decision, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if req.Result.Type == protocol.ResultWaitTimeout {
		for _, j := range h.jobs {
			if j.waitID == req.DecisionID { // correlation by decision id
				if j.done {
					return &protocol.Decision{
						Decision: protocol.DecisionText, DecisionID: protocol.GenerateUUID(),
						Text: &protocol.TextResp{Content: j.report, Finished: !h.streaming},
					}, nil
				}
				j.waitID = protocol.GenerateUUID() // re-arm with a fresh id
				return &protocol.Decision{
					Decision: protocol.DecisionWait, DecisionID: j.waitID,
					Wait: &protocol.Wait{Reason: "still generating " + j.topic, DurationSeconds: intPtr(5), PollEndpoint: "/v1/process"},
				}, nil
			}
		}
	}
	// ... tool_result / default end-handling as in docs ...
	return &protocol.Decision{
		Decision: protocol.DecisionEnd, DecisionID: protocol.GenerateUUID(),
		End: &protocol.End{Reason: protocol.EndTaskComplete, Summary: "slowjobs session complete"},
	}, nil
}

func (h *ReportHarness) OnCancel(req *protocol.CancelRequest) error { return nil }
func (h *ReportHarness) OnSessionTerminate(sessionID string) error  { return nil }

func (h *ReportHarness) Health() *protocol.HealthResponse {
	return &protocol.HealthResponse{
		Status: protocol.HealthOK, Version: "1.0.0", Transport: "rest",
		ProtocolVersion: "1.0",
		Capabilities: []protocol.DecisionType{protocol.DecisionText, protocol.DecisionToolCall,
			protocol.DecisionLLMCall, protocol.DecisionWait, protocol.DecisionDelegate, protocol.DecisionEnd},
	}
}

func intPtr(i int) *int { return &i }

func main() {
	h := &ReportHarness{jobs: map[string]*job{}}
	// NewHTTPServer mounts cleanly as a subtree of a consumer-owned mux:
	root := http.NewServeMux()
	root.Handle("/v1/", harness.NewHTTPServer(h))
	root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "slowjobs root: H3 API lives under /v1/")
	})
	log.Printf("h3-slowjobs listening on :9199")
	log.Fatal(http.ListenAndServe(":9199", root))
}
```

Consumer flow (exactly what a fresh user does):

```bash
mkdir slowjobs && cd slowjobs
go mod init slowjobs
go get github.com/get-h3/sdk-go@latest   # -> v0.1.5
go build ./... && go vet ./...           # clean
go run .                                  # serves :9199
h3-test --endpoint http://localhost:9199  # 45/45 PASSED (0.51–0.54s)
go test -race ./...                       # 0 races after fixing MY consumer bug
```

## Evidence table (every promise probed live, 2026-09-02)

| Promise (docs) | Probe | Result |
|---|---|---|
| `go get @latest` resolves a current module | fresh module | ✅ v0.1.5, builds clean (HEAD = v0.1.5-16; diff is docs/CI only — benign) |
| Quickstart → compiling harness | build + vet | ✅ ~6 min to first success |
| Async pattern: `wait` decision with `poll_endpoint` | POST process `report: quarterly` | ✅ `{"decision":"wait",...,"wait":{"reason":"generating quarterly","duration_seconds":5,"poll_endpoint":"/v1/process"}}` |
| Poll re-arms while job runs | immediate `status` poll | ✅ second wait decision, new id |
| Report delivered when job done | poll after 5s | ✅ text decision with report content |
| `wait_timeout` correlation + resume | POST result type=wait_timeout w/ original decision_id | ✅ report text returned; re-arm path also exercised via testbed |
| Battery passes on a subtree-mounted server | `root.Handle("/v1/", NewHTTPServer(h))` | ✅ 45/45 (0.51–0.54s) — the SDK mux makes no path assumptions |
| Panic → 500 JSON INTERNAL_ERROR (GAP-027 fix) | process "panic" | ✅ `500`, `Content-Type: application/json`, `{"error":{"code":"INTERNAL_ERROR",...}}` — on the PUBLISHED module |
| 31s block → 504 JSON HARNESS_TIMEOUT (GAP-008) | process "slow" | ✅ at 30.05s wall, `504 application/json` |
| Server survives panic + timeout | health after probes | ✅ serving, battery still 45/45 |
| Malformed JSON → 400 JSON | `{not json` | ✅ INVALID_REQUEST |
| role:system rejected (GAP-032 fix) | role:'system' | ✅ 400 `message.role must be user` |
| Unknown route → 404 JSON (GAP-034 fix) | GET /v1/nope | ✅ `{"error":{"code":"NOT_FOUND",...}}` |
| Wrong method → 405 JSON (GAP-035 fix) | POST /v1/health | ✅ `{"error":{"code":"METHOD_NOT_ALLOWED",...}}` |
| Full agent loop | remind-me → tool_call → tool_result → text | ✅ turn_count/decision tracking correct |
| Unit-test with MockHermes | 3 consumer tests | ✅ pass — but history injection impossible (GAP-039) |
| MockHermes surfaces panics (GAP-029 fix) | panicking SendMessage | ✅ returns error, test binary survives |
| Race-free | `go test -race -count=2` | ✅ SDK race-free; my mixed-mutex consumer draft DID race (see below) |

## Errors hit & their answers

1. **First two `POST /v1/process` curls → 400 `identity.platform is required`.**
   My payload had no `identity` block. The required fields are named in the
   troubleshooting tables, but no README/quickstart-level example shows a
   complete valid body (the full one lives in api-reference.md §2, ~100 lines
   deeper). **Finding GAP-040 (P2).**
2. **`bash` substitution mangled a curl payload** (`"…"` with `${VAR/…}`
   produced a JSON string instead of an object → 400 decode error). Not an SDK
   issue — but it underscores why a copy-pasteable curl example matters.
3. **`go test -race` FAIL on my first draft** — the background goroutine
   locked a per-job mutex while readers locked the harness mutex: mixed lock
   scopes = race by design. The SDK was blameless; the docs' "guard shared
   harness state with `sync.Mutex`" (ONE mutex for everything the harness
   touches) is exactly right. **This is the predictable failure mode of the
   undocumented async pattern → GAP-042 (P3, add examples/wait-resume/).**
4. **MockHermes can't inject history** — `SendMessage` hardcodes
   `DefaultContext()` (empty history), so the never-shrinking-history pattern
   the docs push as #2 can't be unit-tested through the documented API; my
   test had to drive `h.OnProcess(&protocol.ProcessRequest{...})` raw.
   **Finding GAP-039 (P2).**

## Friction count: 4 (+1 instructive consumer-side race)

| # | Friction | Severity | Task |
|---|---|---|---|
| 1 | Release drift 5th recurrence: v0.1.5 is 16 commits behind HEAD (benign: docs/CI only this time) | P2 | GAP-038 |
| 2 | Testbed history injection missing | P2 | GAP-039 |
| 3 | No curl/request-body examples in README/quickstart | P2 | GAP-040 |
| 4 | api-reference.md:117 role rule stale ("non-empty" vs must-be-"user") | P3 | GAP-041 |
| 5 | No runnable wait/resume example; consumers invent the concurrency | P3 | GAP-042 |

## The right way (updated 2026-09-02)

- **Async work:** goroutine + `wait` decision (`duration_seconds`,
  `poll_endpoint`) → correlate `OnResult(wait_timeout)` by `req.DecisionID`
  against the id you put in the wait decision → re-arm with a NEW id if not
  done, deliver text when done. **Use one harness-wide mutex** for all state
  the goroutine and the handlers touch.
- **Mounting:** `NewHTTPServer` under a prefix (`root.Handle("/v1/", ...)`)
  just works — 404/405 interception is path-agnostic (proven by 45/45 on the
  subtree mount).
- **Always `protocol.NewDecision(type)`** / explicit IDs — correlation logic
  depends on them.
- **Gate with `h3-test`** (fast, 0.5s) **then probe what it can't reach**:
  panic, >30s block, malformed bodies, wrong roles/methods/routes.

## Bottom line

Fourth run, and the first where *nothing* on the main path or the error
contract broke — including the async pattern that previous runs never executed.
The published v0.1.5 delivers a compliant harness in minutes and survives
hostile probes. Remaining work is ergonomics: an example for the async pattern,
testbed history injection, curl examples up front, and (as always in this repo)
the release-tagging discipline.
