package codex

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
)

func renderAgentAssignment(call codingagent.Assignment) (string, error) {
	if !call.Role.IsNative() {
		return codingagent.RenderParserPrompt(call)
	}
	if call.Role == codingagent.Plan {
		if call.Objective == nil || strings.TrimSpace(call.Objective.Instruction) == "" {
			return "", fmt.Errorf("planning requires the current owner turn")
		}
		return call.Objective.Instruction, nil
	}
	if (call.Role == codingagent.Execute || call.Role == codingagent.Review) &&
		(call.Task == nil || strings.TrimSpace(call.Task.Goal) == "") {
		return "", fmt.Errorf("%s requires its frozen work instruction", call.Role)
	}
	switch call.Role {
	case codingagent.Execute, codingagent.Review:
		return call.Task.Goal, nil
	default:
		return "", fmt.Errorf("unknown agent role")
	}
}

func agentRoleInstructions(role codingagent.Role) string {
	switch role {
	case codingagent.Plan:
		return "Your assignment is the planning conversation, before the owner freezes the ledger. Return ordinary prose proposing or amending only the requested implementation tasks. Preserve concrete scope, details and required verification. The execution stage implements the approved tasks later. Planning requires no implementation writes; its read-only permissions are not a blocker to proposing the work. Ask only for an essential owner decision that the supplied context and workspace cannot resolve."
	case codingagent.Execute:
		return "Implement and verify only the supplied frozen task to completion. Use the task, persisted clarifications, findings and current workspace as the complete authority. Resolve ordinary implementation details yourself; ask the owner only when a truly essential decision or information cannot be obtained from those sources. Host execution permissions are already granted. Do not expand scope, invent requirements, or treat unrequested delivery work as a completion obligation. Keep necessary work and verification in the workspace and persisted task evidence, never solely in conversation history."
	case codingagent.Review:
		return "Independently inspect whether this task was fulfilled, using normal Codex tools and the host environment as needed. Inspect only this task, not the full plan or sibling tasks. The file list is context, not an access allowlist. Do not modify the implementation. Assess exactly the supplied task instructions and recheck supplied findings. Respond naturally in ordinary prose: describe what you verified and any concrete defects against those instructions, or ask an essential question and explain any unresolved inability to verify. Do not add requirements, suggestions, adjacent work, or preferences."
	default:
		return ""
	}
}

func agentPromptContext(call codingagent.Assignment) any { return codingagent.PromptContext(call) }

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	return codingagent.ParserOutputSchema(call)
}
