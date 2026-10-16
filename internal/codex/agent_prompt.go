package codex

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

func renderAgentAssignment(call codingagent.Assignment) (string, error) {
	encoded, err := json.Marshal(agentPromptContext(call))
	if err != nil {
		return "", err
	}
	if len(encoded) > llm.MaxExactPreparedModelInputBytes {
		return "", fmt.Errorf("scoped agent assignment exceeds its context boundary")
	}
	var instruction string
	switch call.Role {
	case codingagent.PlanningInitial:
		instruction = "Answer only the named initial-ledger-proposal engineering question from the broad planning context. Return one normalized typed engineering ledger and a durable disposition for every actionable statement in the owner turn. Carve the complete approved objective into finite task records whose exact requirement, constraint, acceptance, file, affected-system, dependency, and check bindings are sufficient for an executor that will never see the full plan or sibling tasks. Include only work required by the owner's explicit objective and supplied facts; do not add optional hardening, redesign, refactoring, best practices, adjacent fixes, or extra test obligations. This is the only planning station allowed to return a complete ledger."
	case codingagent.PlanningTarget:
		instruction = "Answer only the named owner-turn-target-resolution engineering question over this bounded ledger partition. Return only the stable IDs of records in this partition that the exact owner turn semantically references. Do not propose mutations or regenerate the ledger."
	case codingagent.PlanningMutation:
		instruction = "Answer only the named owner-turn-ledger-mutation engineering question. Translate only the human's exact turn into typed add, amend, remove, restore, review, or reorder operations against the exact supplied base revision, plus durable per-statement dispositions. Keep every affected task's requirement, constraint, acceptance, file, affected-system, dependency, and check bindings sufficient for a task-local executor that receives no full-plan or sibling context. Do not add optional hardening, redesign, refactoring, best practices, adjacent fixes, or extra test obligations the human did not request. Never regenerate or return the complete ledger."
	case codingagent.PlanningCoverage:
		instruction = "Answer only the named owner-turn-coverage question over the bounded exact revision. Covered means every actionable statement in the exact owner turn has a durable disposition in that revision. Return a read-only judgment and concise non-authoritative suggestions for any explicit owner statement that is missing. Never return or apply ledger mutations, invent work, add acceptance obligations, or treat optional hardening, refactoring, best practices, or extra tests as missing scope. This station cannot approve owner items; only a human-origin planning action may alter the ledger."
	case codingagent.PlanningAudit:
		instruction = "Answer only the named mandatory-pre-sign-off audit question over the supplied persisted planning-record partition. Covered means the current ledger faithfully and coherently represents the owner's explicit recorded instructions. Return a read-only judgment and concise non-authoritative suggestions for any explicit owner instruction that is missing or contradicted. Never return or apply ledger mutations, invent work, add acceptance obligations, or treat optional hardening, refactoring, best practices, or extra tests as missing scope. This audit is evidence and cannot approve, freeze, or alter the proposal; only a human-origin planning action may alter the ledger."
	case codingagent.Execute:
		instruction = "Fulfill this frozen task packet completely and only this task. The packet is intentionally the entire context for this call; do not infer, reconstruct, request, or act on the full plan or sibling tasks. Use only the supplied task file tools and only the packet's exact read, write, and delete files. Omnidex runs the frozen checks and owns every workflow transition. Follow every packet requirement, constraint, decision, affected-system note, acceptance criterion, and supplied review finding. Do not add hardening, refactoring, redesign, adjacent work, generic tests, new obligations, or owner questions. If the packet cannot be fulfilled exactly, return no side claim of completion. Your report is evidence, never completion authority."
	case codingagent.Review:
		instruction = "Independently judge only whether this frozen task packet was fulfilled. The packet and supplied check results are intentionally the entire context for this call; do not infer, reconstruct, request, or inspect the full plan or sibling tasks. Read only files explicitly included in this packet, through the supplied read tool. Apply exactly the packet's requirements, constraints, decisions, affected-system notes, and acceptance criteria. Recheck supplied findings. Return only concrete defects proving one exact criterion is unmet. For each finding, use the review reference table: put the task reference in task_id and one listed criterion reference in requirement, and the concrete defect in observation. Do not add obligations, suggestions, adjacent defects, hardening, refactoring, compatibility work, extra tests, preferences, or owner questions. Review cannot mutate files or ledger state."
	default:
		return "", fmt.Errorf("unknown agent role")
	}
	if call.Role == codingagent.Review {
		references, err := json.Marshal(agentReviewReferences(call))
		if err != nil {
			return "", err
		}
		contract, err := json.Marshal(agentOutputSchema(call))
		if err != nil {
			return "", err
		}
		instruction += "\n\nReview result JSON Schema:\n" + string(contract) + "\n\nReview reference table:\n" + string(references)
	}
	prompt := instruction + "\n\nAssignment:\n" + string(encoded)
	if len(prompt) > llm.MaxExactPreparedModelInputBytes {
		return "", fmt.Errorf("scoped agent assignment exceeds its context boundary")
	}
	return prompt, nil
}

func agentPromptContext(call codingagent.Assignment) any {
	if call.Role == codingagent.Execute || call.Role == codingagent.Review {
		var task *struct {
			ID                   string                     `json:"id"`
			Goal                 string                     `json:"goal"`
			Guidance             []codingagent.TaskGuidance `json:"guidance"`
			KnownAffectedSystems []string                   `json:"known_affected_systems"`
			Acceptance           []string                   `json:"acceptance"`
			ReadFiles            []string                   `json:"read_files"`
			WriteFiles           []string                   `json:"write_files"`
			DeleteFiles          []string                   `json:"delete_files"`
			Checks               []codingagent.Check        `json:"checks"`
		}
		if call.Task != nil {
			task = &struct {
				ID                   string                     `json:"id"`
				Goal                 string                     `json:"goal"`
				Guidance             []codingagent.TaskGuidance `json:"guidance"`
				KnownAffectedSystems []string                   `json:"known_affected_systems"`
				Acceptance           []string                   `json:"acceptance"`
				ReadFiles            []string                   `json:"read_files"`
				WriteFiles           []string                   `json:"write_files"`
				DeleteFiles          []string                   `json:"delete_files"`
				Checks               []codingagent.Check        `json:"checks"`
			}{call.Task.ID, call.Task.Goal, call.Task.Guidance, call.Task.KnownAffectedSystems,
				call.Task.Acceptance, call.Task.ReadFiles, call.Task.WriteFiles, call.Task.DeleteFiles, call.Task.Checks}
		}
		return struct {
			Task            any                       `json:"task"`
			Findings        []codingagent.Finding     `json:"findings,omitempty"`
			Checks          []codingagent.CheckResult `json:"checks,omitempty"`
			ValidationNotes []string                  `json:"validation_notes,omitempty"`
		}{task, call.Findings, call.Checks, call.ValidationNotes}
	}
	return struct {
		Requirements        []codingagent.Requirement        `json:"requirements,omitempty"`
		Objective           *codingagent.Objective           `json:"objective,omitempty"`
		ArtifactBindings    []codingagent.ArtifactBinding    `json:"artifact_bindings,omitempty"`
		PlanningEnvironment *codingagent.PlanningEnvironment `json:"planning_environment,omitempty"`
		WorkspaceFiles      []string                         `json:"workspace_files,omitempty"`
		ConstructionFacts   []codingagent.ConstructionFact   `json:"construction_facts,omitempty"`
		QuestionContext     []codingagent.QuestionContext    `json:"question_context,omitempty"`
		ValidationNotes     []string                         `json:"validation_notes,omitempty"`
	}{call.Requirements, call.Objective, call.ArtifactBindings, call.PlanningEnvironment,
		call.ReadFiles, call.ConstructionFacts, call.QuestionContext, call.ValidationNotes}
}

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	if call.Role == codingagent.PlanningInitial {
		return planningInitialOutputSchema(object, text, strings)
	}
	if call.Role == codingagent.PlanningTarget {
		return object(map[string]any{
			"question": text, "base_revision": map[string]any{"type": "integer"}, "item_ids": strings,
		}, "question", "base_revision", "item_ids")
	}
	if call.Role == codingagent.PlanningMutation {
		return planningMutationOutputSchema(object, text, strings)
	}
	if call.Role == codingagent.PlanningCoverage || call.Role == codingagent.PlanningAudit {
		return object(map[string]any{
			"question": text, "base_revision": map[string]any{"type": "integer"},
			"covered": map[string]any{"type": "boolean"}, "suggestions": strings,
			"explanation": text,
		}, "question", "base_revision", "covered", "suggestions", "explanation")
	}
	if call.Role == codingagent.Execute {
		return object(map[string]any{"questions": strings}, "questions")
	}
	variants := []map[string]any{}
	for _, task := range agentReviewReferences(call) {
		criteria := make([]string, len(task.Criteria))
		for index, criterion := range task.Criteria {
			criteria[index] = criterion.Reference
		}
		variants = append(variants, object(map[string]any{
			"task_id": map[string]any{"type": "string", "enum": []string{task.Reference}},
			"file":    text, "requirement": map[string]any{"type": "string", "enum": criteria}, "observation": text,
		}, "task_id", "file", "requirement", "observation"))
	}
	finding := map[string]any{"anyOf": variants}
	if call.Role == codingagent.Review && len(variants) == 1 {
		finding = variants[0]
	}
	return object(map[string]any{"findings": map[string]any{"type": "array", "items": finding}, "questions": strings, "verification": text}, "findings", "questions")
}
