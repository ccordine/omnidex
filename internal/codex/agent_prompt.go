package codex

import (
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
)

func renderAgentAssignment(call codingagent.Assignment) (string, error) {
	if !call.Role.IsNative() {
		return codingagent.RenderParserPrompt(call)
	}
	if call.Role == codingagent.Execute && call.Reply != "" && call.Session != nil {
		// Native history already holds the task and earlier messages. Send only
		// the owner's new reply, not a duplicated transcript or task snapshot.
		return call.Reply, nil
	}
	if call.Role == codingagent.Plan && call.Objective != nil {
		return call.Objective.Instruction, nil
	}
	var instruction string
	switch call.Role {
	case codingagent.Plan:
		instruction = "Continue this planning conversation with the owner. Use the supplied conversation history and current persisted planning ledger as context. Respond naturally in ordinary planning prose. Describe the planning changes requested in the current turn. When proposing implementation tasks, describe their task-local acceptance criteria and verification, usable before advancing to the next task. Questions remain conversational questions, not implementation tasks. An owner answer may resolve or change the referenced proposed work; do not preserve obsolete questions as outstanding tasks."
	case codingagent.Execute:
		instruction = "Execute this approved ledger task now using Codex normally; its planning review is already complete. The supplied context describes this task's scope, guidance, relevant files, and acceptance criteria."
	case codingagent.Review:
		instruction = "Independently inspect whether this task was fulfilled, using normal Codex tools and the host environment as needed. Inspect only this task, not the full plan or sibling tasks. The file list is context, not an access allowlist. Do not modify the implementation or ledger. Apply exactly the supplied acceptance criteria and recheck supplied findings. Respond naturally in ordinary prose: describe what you verified and any concrete defects against the supplied criteria, or ask any necessary question or permission and explain any unresolved error or inability to verify. Do not add requirements, suggestions, adjacent work, or preferences."
	default:
		return "", fmt.Errorf("unknown agent role")
	}
	return instruction, nil
}

func agentPromptContext(call codingagent.Assignment) any { return codingagent.PromptContext(call) }

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	return codingagent.ParserOutputSchema(call)
}
