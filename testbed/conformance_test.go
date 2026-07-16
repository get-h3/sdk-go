package testbed

import (
	"testing"

	"github.com/get-h3/sdk-go/protocol"
)

func TestConformanceHarness_ToolCall(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	ctx := DefaultContext()
	req := &protocol.ProcessRequest{
		SessionID: "sess-tool",
		Message:   QuickMessage("echo hello"),
		Identity:  QuickIdentity("tester", "user-1"),
		Context:   ctx,
	}
	dec, err := mh.Harness.OnProcess(req)
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionToolCall)
	AssertDecisionValid(t, dec)
	if dec.ToolCall.Name != ctx.Tools[0].Name {
		t.Errorf("expected tool name %q, got %q", ctx.Tools[0].Name, dec.ToolCall.Name)
	}
}

func TestConformanceHarness_NoToolCallWithoutTools(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	ctx := DefaultContext()
	ctx.Tools = nil
	req := &protocol.ProcessRequest{
		SessionID: "sess-no-tool",
		Message:   QuickMessage("echo hello"),
		Identity:  QuickIdentity("tester", "user-1"),
		Context:   ctx,
	}
	dec, err := mh.Harness.OnProcess(req)
	AssertNoError(t, err)
	if dec.Decision == protocol.DecisionToolCall {
		t.Errorf("expected non-tool decision, got %q", dec.Decision)
	}
}

func TestConformanceHarness_LLMCall(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	ctx := DefaultContext()
	req := &protocol.ProcessRequest{
		SessionID: "sess-llm",
		Message:   QuickMessage("run model"),
		Identity:  QuickIdentity("tester", "user-1"),
		Context:   ctx,
	}
	dec, err := mh.Harness.OnProcess(req)
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionLLMCall)
	AssertDecisionValid(t, dec)
	if dec.LLMCall.Model != ctx.Models[0].Name {
		t.Errorf("expected model name %q, got %q", ctx.Models[0].Name, dec.LLMCall.Model)
	}
}

func TestConformanceHarness_NoLLMCallWithoutModels(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	ctx := DefaultContext()
	ctx.Models = nil
	req := &protocol.ProcessRequest{
		SessionID: "sess-no-llm",
		Message:   QuickMessage("run model"),
		Identity:  QuickIdentity("tester", "user-1"),
		Context:   ctx,
	}
	dec, err := mh.Harness.OnProcess(req)
	AssertNoError(t, err)
	if dec.Decision == protocol.DecisionLLMCall {
		t.Errorf("expected non-llm decision, got %q", dec.Decision)
	}
}

func TestConformanceHarness_TextFinishedFalse(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	dec, err := mh.SendMessage("sess-thought", "start a thought, do not finish", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionText)
	if dec.Text.Finished {
		t.Errorf("expected finished=false, got true")
	}
}

func TestConformanceHarness_TextFinishedTrue(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	dec, err := mh.SendMessage("sess-final", "final answer", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionText)
	AssertTextContent(t, dec, "The answer is 42.", true)
}

func TestConformanceHarness_Delegate(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	dec, err := mh.SendMessage("sess-delegate", "delegate this task", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionDelegate)
	AssertDecisionValid(t, dec)
	expectedTask := "delegated task: delegate this task"
	if dec.Delegate.Task != expectedTask {
		t.Errorf("expected task %q, got %q", expectedTask, dec.Delegate.Task)
	}
}

func TestConformanceHarness_End(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	dec, err := mh.SendMessage("sess-end", "DONE", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionEnd)
	AssertEndReason(t, dec, protocol.EndTaskComplete)
}

func TestConformanceHarness_FullLoop(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	sessionID := "sess-loop"

	// Step 1: tool_call
	dec1, err := mh.SendMessage(sessionID, "echo hello", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec1, protocol.DecisionToolCall)

	// Step 2: result
	res := protocol.Result{
		Type:    protocol.ResultTool,
		Success: true,
		Data:    map[string]any{"output": "hello"},
	}
	dec2, err := mh.SendResult(sessionID, dec1.DecisionID, res)
	AssertNoError(t, err)
	AssertDecisionType(t, dec2, protocol.DecisionText)
	AssertTextContent(t, dec2, "Result received: tool_result", true)

	// Step 3: text
	dec3, err := mh.SendMessage(sessionID, "final answer", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec3, protocol.DecisionText)
	AssertTextContent(t, dec3, "The answer is 42.", true)

	// Step 4: end
	dec4, err := mh.SendMessage(sessionID, "DONE", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, dec4, protocol.DecisionEnd)
	AssertEndReason(t, dec4, protocol.EndTaskComplete)
}

func TestConformanceHarness_SessionIsolation(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	decA, err := mh.SendMessage("sess-a", "echo hello", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, decA, protocol.DecisionToolCall)

	decB, err := mh.SendMessage("sess-b", "final answer", "tester", "user-1")
	AssertNoError(t, err)
	AssertDecisionType(t, decB, protocol.DecisionText)
	AssertTextContent(t, decB, "The answer is 42.", true)

	// Force a result on sess-a and verify it does not affect sess-b.
	res := protocol.Result{Type: protocol.ResultTool, Success: true}
	decA2, err := mh.SendResult("sess-a", decA.DecisionID, res)
	AssertNoError(t, err)
	AssertDecisionType(t, decA2, protocol.DecisionText)
}

func TestConformanceHarness_ResultError(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	dec, err := mh.SendResult("sess-error", "dec-001", protocol.Result{
		Type:    protocol.ResultError,
		Success: false,
	})
	AssertNoError(t, err)
	AssertDecisionType(t, dec, protocol.DecisionEnd)
	AssertEndReason(t, dec, protocol.EndError)
}

func TestConformanceHarness_HealthCapabilities(t *testing.T) {
	h := NewConformanceHarness()
	mh := NewMockHermes(h)

	resp := mh.Health()
	if resp == nil {
		t.Fatal("expected health response, got nil")
	}
	if resp.Status != protocol.HealthOK {
		t.Errorf("expected status ok, got %q", resp.Status)
	}
	if len(resp.Capabilities) == 0 {
		t.Error("expected non-empty capabilities list")
	}
	expected := map[protocol.DecisionType]bool{}
	for _, dt := range resp.Capabilities {
		expected[dt] = true
	}
	for _, dt := range []protocol.DecisionType{
		protocol.DecisionToolCall,
		protocol.DecisionLLMCall,
		protocol.DecisionText,
		protocol.DecisionWait,
		protocol.DecisionDelegate,
		protocol.DecisionEnd,
	} {
		if !expected[dt] {
			t.Errorf("expected capability %q not found", dt)
		}
	}
}
