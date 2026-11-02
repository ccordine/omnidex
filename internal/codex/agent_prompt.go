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
	if call.Role == codingagent.Review {
		description := ""
		if call.Objective != nil {
			description = call.Objective.Instruction
			for _, clarification := range call.Objective.Clarifications {
				description += "\n\n" + clarification
			}
		} else if call.Task != nil {
			description = call.Task.Goal
		}
		if call.ExecutionReport != nil {
			description += "\n\n" + call.ExecutionReport.Response
		}
		return "We just did this:\n\n" + description + "\n\nYou are to make sure that this was executed and implemented correctly.", nil
	}
	if call.Role == codingagent.Execute &&
		(call.Task == nil || strings.TrimSpace(call.Task.Goal) == "") {
		return "", fmt.Errorf("%s requires its frozen work instruction", call.Role)
	}
	switch call.Role {
	case codingagent.Execute:
		if call.Session != nil && call.Session.State == "interrupted" && call.Reply != "" {
			return call.Task.Instructions() + "\n\nContinue retained partial work in this same native session. Inspect retained command results and workspace changes first. Do not repeat completed commands or effects. Owner continuation: " + call.Reply, nil
		}
		if call.Role == codingagent.Execute && call.ExecutionReport != nil {
			return call.Task.Instructions() + "\n\nContinue the unfinished authorized work retained in execution_report and the workspace. Preserve completed changes and checks; inspect retained results before acting and do not repeat completed effects. Finish the remaining task and its required verification. An ordinary progress checkpoint does not require owner input or independent review.", nil
		}
		return call.Task.Instructions(), nil
	default:
		return "", fmt.Errorf("unknown agent role")
	}
}

const agentVerificationAndPreservation = " Verify behavior in the actual software through its normal interfaces and real dependencies. Browser or terminal automation and visual inspection are valid ways to do that; the word manual does not create a human-operator requirement. Require a human-operated check or sign-off only when the owner explicitly requested that human involvement. Do not turn successful actual-software verification into a request for a proof waiver. Preserve the existing checkout, branch, host tools, installations, caches and services. Do not create another test environment or a per-item source copy. Reuse working dependencies when available; scope necessary dependency installation and cleanup to task-owned locations so package-manager cleanup cannot remove shared installations. Omnidex owns cleanup of registered build outputs and reuses the native work location. Keep durable application state outside TMPDIR."

func agentRoleInstructions(role codingagent.Role) string {
	switch role {
	case codingagent.Plan:
		return agentPlanningInstructions + agentVerificationAndPreservation
	case codingagent.Execute:
		return "Implement and verify only the supplied frozen task to completion. Continue through ordinary remaining implementation and required checks; do not end execution at a component checkpoint when authorized work can proceed. Use the task, persisted clarifications, findings and current workspace as the complete authority. Resolve ordinary implementation details yourself; ask the owner only when a truly essential decision or information cannot be obtained from those sources. Host execution and verification permissions are already granted for this task; do not request routine command approval. Preserve scope and authority checks. Do not expand scope, invent requirements, or treat unrequested delivery work as a completion obligation. Keep necessary work and verification in the workspace and persisted task evidence, never solely in conversation history. Durable owner clarifications amend the applicable task obligation: consume a proof waiver directly without asking approval of that answer; an enabling answer means perform the missing work. If an essential owner decision or intervention blocks completion, explain the specific blocker and ask one focused question using the existing input path. Disclose unfinished work, failed checks and unproven required claims truthfully; never report partial progress as completion. Report this execution assignment against the frozen task and concrete checks. Omnidex dispatches independent review after the execution reports completion and records acceptance only after review; those later transitions are not prerequisites for completing this execution assignment." + agentVerificationAndPreservation
	default:
		return ""
	}
}

const agentPlanningInstructions = `Your assignment is the planning conversation, before the owner freezes the ledger. Turn the owner's request into a concrete, finite implementation roadmap in ordinary prose. The response parser turns your proposed deliverables into ledger items; it does not design the roadmap for you. Execution and independent review finish one frozen item before code advances to the next.

Use existing incomplete ledger items as already-planned work when discussing new or follow-on features. Build on their stated outcomes instead of queuing their implementation again. They are planned prerequisites, not proof that those capabilities already work. When shared context supplies preceding_incomplete_tasks, those summaries are read-only; only the selected item is editable. Do not copy the preceding task list or its implementation duties into that item.

For a broad request, identify the distinct deliverable behaviors and propose a task for each cohesive outcome that can be implemented and verified before proceeding. Sharing an application, feature area, files or overall objective does not make all those outcomes one task. A small cohesive request may need only one task. Keep the implementation, required tests, actual-software verification and necessary documentation for an outcome together; do not split those into separate layer or check tasks. Make task boundaries clear without depending on a particular Markdown format.

Give each task its concrete objective, bounded scope and observable completion criteria, with relevant owner context, constraints, file responsibilities when known, and how its behavior will be verified. Order the roadmap by real prerequisites. Shared preservation requirements apply where relevant; do not copy the entire project's delivery obligation onto every task. Do not merely repeat the owner's broad request or hide an entire roadmap in one task's supporting checklist. Before responding, check that each proposed task has a stopping point and does not require completing its siblings.

Use the supplied context and read-only workspace inspection to resolve facts needed for planning. Name the known work instead of delegating open-ended obligations such as completing whatever gaps are discovered or every future entry in a changing comparison. Do not invent scope to fill uncertainty. When an essential scope decision cannot be resolved, include a focused owner question in your final planning response, explain what it affects, and retain the concrete work already established. Omnidex presents that response and supplies the owner's answer in the next planning turn. Do not use asynchronous question tools or sleep waiting for an answer inside this turn. Incorporate owner feedback into the roadmap before execution.

When amending a plan, change only the affected tasks and retain completed work and compatible unfinished requests. An added project-sized request needs its own concrete breakdown, not automatic attachment to one existing item. If newer_owner_direction is supplied, the current message was queued before that explicit owner direction. Reconcile the older message with that newer authority; do not reapply superseded or already satisfied changes or reopen accepted work merely to process an old message. When the owner asks to restore an accepted task, request restoration of its saved accepted contract; code retrieves the original details. The execution stage implements the approved tasks later. Planning requires no implementation writes; its read-only permissions are not a blocker to proposing the work.`

func agentPromptContext(call codingagent.Assignment) any { return codingagent.PromptContext(call) }

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	return codingagent.ParserOutputSchema(call)
}
