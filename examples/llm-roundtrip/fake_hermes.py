#!/usr/bin/env python3
"""Scripted fake-Hermes client for examples/llm-roundtrip.

Hermes is the party that EXECUTES decisions. This script stands in for it: it
drives the harness over REST, does what the harness asked for (calls the model
the `llm_call` decision named, in scripted fashion) and posts the answer back
through POST /v1/result. It asserts the contracts a consumer of the llm_call
round trip has to get right — the deliberation loop, the VERDICT text, history
that never shrinks, the streaming flip, the no-models fallback, the completed
session and the DELETE -> 404 teardown.

Usage (the port is overridable by argv or the PORT environment variable):

    python3 examples/llm-roundtrip/fake_hermes.py          # -> 127.0.0.1:9191
    python3 examples/llm-roundtrip/fake_hermes.py 9291     # explicit port
    PORT=9291 python3 examples/llm-roundtrip/fake_hermes.py

Start the harness first, on the same port:

    PORT=9291 go run ./examples/llm-roundtrip/

Exit code 0 = every assert held; 1 = a contract broke (the failing assert is
printed with the response body that broke it).

Stdlib only — no pip install, no HTTP library.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request

DEFAULT_PORT = "9191"
DEFAULT_HOST = "127.0.0.1"

# The two models this script "offers" Hermes. The harness must pick from these,
# never invent one.
MODEL_A = "fast-a"
MODEL_B = "fast-b"


class CheckFailure(AssertionError):
    """A protocol contract did not hold."""


def check(condition: bool, message: str, body: object = None) -> None:
    """Assert one contract, quoting the offending body when it breaks."""
    if condition:
        return
    detail = f"\n      body: {json.dumps(body)[:600]}" if body is not None else ""
    raise CheckFailure(f"{message}{detail}")


class FakeHermes:
    """Minimal REST transport for the H3 endpoints, standing in for Hermes."""

    def __init__(self, host: str = DEFAULT_HOST, port: str = DEFAULT_PORT) -> None:
        self.base = f"http://{host}:{port}"

    def request(self, method: str, path: str, body: dict | None = None) -> tuple[int, object]:
        """Return ``(status, decoded-body)``; never raises on 4xx/5xx."""
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(
            self.base + path,
            data=data,
            method=method,
            headers={"Content-Type": "application/json"} if data else {},
        )
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                raw = resp.read().decode() or "null"
                return resp.status, json.loads(raw)
        except urllib.error.HTTPError as exc:  # 4xx/5xx come through here
            raw = exc.read().decode() or "null"
            try:
                return exc.code, json.loads(raw)
            except json.JSONDecodeError:
                return exc.code, raw

    # ---- request builders -------------------------------------------------

    @staticmethod
    def blank_context() -> dict:
        """The minimal schema-valid context (config/session_state are required)."""
        return {
            "history": [],
            "tools": [],
            "models": [],
            "memory": "",
            "skills": [],
            "config": {"max_iterations": 10, "timeout_seconds": 300},
            "session_state": {
                "turn_count": 0,
                "total_tool_calls": 0,
                "total_llm_calls": 0,
                "cost_so_far": 0.0,
                "started_at": "2026-09-20T00:00:00+00:00",
            },
        }

    def process(
        self,
        session_id: str,
        content: str,
        *,
        models: list[str] | None = None,
        history: list[dict] | None = None,
    ) -> tuple[int, object]:
        ctx = self.blank_context()
        if models:
            ctx["models"] = [
                {
                    "name": name,
                    "provider": "openai",
                    "cost_per_1k_input": 0.0,
                    "cost_per_1k_output": 0.0,
                    "context_window": 8192,
                    "supports_vision": False,
                    "supports_tool_calling": True,
                }
                for name in models
            ]
        ctx["history"] = list(history or [])
        ctx["history"].append({"role": "user", "content": content})
        return self.request(
            "POST",
            "/v1/process",
            {
                "session_id": session_id,
                "message": {
                    "role": "user",
                    "content": content,
                    "timestamp": "2026-09-20T00:00:00+00:00",
                },
                "identity": {"platform": "fake-hermes", "chat_id": "fake-hermes-chat"},
                "context": ctx,
            },
        )

    def result(
        self,
        session_id: str,
        decision_id: str,
        result_type: str,
        data: dict,
        *,
        success: bool = True,
    ) -> tuple[int, object]:
        return self.request(
            "POST",
            "/v1/result",
            {
                "session_id": session_id,
                "decision_id": decision_id,
                "result": {
                    "type": result_type,
                    "data": data,
                    "success": success,
                    "duration_ms": 1.0,
                },
            },
        )

    def health(self) -> tuple[int, object]:
        return self.request("GET", "/v1/health")

    def session(self, session_id: str) -> tuple[int, object]:
        return self.request("GET", f"/v1/sessions/{session_id}")

    def terminate(self, session_id: str) -> tuple[int, object]:
        return self.request("DELETE", f"/v1/sessions/{session_id}")

    # ---- shared assertions ------------------------------------------------

    def assert_teardown(self, session_id: str) -> None:
        """DELETE releases the session, and a second GET is a clean 404."""
        status, body = self.terminate(session_id)
        check(status == 200, f"DELETE /v1/sessions/{session_id} -> {status}, want 200", body)
        check(body.get("terminated") is True, "DELETE did not report terminated=true", body)

        status, body = self.session(session_id)
        check(
            status == 404,
            f"GET after DELETE -> {status}, want 404 (session should be gone)",
            body,
        )

    @staticmethod
    def history_of(body: dict) -> list:
        history = body.get("history") or []
        check(isinstance(history, list), "decision.history is not a list", body)
        return history


class DeliberationClient(FakeHermes):
    """Two-model deliberation: the full llm_call round trip.

    process -> llm_call(model A) -> result -> llm_call(model B) ->
    result -> text VERDICT (finished) -> result -> end(task_complete).
    """

    def run(self) -> str:
        session_id = "fake-hermes-deliberation"
        print("[deliberation] health")
        status, body = self.health()
        check(status == 200, f"GET /v1/health -> {status}, want 200", body)
        check(body.get("status") == "ok", "health status is not 'ok'", body)
        check(
            "llm_call" in (body.get("capabilities") or []),
            "health capabilities do not advertise llm_call",
            body,
        )

        seeded = [
            {"role": "user", "content": "earlier question"},
            {"role": "assistant", "content": "earlier answer"},
        ]

        print("[deliberation] process -> llm_call (model A)")
        status, body = self.process(
            session_id,
            "Compare the two release plans and pick one.",
            models=[MODEL_A, MODEL_B],
            history=seeded,
        )
        check(status == 200, f"POST /v1/process -> {status}, want 200", body)
        check(body.get("decision") == "llm_call", "process did not return llm_call", body)
        draft_id = body.get("decision_id")
        check(bool(draft_id), "llm_call decision has no decision_id", body)
        llm = body.get("llm_call") or {}
        check(
            llm.get("model") == MODEL_A,
            f"first llm_call asked for model {llm.get('model')!r}, want {MODEL_A!r}",
            body,
        )
        check(bool(llm.get("messages")), "llm_call carries no messages", body)
        history_seed = len(self.history_of(body))
        check(
            history_seed >= len(seeded),
            f"history shrank at process: {history_seed} < {len(seeded)}",
            body,
        )

        print("[deliberation] result -> llm_call (model B, critique)")
        status, body = self.result(
            session_id,
            draft_id,
            "llm_response",
            {"content": "Draft: ship plan one on Friday.", "model": MODEL_A},
        )
        check(status == 200, f"POST /v1/result (draft) -> {status}, want 200", body)
        check(body.get("decision") == "llm_call", "draft result did not return llm_call", body)
        critique_id = body.get("decision_id")
        check(
            critique_id and critique_id != draft_id,
            "the second llm_call reused the first decision_id",
            body,
        )
        llm = body.get("llm_call") or {}
        check(
            llm.get("model") == MODEL_B,
            f"critique asked for model {llm.get('model')!r}, want {MODEL_B!r}",
            body,
        )
        history_draft = len(self.history_of(body))
        check(
            history_draft >= history_seed,
            f"history shrank on the draft result: {history_draft} < {history_seed}",
            body,
        )

        print("[deliberation] result -> text VERDICT (finished)")
        status, body = self.result(
            session_id,
            critique_id,
            "llm_response",
            {"content": "Critique: Friday is risky; plan two is safer.", "model": MODEL_B},
        )
        check(status == 200, f"POST /v1/result (critique) -> {status}, want 200", body)
        check(body.get("decision") == "text", "critique result did not return text", body)
        verdict_id = body.get("decision_id")
        text = body.get("text") or {}
        verdict = text.get("content") or ""
        check(
            verdict.startswith("VERDICT:"),
            f"verdict text does not start with 'VERDICT:': {verdict[:120]!r}",
            body,
        )
        check(text.get("finished") is True, "verdict text is not finished=true", body)
        check("ship plan one" in verdict, "verdict lost the draft completion", body)
        check("Friday is risky" in verdict, "verdict lost the critique completion", body)
        history_verdict = len(self.history_of(body))
        check(
            history_verdict >= history_draft,
            f"history shrank on the verdict: {history_verdict} < {history_draft}",
            body,
        )

        print("[deliberation] result -> end(task_complete)")
        status, body = self.result(
            session_id, verdict_id, "text_sent", {"content": verdict, "finished": True}
        )
        check(status == 200, f"POST /v1/result (verdict) -> {status}, want 200", body)
        check(body.get("decision") == "end", "verdict result did not return end", body)
        end = body.get("end") or {}
        check(
            end.get("reason") == "task_complete",
            f"end reason is {end.get('reason')!r}, want 'task_complete'",
            body,
        )
        check(
            len(self.history_of(body)) >= history_verdict,
            "history shrank on the end decision",
            body,
        )

        print("[deliberation] session status + teardown")
        status, body = self.session(session_id)
        check(status == 200, f"GET /v1/sessions -> {status}, want 200", body)
        check(
            body.get("status") == "completed",
            f"session status is {body.get('status')!r} after end, want 'completed'",
            body,
        )
        self.assert_teardown(session_id)
        return verdict.splitlines()[0]


class SingleModelClient(FakeHermes):
    """The other shapes of the same loop, on their own sessions:

    (a) one model offered -> the deliberation still runs two rounds, re-using
        the only model for the critique (it never invents a second one), then
        the verdict, then end;
    (b) a message asking for unfinished text ("do not finish") -> text
        finished=false, and the NEXT result flips it to finished=true;
    (c) no models offered  -> NO llm_call (a model picked out of thin air is a
        hallucinated model), a plain text fallback instead.
    """

    def run(self) -> str:
        verdict = self.run_single_model()
        self.run_streaming()
        self.run_no_models()
        return verdict

    def run_single_model(self) -> str:
        session_id = "fake-hermes-single-model"
        print("[single-model] process -> llm_call (the only model offered)")
        status, body = self.process(session_id, "Summarise the incident.", models=[MODEL_A])
        check(status == 200, f"POST /v1/process -> {status}, want 200", body)
        check(body.get("decision") == "llm_call", "process did not return llm_call", body)
        check(
            (body.get("llm_call") or {}).get("model") == MODEL_A,
            "llm_call used a model that was not offered",
            body,
        )
        draft_id = body.get("decision_id")

        print("[single-model] result -> llm_call (critique re-uses the only model)")
        status, body = self.result(
            session_id, draft_id, "llm_response", {"content": "Draft summary.", "model": MODEL_A}
        )
        check(status == 200, f"POST /v1/result -> {status}, want 200", body)
        check(body.get("decision") == "llm_call", "draft result did not return llm_call", body)
        check(
            (body.get("llm_call") or {}).get("model") == MODEL_A,
            "the critique round invented a model that was not offered",
            body,
        )
        critique_id = body.get("decision_id")

        print("[single-model] result -> text VERDICT")
        status, body = self.result(
            session_id,
            critique_id,
            "llm_response",
            {"content": "Incident summary.", "model": MODEL_A},
        )
        check(status == 200, f"POST /v1/result -> {status}, want 200", body)
        check(body.get("decision") == "text", "result did not return text", body)
        verdict = (body.get("text") or {}).get("content") or ""
        check(
            verdict.startswith("VERDICT:"),
            f"verdict text does not start with 'VERDICT:': {verdict[:120]!r}",
            body,
        )
        check("Incident summary." in verdict, "verdict lost the completion", body)
        verdict_id = body.get("decision_id")

        status, body = self.result(
            session_id, verdict_id, "text_sent", {"content": verdict, "finished": True}
        )
        check(status == 200, f"POST /v1/result (verdict) -> {status}, want 200", body)
        check(body.get("decision") == "end", "verdict result did not return end", body)
        check(
            (body.get("end") or {}).get("reason") == "task_complete",
            "end reason is not task_complete",
            body,
        )

        status, body = self.session(session_id)
        check(body.get("status") == "completed", "session did not reach completed", body)
        self.assert_teardown(session_id)
        return verdict.splitlines()[0]

    def run_streaming(self) -> None:
        session_id = "fake-hermes-streaming"
        print("[streaming] process -> text finished=false ('do not finish')")
        status, body = self.process(
            session_id, "Just start a thought, do not finish it yet.", models=[MODEL_A]
        )
        check(status == 200, f"POST /v1/process -> {status}, want 200", body)
        check(body.get("decision") == "text", "streaming request did not return text", body)
        text = body.get("text") or {}
        check(text.get("finished") is False, "streaming text is not finished=false", body)
        history_open = len(self.history_of(body))
        first_id = body.get("decision_id")

        print("[streaming] result -> text finished=true")
        status, body = self.result(
            session_id, first_id, "text_sent", {"content": "started", "finished": False}
        )
        check(status == 200, f"POST /v1/result -> {status}, want 200", body)
        check(body.get("decision") == "text", "the streaming result did not flip to text", body)
        text = body.get("text") or {}
        check(text.get("finished") is True, "the next text is not finished=true", body)
        check(
            len(self.history_of(body)) >= history_open,
            "history shrank in the streaming loop",
            body,
        )
        second_id = body.get("decision_id")

        print("[streaming] result -> end(task_complete)")
        status, body = self.result(
            session_id, second_id, "text_sent", {"content": text.get("content") or "done"}
        )
        check(status == 200, f"POST /v1/result -> {status}, want 200", body)
        check(body.get("decision") == "end", "the streaming session did not end", body)
        check(
            (body.get("end") or {}).get("reason") == "task_complete",
            "streaming end reason is not task_complete",
            body,
        )

        status, body = self.session(session_id)
        check(body.get("status") == "completed", "streaming session did not complete", body)
        self.assert_teardown(session_id)

    def run_no_models(self) -> None:
        session_id = "fake-hermes-no-models"
        print("[no-models] process -> text (never llm_call without an offered model)")
        status, body = self.process(session_id, "use any model you want", models=[])
        check(status == 200, f"POST /v1/process -> {status}, want 200", body)
        check(
            body.get("decision") != "llm_call",
            "harness returned llm_call although context.models was empty "
            "(a model nobody offered is a hallucinated model)",
            body,
        )
        check(
            body.get("decision") == "text",
            f"expected the plain text fallback, got {body.get('decision')!r}",
            body,
        )
        first_id = body.get("decision_id")

        status, body = self.result(session_id, first_id, "text_sent", {"content": "fallback sent"})
        check(body.get("decision") == "end", "the fallback session did not end", body)
        status, body = self.session(session_id)
        check(body.get("status") == "completed", "fallback session did not complete", body)
        self.assert_teardown(session_id)


def resolve_port(argv: list[str]) -> str:
    """argv[1] wins, then PORT, then the default port every example uses."""
    if len(argv) > 1 and argv[1].strip():
        return argv[1].strip()
    return os.environ.get("PORT", "").strip() or DEFAULT_PORT


def main(argv: list[str]) -> int:
    port = resolve_port(argv)
    print(f"fake-hermes: driving http://{DEFAULT_HOST}:{port} (PORT=%s)" % port)
    try:
        # Fail fast and clearly when nothing is listening, instead of surfacing
        # a connection error mid-round-trip.
        probe = FakeHermes(port=port)
        status, body = probe.health()
        check(status == 200, f"GET /v1/health -> {status}, want 200", body)

        verdicts = [
            ("deliberation", DeliberationClient(port=port).run()),
            ("single-model", SingleModelClient(port=port).run()),
        ]
    except CheckFailure as exc:
        print(f"\nFAIL: {exc}", file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:
        print(
            f"\nFAIL: cannot reach the harness on port {port}: {exc}\n"
            f"      start it first: PORT={port} go run ./examples/llm-roundtrip/",
            file=sys.stderr,
        )
        return 1

    print("")
    for name, verdict in verdicts:
        print(f"OK  {name}: {verdict}")
    print("PASS: llm_call round trip verified (verdict text, completed sessions, DELETE -> 404)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
