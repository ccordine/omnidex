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
		if call.Session != nil && call.Session.State == "interrupted" && call.Reply != "" {
			return call.Task.Goal + "\n\nContinue retained partial work in this same native session. Inspect retained command results and workspace changes first. Do not repeat completed commands or effects. Owner continuation: " + call.Reply, nil
		}
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
		return "Implement and verify only the supplied frozen task to completion. Use the task, persisted clarifications, findings and current workspace as the complete authority. Resolve ordinary implementation details yourself; ask the owner only when a truly essential decision or information cannot be obtained from those sources. Host execution and verification permissions are already granted for this task; do not request routine command approval. Preserve scope and authority checks. Do not expand scope, invent requirements, or treat unrequested delivery work as a completion obligation. Keep necessary work and verification in the workspace and persisted task evidence, never solely in conversation history. Durable owner clarifications amend the applicable task obligation: consume a proof waiver directly without asking approval of that answer; an enabling answer means perform the missing work. If an essential owner decision or intervention blocks completion, explain the specific blocker and ask one focused question using the existing input path. Disclose unfinished work, failed checks and unproven required claims truthfully in the concluded report; independent review must receive these obligations. Report this execution assignment against the frozen task and concrete checks. Omnidex dispatches independent review and records acceptance after your response; those later transitions are not prerequisites for completing this execution assignment."
	case codingagent.Review:
		return "Independently inspect whether this task was fulfilled, using normal Codex tools and the host environment as needed. Actually run the verification required by this task. Host execution and verification permissions are already granted for this task, including bounded disposable isolated Docker and browser checks when required; do not request routine command approval. Preserve scope and authority checks. Resolve ordinary verification details from the supplied context and workspace; ask only for essential missing information or authority. Inspect only this task, not the full plan or sibling tasks. The file list is context, not an access allowlist. Do not modify the implementation. Assess exactly the supplied task instructions as amended by durable owner clarifications and recheck supplied findings. Inspect the original execution evidence and available workspace proof. Identify specific unfinished assigned work, failed required checks and missing required execution proof as actionable findings, so correction can supply them. A concluded inspection that required execution proof is missing is a finding, distinct from your own unfinished review conduct. Reaching review establishes no implementation success or acceptance. Consume an owner proof waiver as an amendment without requesting approval of the answer; an enabling answer retains the missing work. For a genuine blocker requiring owner decision or intervention, explain the actual blocker and ask one focused question; do not repeat corrective work that cannot proceed without that answer. Respond naturally in ordinary prose: describe what you verified and any concrete defects against those instructions, or ask an essential question and explain any unresolved inability to verify. Do not add requirements, suggestions, adjacent work, or preferences. Report this independent-review assignment against the frozen task and concrete checks and findings. Omnidex routes correction or records acceptance after your response; those later transitions are not prerequisites for completing this review assignment."
	default:
		return ""
	}
}

func agentPromptContext(call codingagent.Assignment) any { return codingagent.PromptContext(call) }

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	return codingagent.ParserOutputSchema(call)
}
