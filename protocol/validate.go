package protocol

import "fmt"

// Validate checks that a ProcessRequest is well-formed.
// Required fields: session_id, message.role, identity.platform, identity.chat_id.
// Timestamp, user_name, user_id, and config values are optional per the H3
// protocol; the h3-test battery sends minimal requests without them.
func (r *ProcessRequest) Validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if r.Message.Role == "" {
		return fmt.Errorf("message.role is required")
	}
	if r.Identity.Platform == "" {
		return fmt.Errorf("identity.platform is required")
	}
	if r.Identity.ChatID == "" {
		return fmt.Errorf("identity.chat_id is required")
	}
	return nil
}

// Validate checks that a Decision is well-formed.
func (d *Decision) Validate() error {
	if d.DecisionID == "" {
		return fmt.Errorf("decision_id is required")
	}
	switch d.Decision {
	case DecisionToolCall:
		if d.ToolCall == nil {
			return fmt.Errorf("tool_call is required for decision type 'tool_call'")
		}
		if d.ToolCall.Name == "" {
			return fmt.Errorf("tool_call.name is required")
		}
	case DecisionLLMCall:
		if d.LLMCall == nil {
			return fmt.Errorf("llm_call is required for decision type 'llm_call'")
		}
		if d.LLMCall.Model == "" {
			return fmt.Errorf("llm_call.model is required")
		}
		if len(d.LLMCall.Messages) == 0 {
			return fmt.Errorf("llm_call.messages must have at least one message")
		}
	case DecisionText:
		if d.Text == nil {
			return fmt.Errorf("text is required for decision type 'text'")
		}
		if d.Text.Content == "" {
			return fmt.Errorf("text.content is required")
		}
	case DecisionWait:
		if d.Wait == nil {
			return fmt.Errorf("wait is required for decision type 'wait'")
		}
		if d.Wait.Reason == "" {
			return fmt.Errorf("wait.reason is required")
		}
	case DecisionDelegate:
		if d.Delegate == nil {
			return fmt.Errorf("delegate is required for decision type 'delegate'")
		}
		if d.Delegate.Task == "" {
			return fmt.Errorf("delegate.task is required")
		}
	case DecisionEnd:
		if d.End == nil {
			return fmt.Errorf("end is required for decision type 'end'")
		}
		if d.End.Reason == "" {
			return fmt.Errorf("end.reason is required")
		}
	default:
		return fmt.Errorf("unknown decision type: %q", d.Decision)
	}
	return nil
}
