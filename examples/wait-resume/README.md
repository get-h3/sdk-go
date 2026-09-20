# wait/resume — the async pattern for >30s work

The harness middleware has a fixed 30s timeout, so long-running work must
not block `OnProcess`. This example returns a `wait` decision immediately,
finishes the work in a background goroutine, and hands the result back when
Hermes posts the `wait_timeout` result. The simulated job is ~3s so the demo
completes fast.

Run it (any example honors the `PORT` override, default 9191):

```bash
PORT=9293 go run ./examples/wait-resume/
```

## Consumer flow (curl steps)

### 1. Start the task

```bash
curl -s http://localhost:9293/v1/process -d '{
  "session_id": "sess-1",
  "identity": {"platform": "cli", "chat_id": "chat-1", "user_name": "hermes"},
  "message": {"role": "user", "content": "start job", "timestamp": ""},
  "context": {"history": [], "tools": [], "models": [], "config": {}, "session_state": {}}
}'
```

Response — a `wait` decision with a `decision_id` and a `poll_endpoint`:

```json
{"decision":"wait","decision_id":"wait-001","wait":{"reason":"long-running work started in background","duration_seconds":3,"poll_endpoint":"/v1/sessions/sess-1"}}
```

### 2. Wait past `duration_seconds`, then post the timeout result

```bash
sleep 4
curl -s http://localhost:9293/v1/result -d '{
  "session_id": "sess-1",
  "decision_id": "wait-001",
  "result": {"type": "wait_timeout", "success": false}
}'
```

Response — the completed work as finished text:

```json
{"decision":"text","decision_id":"text-001","text":{"content":"background job wait-001 completed: computed result-1729","finished":true}}
```

If you post before the work finished, the harness returns another short
`wait` decision instead of blocking — poll and repost the timeout.

### 3. Post the text-delivery result to end the task

```bash
curl -s http://localhost:9293/v1/result -d '{
  "session_id": "sess-1",
  "decision_id": "text-001",
  "result": {"type": "text_sent", "success": true}
}'
```

Response:

```json
{"decision":"end","decision_id":"end-001","end":{"reason":"task_complete","summary":"wait/resume task complete: background result delivered"}}
```

## Concurrency notes

- The background goroutine stores its outcome in a mutex-protected map, so
  the result is safe whether it lands before or after the consumer posts
  `/v1/result`.
- The handler never blocks on the work — blocking is what trips the 30s
  middleware timeout this pattern exists to avoid.
