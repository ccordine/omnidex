package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Context is history data, not part of the current user message and never a
// developer/system instruction. The existing compiler owns its minification.
func renderAgentContext(call codingagent.Assignment) (string, error) {
	encoded := call.Context
	if encoded == "" {
		raw, err := json.Marshal(agentPromptContext(call))
		if err != nil {
			return "", err
		}
		encoded = string(raw)
	}
	context := "Shared context (background, not a new request):"
	if call.Role == codingagent.Plan {
		guidance := call
		guidance.Objective = nil
		instruction, err := renderAgentAssignment(guidance)
		if err != nil {
			return "", err
		}
		context += "\n" + instruction
	}
	if call.Role == codingagent.ReviewResponse {
		references, err := json.Marshal(agentReviewReferences(call))
		if err != nil {
			return "", err
		}
		contract, err := json.Marshal(agentOutputSchema(call))
		if err != nil {
			return "", err
		}
		context += "\n\nReview result JSON Schema:\n" + string(contract) + "\n\nReview reference table:\n" + string(references)
	}
	if !call.Role.IsNative() && call.Role != codingagent.ReviewResponse {
		contract, err := json.Marshal(agentOutputSchema(call))
		if err != nil {
			return "", err
		}
		context += "\n\nParser result JSON Schema:\n" + string(contract)
	}
	return context + "\n\nAssignment:\n" + encoded, nil
}

type agentHistoryContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type agentHistoryMessage struct {
	Type    string                `json:"type"`
	Role    string                `json:"role"`
	Content []agentHistoryContent `json:"content"`
}

func (connection *connection) injectAgentContext(call codingagent.Assignment, threadID string) (json.RawMessage, error) {
	if call.Role == codingagent.Execute && call.Session != nil {
		return nil, nil // Resumed native history already contains this task.
	}
	context, err := renderAgentContext(call)
	if err != nil {
		return nil, err
	}
	return connection.call("thread/inject_items", struct {
		ThreadID string                `json:"threadId"`
		Items    []agentHistoryMessage `json:"items"`
	}{threadID, []agentHistoryMessage{{Type: "message", Role: "assistant",
		Content: []agentHistoryContent{{Type: "output_text", Text: context}}}}}, nil)
}
