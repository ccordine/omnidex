package codex

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Context is history data, not part of the current user message and never a
// developer/system instruction. The existing compiler owns its minification.
func renderAgentContext(call codingagent.Assignment) (string, error) {
	if !call.Role.IsNative() {
		return codingagent.RenderParserContext(call)
	}
	encoded := call.Context
	if encoded == "" {
		raw, err := json.Marshal(agentPromptContext(call))
		if err != nil {
			return "", err
		}
		encoded = string(raw)
	}
	context := "Shared context (background, not a new request):"
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
