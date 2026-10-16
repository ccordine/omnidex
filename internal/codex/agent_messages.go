package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Asynchronous native questions precede, rather than replace, the terminal
// answer. Retain their prose for the separate response parser. Original frames
// remain unchanged; two actual terminal answers are still invalid.
type agentResponseMessages struct{ async []string }

func (messages *agentResponseMessages) collectItem(result *codingagent.Result, role codingagent.Role, kind, phase, delivery, text string) error {
	if role.IsNative() && kind == "agentMessage" && delivery == "async" {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("native asynchronous message is empty")
		}
		messages.async = append(messages.async, text)
		return nil
	}
	return collectAgentResultItem(result, role, kind, phase, text)
}

func (messages *agentResponseMessages) finish(result *codingagent.Result) error {
	if len(messages.async) == 0 {
		return nil
	}
	var final string
	if err := json.Unmarshal(result.Content, &final); err != nil || strings.TrimSpace(final) == "" {
		return fmt.Errorf("native asynchronous messages lack a terminal answer")
	}
	content, err := json.Marshal(strings.Join(append(messages.async, final), "\n\n"))
	if err != nil {
		return err
	}
	result.Content = content
	messages.async = nil
	return nil
}
