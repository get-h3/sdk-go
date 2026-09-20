# GAP-051 — session status machine, verified live

Date: 2026-09-20
Repo: get-h3/sdk-go
Subject: `GET /v1/sessions/{id}` → `status`
Result: the transition table now in `docs/api-reference.md` §2 and
`docs/integration-guide.md` §6 reproduced exactly against a live harness

## Why this exists

GAP-051 documented an enum that was never a state machine: the doc listed four
values without saying which event produces each, and one of the four (`expired`)
has no assignment site anywhere in the SDK. A consumer building expiry
monitoring on the documented enum would wait forever.

The docs now carry the transitions. This file is the live evidence for them —
the Tier-2 judge on the doc-only commit correctly refused the claim without the
observed responses, so they are recorded here.

## How it was driven

    cd /home/kara/get-h3/sdk-go
    (PORT=9296 go run ./examples/echo/ > /tmp/echo.log 2>&1 &)
    curl http://127.0.0.1:9296/v1/health        # 200

All requests below are against that single listener on the non-default port
9296 (every example honours `PORT`; default is 9191).

### 1. `POST /v1/process` creates the session, status `active`

    $ curl -s -X POST http://127.0.0.1:9296/v1/process -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261","identity":{"platform":"test","chat_id":"c1"},
             "message":{"role":"user","content":"hello"},"context":{"history":[]}}'
    {"decision":"text","decision_id":"echo-001","text":{"content":"Echo: hello","finished":true}}

    $ curl -s http://127.0.0.1:9296/v1/sessions/verify-261
    {"session_id":"verify-261","started_at":"2026-09-20T14:27:00-05:00",
     "last_active":"2026-09-20T14:27:00-05:00","turn_count":1,"status":"active",
     "current_decision":"echo-001","current_decision_type":"text"}

### 2. A `text` decision with `finished: true` leaves the session `active`

Driving the result for that decision returns another `text` — the finished turn
does NOT complete the session. Status stays `active`, turn_count advances:

    $ curl -s -X POST http://127.0.0.1:9296/v1/result -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261","decision_id":"echo-001",
             "result":{"type":"tool_result","success":true}}'
    {"decision":"text","decision_id":"echo-002",
     "text":{"content":"Result received: echo-001","finished":true}}

    $ curl -s http://127.0.0.1:9296/v1/sessions/verify-261
    {"session_id":"verify-261",...,"turn_count":2,"status":"active",
     "current_decision":"echo-002","current_decision_type":"text"}

This is the exact case the old wording hid: `finished: true` ends the **turn**,
not the **session**.

### 3. An `end` decision sets `completed`

    $ curl -s -X POST http://127.0.0.1:9296/v1/result -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261","decision_id":"echo-002",
             "result":{"type":"tool_result","success":true}}'
    {"decision":"end","decision_id":"echo-end",
     "end":{"reason":"task_complete","summary":"Echo conversation complete"}}

    $ curl -s http://127.0.0.1:9296/v1/sessions/verify-261
    {"session_id":"verify-261",...,"turn_count":3,"status":"completed",
     "current_decision":"echo-end","current_decision_type":"end"}

### 4. `POST /v1/cancel` sets `cancelled`, and it is terminal

    $ curl -s -X POST http://127.0.0.1:9296/v1/process -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261-cancel","identity":{"platform":"test","chat_id":"c2"},
             "message":{"role":"user","content":"hi"},"context":{"history":[]}}'
    {"decision":"text","decision_id":"echo-001","text":{"content":"Echo: hi","finished":true}}

    $ curl -s -X POST http://127.0.0.1:9296/v1/cancel -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261-cancel","reason":"user_interrupt"}'
    {"cancelled":true,"cancelled_decision_id":"echo-001"}

    $ curl -s http://127.0.0.1:9296/v1/sessions/verify-261-cancel
    {"session_id":"verify-261-cancel",...,"turn_count":1,"status":"cancelled",
     "current_decision":"echo-001","current_decision_type":"text"}

A late result for the pre-cancel decision still executes the harness (it returns
`end`) but does NOT rewrite lifecycle state — GAP-028's terminal-cancelled
guarantee, re-observed here:

    $ curl -s -X POST http://127.0.0.1:9296/v1/result -H 'Content-Type: application/json' \
        -d '{"session_id":"verify-261-cancel","decision_id":"echo-001",
             "result":{"type":"tool_result","success":true}}'
    {"decision":"end","decision_id":"echo-end","end":{...}}

    $ curl -s http://127.0.0.1:9296/v1/sessions/verify-261-cancel
    {"session_id":"verify-261-cancel",...,"turn_count":1,"status":"cancelled",...}

### 5. `DELETE` removes the session; a following GET is 404

    $ curl -s -X DELETE http://127.0.0.1:9296/v1/sessions/verify-261-cancel
    {"terminated":true,"session_id":"verify-261-cancel"}

    $ curl -s -w '\nHTTP %{http_code}\n' http://127.0.0.1:9296/v1/sessions/verify-261-cancel
    {"error":{"code":"SESSION_NOT_FOUND","message":"session not found: verify-261-cancel"}}
    HTTP 404

### 6. `expired` is never observed

No sequence in this SDK produces `expired`: there is no TTL and no expiry timer,
so no request can reach it. `grep -rn 'SessionExpired' harness/` finds no
assignment — only the constant in `protocol/types.go` and its sibling error code.
The docs now say so explicitly; this note records that the negative was checked
rather than assumed.

## Cleanup

The listener started for this verification was killed and
`ss -tlnp | grep 9296` confirmed empty afterwards. No other process was touched.
