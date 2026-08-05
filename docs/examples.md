# H3 Go SDK — Examples

A tour of the four example harnesses, what each demonstrates, and which one to
reach for.

## Comparison

| Example | Source | What it shows | 43/43 compliant? | When to use |
|---|---|---|---|---|
| `minimal` | [`examples/minimal/main.go`](../examples/minimal/main.go) | Smallest possible harness — fixed greeting text, ends on any result | ✔ (text-only loop) | Absolute starting point; skeleton for your own harness |
| `echo` | [`examples/echo/main.go`](../examples/echo/main.go) | Echo loop with streaming awareness (`do not finish`), history echo, result tracking | ✔ **Compliance reference** — same code as the README quickstart | Learning the loop contract; baseline for battery runs |
| `conformance` | [`examples/conformance/main.go`](../examples/conformance/main.go) + [`testbed/conformance.go`](../testbed/conformance.go) | Keyword-triggered full agent loop exercising **five of the six decision types** (`tool_call`, `llm_call`, `text`, `delegate`, `end`); `wait` is advertised in capabilities but never returned | ✔ (purpose-built for h3-test) | Demonstrating/validating the full protocol surface against the battery |
| `consensus` | [`examples/consensus/main.go`](../examples/consensus/main.go) | Real-world integration: H3 harness driving the Consensus REST API for multi-model deliberation | ✔ (with Consensus running; falls back to echo) | Template for connecting an external agent backend to H3 |

All four serve on `:9191` via `harness.NewHTTPServer` and can be battery-checked
the same way:

```bash
go run ./examples/<name>
# in another terminal:
h3-test --endpoint http://localhost:9191
```

## 1. minimal — the smallest compliant harness

`examples/minimal/main.go`

- `OnProcess` always returns a `text` decision: `"Hello from H3 Go SDK!"` with
  `Finished: true`.
- `OnResult` immediately returns `end` (`task_complete`).
- `OnCancel` / `OnSessionTerminate` / `Health` are no-ops / defaults.

**What it teaches:** the bare shape of the `Harness` interface — five methods,
two of which carry the loop. ~55 lines total. Because every decision carries its
payload and history never shrinks, it is already battery-compliant.

**Use when:** starting a new harness. Copy it, then grow `OnProcess`/`OnResult`
into your real logic.

## 2. echo — the compliance reference

`examples/echo/main.go`

- Echoes the user message back as `text` (`"Echo: <content>"`).
- **Streaming awareness:** a message containing `"do not finish"` sets
  `streaming = true`, and subsequent text decisions return `Finished: false` —
  the harness stays in the stream instead of ending.
- **History preservation:** echoes `req.Context.History` back verbatim in every
  decision — the battery's anti-shrink rule.
- Ends (`end` / `task_complete`) after the second result in normal mode.

This is the exact logic of the README quickstart (GAP-002 fixed the quickstart to
be battery-clean). It is the canonical answer to "what does a compliant harness
look like?"

**Use when:** you want the reference implementation, or you're debugging a
battery failure and need a known-good baseline to compare against.

## 3. conformance — the full protocol surface

`examples/conformance/main.go`

A 7-line server that wraps `testbed.NewConformanceHarness()` (defined in
[`testbed/conformance.go`](../testbed/conformance.go)). The harness dispatches on
message keywords:

| Message contains | Decision returned |
|---|---|
| `start a thought, do not finish` | `text` with `Finished: false` (streaming) |
| `final answer` / `finished` | `text` `"The answer is 42."` |
| tool keywords (`echo`, `search`, `lookup`, `noop`, `use`) | `tool_call` (first tool from `Context.Tools`) |
| model keywords (`model`, `run`) | `llm_call` (first model from `Context.Models`) |
| `delegate` / `sub-agent` / `summarise` / `spawn` | `delegate` |
| `done` / `end` | `end` (`task_complete`) |
| anything else | `text` echo, `Finished: true` |

`OnResult` returns text after successful results and forces `end` after 3 results
or on a failed/`error` result — the loop always terminates. `Health` advertises
all six decision types in `capabilities`.

**What it teaches:** the full agent loop (`tool_call` → result → `text` → `end`),
history threading through both methods, and the result-driven termination rules.
Note that the reference loop emits five decision types — `tool_call`, `llm_call`,
`text`, `delegate`, `end`; `wait` appears in `Health.capabilities` but is not
returned by the loop (the battery's Decision Types category likewise has no
`wait` test).

**Use when:** validating a fresh checkout against `h3-test` (it is the harness the
battery's own conformance behaviour is derived from, S04 §6), or when you need a
harness that can *demonstrate* `tool_call`/`llm_call`/`delegate`/`wait` paths
without wiring a real backend.

## 4. consensus — real-world integration

`examples/consensus/main.go`

A complete reference for connecting H3 to an **external agent backend**: it
drives the Consensus multi-model deliberation REST API.

- `OnProcess` creates a Consensus session, then returns a `tool_call` decision
  (`consensus_deliberate`) — the first step of the agent loop.
- `OnResult` receives the tool result, refines the deliberation (up to
  `maxTurns = 3` turns), then returns a `text` summary and `end`.
- `OnCancel` / `OnSessionTerminate` clean up the per-session Consensus state.
- Configuration via env vars: `CONSENSUS_URL` (default `http://localhost:8080`)
  and `CONSENSUS_API_KEY`.
- Resilience pattern: if Consensus is unreachable, `OnProcess` falls back to a
  `text` response instead of erroring the session.

```bash
CONSENSUS_URL=http://localhost:8080 go run ./examples/consensus
```

**What it teaches:** the full `tool_call` → result → `text` → `end` loop against
a real service, session-state bookkeeping (mutex-guarded map), external HTTP
client timeouts, env-var configuration, and graceful degradation.

**Use when:** you are integrating H3 with an external brain (Consensus, an LLM
gateway, a tool service) and want the canonical shape of that integration.

## Which one for which job?

1. **Brand-new harness** → start from `minimal`, grow it.
2. **Compliance questions / battery failures** → run `echo` and diff behaviour.
3. **Protocol surface demos / SDK smoke tests** → run `conformance`.
4. **External-agent integration** → study `consensus`, adapt the pattern.
