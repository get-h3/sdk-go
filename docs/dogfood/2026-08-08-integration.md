# Dogfood Integration Report — 2026-08-08

**Project:** get-h3/sdk-go — Go SDK for building H3-compliant agent harnesses.
**Verdict:** 🟡 PROMISING-BUT-ROUGH.
**How this was produced:** an independent consumer harness was built OUTSIDE the
repo (`/tmp/dogfood-h3-sdk-go`), following only the public docs, then verified
against the compliance battery and the live HTTP contract. No internal helpers
were used.

---

## 1. What was built

A **units-converter assistant harness** (`conv`): parses
`convert <value> <from> to <to>` from the user message, emits a `tool_call`
decision, converts the `tool_result` into a text answer, streams when asked,
and ends the session. It exercises: `tool_call` decisions, result routing with
real data, streaming text (`finished: false`), history preservation, all three
end-reasons paths, session lifecycle, and the server timeout path.

Full working code: `/tmp/dogfood-h3-sdk-go/main.go` (also mirrored in
`docs/dogfood/diagnostics.md`).

## 2. The documented path, as executed

```bash
mkdir /tmp/dogfood-h3-sdk-go && cd /tmp/dogfood-h3-sdk-go
go mod init conv
go mod edit -replace github.com/get-h3/sdk-go=/home/kara/get-h3/sdk-go
go get github.com/get-h3/sdk-go      # works offline — SDK has zero deps
go build -o conv .                   # first try, no errors
./conv &                             # serves :9191
h3-test --endpoint http://localhost:9191
```

The `replace`-directive workflow in integration-guide §3 works exactly as
documented. The module's "zero external dependencies" claim is true
(`go.mod` has no requires).

## 3. Results (live evidence)

### 3.1 Compliance battery — the core promise

```
H3 Compliance Test Battery v1.0.0
Target: http://localhost:9191
Transport: REST

  Health & Protocol                   7/7  ✅ PASSED
  Process Basic Flows                 8/8  ✅ PASSED
  Decision Types                      6/6  ✅ PASSED
  Result Handling                     7/7  ✅ PASSED
  Error & Edge Cases                  11/11  ✅ PASSED
  Stress & Performance                5/5  ✅ PASSED
  TOTAL                               44/44  PASSED
  Duration                            0.24s
  Latency p50/p95                     0.77ms / 33.53ms
EXIT_CODE:0
```

**44/44 on the first run**, 0.24s. The battery is genuinely black-box (HTTP
only) and fast enough to run on every commit.

### 3.2 Full agent loop (curl, as Hermes would drive it)

```
POST /v1/process  {"session_id":"sess-1", "message":{"role":"user","content":"convert 10 km to miles"}, ...}
→ {"decision":"tool_call","decision_id":"305354f2-…","history":[{user hi},{user convert 10 km to miles}],
   "tool_call":{"name":"unit_convert","params":{"from":"km","to":"miles","value":10},...}}

POST /v1/result   {"session_id":"sess-1","decision_id":"305354f2-…","result":{"type":"tool_result",
                   "data":{"value":10,"from":"km","to":"miles","result":6.2137},"success":true}}
→ {"decision":"text",…,"text":{"content":"Answer: 10 km = 6.2137 miles","finished":true}}

POST /v1/result   {"session_id":"sess-1",…,"result":{"type":"text_sent","success":true}}
→ {"decision":"end",…,"end":{"reason":"task_complete","summary":"conversation complete"}}
```

History is preserved verbatim across the whole loop. UUIDv4 decision IDs are
generated when omitted (verified). Streaming works (`finished:false` until the
"do not finish" session is cancelled).

### 3.3 Error surface

| Probe | Result |
|---|---|
| Malformed JSON body | 400 `INVALID_REQUEST` with decode detail ✅ |
| Missing `session_id` | 400 `INVALID_REQUEST` "session_id is required" ✅ |
| GET unknown session | 404 `SESSION_NOT_FOUND` ✅ |
| DELETE unknown session | 404 `SESSION_NOT_FOUND` ✅ |
| POST /v1/cancel unknown session | **200 `{"cancelled":true,...}` — should be 404 per OpenAPI (GAP-DOG-002)** |
| POST /v1/result unknown session | 200 + decision executes (no 404 in contract; asymmetry noted in GAP-DOG-002) |
| Harness method hangs 35s | **504 JSON `{"error":{"code":"HARNESS_TIMEOUT",...}}` after exactly 30.03s ✅ impl; docs still say 503 text/plain (GAP-DOG-001)** |
| Server during hang | stays responsive (goroutine-based timeout writer works) ✅ |

### 3.4 Trust checks

- README quickstart copied verbatim into a fresh module → compiles, zero edits
  (GAP-001/GAP-002 fixes hold).
- `testbed.MockHermes` unit tests for the converter harness → 3/3 pass
  (the "test without HTTP" story works).
- `go test ./... -count=1` → 0.45s, `go vet` clean.

## 4. Friction log (in order hit)

1. **Timeout doc drift (GAP-DOG-001).** integration-guide.md L229/L245 and
   api-reference.md L214/L499 document `503 plain-text "request timeout"` via
   `http.TimeoutHandler`; api-reference L499 even claims the SDK server "does
   not emit" `HARNESS_TIMEOUT`. Live probe: the server returns `504` + JSON
   `HARNESS_TIMEOUT` (correct implementation, GAP-008 fix — the docs were never
   updated). A user debugging a slow harness would parse for the wrong shape.
2. **False cancel ack (GAP-DOG-002).** `POST /v1/cancel` with a stale session
   id returns 200 `cancelled:true` — the OpenAPI contract (h3-protocol.yaml
   L133-134) defines a 404 for exactly this.
3. **Status never "completed" (GAP-DOG-003).** After a full loop that ended
   with `end`, `GET /v1/sessions/sess-1` still reports `"status":"active"`.
   OpenAPI describes the endpoint as returning "metadata for an active **or
   completed** session".
4. **Ghost-session result executes.** `POST /v1/result` for an unknown session
   is accepted and executed (200 + decision). Folded into GAP-DOG-002.
5. **GAP-009 live-confirmed (already tracked).** Mid-decision `GET
   /v1/sessions/{id}` shows no `current_decision`/`current_decision_type`;
   `POST /v1/cancel` returns `cancelled_decision_id:""` even while a streaming
   decision was in flight.

## 5. What a new user should know (short version)

- The happy path is **fast and clean**: docs → 44/44 in ~15 minutes, zero
  dependency issues, first-build success.
- Use `protocol.NewDecision(type)` so every decision carries a UUID.
- Echo `req.Context.History` back verbatim or the battery's history tests fail.
- The server enforces: payload must match decision type, `text.content`
  non-empty, timeout fixed at 30s (do slow work in a goroutine + `wait`
  decision).
- Session state is in-memory and currently **unreliable as a source of truth**
  for "is this session done": status stays `active` after `end`
  (GAP-DOG-003), `current_decision*` is never populated (GAP-009). Key your
  own store by `req.SessionID` if you need real lifecycle semantics.
- Battery green ≠ full contract conformance: it checks status codes and key
  presence, not value semantics (this is how GAP-008/009/GAP-DOG-001..003
  slipped through). Probe error paths yourself.
