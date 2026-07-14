package protocol

import "fmt"

// Validate checks that a ProcessRequest is well-formed.
func (r *ProcessRequest) Validate() error {
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if r.Message.Role == "" {
		return fmt.Errorf("message.role is required")
	}
	if r.Message.Content == "" {
		return fmt.Errorf("message.content is required")
	}
	if r.Message.Timestamp == "" {
		return fmt.Errorf("message.timestamp is required")
	}
	if r.Identity.Platform == "" {
		return fmt.Errorf("identity.platform is required")
	}
	if r.Identity.ChatID == "" {
		return fmt.Errorf("identity.chat_id is required")
	}
	if r.Identity.UserName == "" {
		return fmt.Errorf("identity.user_name is required")
	}
	if r.Identity.UserID == "" {
		return fmt.Errorf("identity.user_id is required")
	}
	if r.Context.Config.MaxIterations < 1 {
		return fmt.Errorf("context.config.max_iterations must be >= 1")
	}
	if r.Context.Config.TimeoutSeconds < 1 {
		return fmt.Errorf("context.config.timeout_seconds must be >= 1")
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
