# GAP-062 — curl walkthroughs use real server-assigned decision_ids, verified live

Date: 2026-09-22
Repo: get-h3/sdk-go
Subject: the README "Test it with curl" sequence and the integration-guide
"Test it with curl" section
Result: every documented step now passes AS WRITTEN end-to-end
(process → result → end → session `completed` → DELETE → 404), against the
quickstart harness the walkthrough tells readers to run.

## Why this exists

The walkthroughs quoted a literal `decision_id` of `"echo-001"` (README L164/L176/L187,
`docs/integration-guide.md` L194/L201). But the harness those docs print never sets
`DecisionID`, so the SDK server auto-generates one per decision (`harness/harness.go`
~L317/L422, H3 protocol §2.1). Following the docs verbatim, step 4's
`POST /v1/result` with `"decision_id":"echo-001"` therefore fails:

    {"error":{"code":"INVALID_REQUEST","message":"decision_id \"echo-001\" does not
    match the session's in-flight decision \"<uuid>\""}}   — HTTP 400

The docs now quote ids from one live run and say explicitly: `decision_id` is
**server-assigned** unless the harness sets `DecisionID` — copy the id from YOUR
own step response, never from the doc. This file is the live evidence.

Note: `examples/echo/main.go` DOES pin literal ids (`echo-001`/`echo-002`/`echo-end`,
added in 7450cc9 "for spec compliance") — the printed quickstart harness does not.
The walkthrough subject is the quickstart harness, so that is what was driven. The
quickstart block was extracted verbatim from README.md (the only change: the bind
port is `:9295`, because the default `:9191` was held by a sibling battery run; the
docs themselves document the `PORT` override, and `harness.ListenAddr()` honours it).

## How it was driven

    cd <worktree of get-h3/sdk-go @ 6f419a7>
    # main.go = README quickstart block, verbatim, bind :9295
    go run main.go &
    curl http://127.0.0.1:9295/v1/health        # 200 (first run compiles first)

Server-assigned ids observed in THIS run (all fresh UUIDs, all distinct):

    f4be6275-852e-465c-9829-8047d713704c   (step 2, text)
    474470b5-569f-438b-a1cc-880bc50929cc   (step 5, text)
    8811c181-bb72-407b-83e0-2b9ec47e47d6   (step 7, end)

## Observed sequence (verbatim transcript)

### 1. GET /v1/health

    $ curl -s http://127.0.0.1:9295/v1/health
    {"status":"ok","version":"1.0.0","transport":"rest","protocol_version":"1.0","uptime_seconds":48,"active_sessions":0,"capabilities":["text"]}
    HTTP 200

### 2. POST /v1/process (documented body)

    $ curl -s -X POST http://127.0.0.1:9295/v1/process \
        -H 'Content-Type: application/json' \
        -d '{"session_id": "curl-demo-1", "identity": {"platform": "telegram", "chat_id": "-1001234567890"}, "message": {"role": "user", "content": "hello from curl"}, "context": {"history": []}}'
    {"decision":"text","decision_id":"f4be6275-852e-465c-9829-8047d713704c","text":{"content":"Echo: hello from curl","finished":true}}
    HTTP 200

`decision_id` is server-assigned (the harness sets none).

### 3. GET /v1/sessions/curl-demo-1

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"session_id":"curl-demo-1","started_at":"2026-09-22T13:23:17-05:00","last_active":"2026-09-22T13:23:17-05:00","turn_count":1,"status":"active","current_decision":"f4be6275-852e-465c-9829-8047d713704c","current_decision_type":"text"}
    HTTP 200

### 4. The old doc-literal body now fails — negative proof of the gap

    $ curl -s -X POST http://127.0.0.1:9295/v1/result \
        -H 'Content-Type: application/json' \
        -d '{"session_id": "curl-demo-1", "decision_id": "echo-001", "result": {"type": "tool_result", "success": true}}'
    {"error":{"code":"INVALID_REQUEST","message":"decision_id \"echo-001\" does not match the session's in-flight decision \"f4be6275-852e-465c-9829-8047d713704c\""}}
    HTTP 400

This is exactly what a reader following the pre-fix docs got.

### 5. POST /v1/result with the real in-flight decision_id

    $ curl -s -X POST http://127.0.0.1:9295/v1/result \
        -H 'Content-Type: application/json' \
        -d '{"session_id": "curl-demo-1", "decision_id": "f4be6275-852e-465c-9829-8047d713704c", "result": {"type": "tool_result", "success": true}}'
    {"decision":"text","decision_id":"474470b5-569f-438b-a1cc-880bc50929cc","text":{"content":"Result received: f4be6275-852e-465c-9829-8047d713704c","finished":true}}
    HTTP 200

Note the echo: the harness quotes the id IT received, and the server stamps a
fresh id on the response.

### 6. GET /v1/sessions/curl-demo-1 (after first result)

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"session_id":"curl-demo-1","started_at":"2026-09-22T13:23:17-05:00","last_active":"2026-09-22T13:23:17-05:00","turn_count":2,"status":"active","current_decision":"474470b5-569f-438b-a1cc-880bc50929cc","current_decision_type":"text"}
    HTTP 200

### 7. POST /v1/result again — harness returns `end`, server assigns its id too

    $ curl -s -X POST http://127.0.0.1:9295/v1/result \
        -H 'Content-Type: application/json' \
        -d '{"session_id": "curl-demo-1", "decision_id": "474470b5-569f-438b-a1cc-880bc50929cc", "result": {"type": "tool_result", "success": true}}'
    {"decision":"end","decision_id":"8811c181-bb72-407b-83e0-2b9ec47e47d6","end":{"reason":"task_complete","summary":"Echo conversation complete"}}
    HTTP 200

### 8. Session is now `completed`

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"session_id":"curl-demo-1","started_at":"2026-09-22T13:23:17-05:00","last_active":"2026-09-22T13:23:17-05:00","turn_count":3,"status":"completed","current_decision":"8811c181-bb72-407b-83e0-2b9ec47e47d6","current_decision_type":"end"}
    HTTP 200

### 9. DELETE /v1/sessions/curl-demo-1

    $ curl -s -X DELETE http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"terminated":true,"session_id":"curl-demo-1"}
    HTTP 200

### 10. GET after DELETE — 404, as the docs promise

    $ curl -s http://127.0.0.1:9295/v1/sessions/curl-demo-1
    {"error":{"code":"SESSION_NOT_FOUND","message":"session not found: curl-demo-1"}}
    HTTP 404

Every step the docs document was executed with the documented request bodies and
returned the documented status codes, AS WRITTEN.

### Re-run on a fresh session (after the doc rewrite)

To prove the flow does not depend on the quoted ids, the full README sequence
and the integration-guide sequence were re-executed against a new session on
the same listener, copying the id from each run's own step response exactly as
the rewritten docs instruct (README steps 1–5, IG steps 1–3b):

    OK   README s1 GET /v1/health (HTTP 200)
    OK   README s2 POST /v1/process (doc body) (HTTP 200)
         copied decision_id from step-2 response: 35b1bcc4-901f-4cd9-8d25-7e223bea84ef
    OK   README s3 GET /v1/sessions/curl-demo-1 (HTTP 200)   [current_decision == copied id]
    OK   README s4 POST /v1/result (copied id) (HTTP 200)
    OK   README s5 DELETE /v1/sessions/curl-demo-1 (HTTP 200)
    OK   IG s1 POST /v1/process (doc body) (HTTP 200)
    OK   IG s2 POST /v1/result (copied id) (HTTP 200)
    OK   IG s3a GET /v1/sessions/curl-demo-1 (HTTP 200)
    OK   IG s3b DELETE /v1/sessions/curl-demo-1 (HTTP 200)
    ALL DOCUMENTED STEPS PASS AS WRITTEN

A different live run produced different server-assigned UUIDs and every step
still passed — which is the whole point of the rewrite.

## What changed in the docs

- `README.md` — walkthrough response/request ids replaced with the live ones
  (L164 response, L176 result body, L187 session view) + the server-assigned-id
  note after step 2.
- `docs/integration-guide.md` — same walkthrough, same treatment (process
  response comment, result body) + the server-assigned-id note after step 3.
- No harness/example source was touched.

## Cleanup

The listener started for this verification (`go run main.go` on :9295, plus the
`_gap062scratch/` extraction) was removed, and `ss -tlnp | grep 9295` confirmed
empty afterwards. No other process was touched; `:9191` was never used.
