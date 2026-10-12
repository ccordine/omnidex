package codex

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

func renderAgentAssignment(call codingagent.Assignment) (string, error) {
	encoded, err := json.Marshal(call)
	if err != nil {
		return "", err
	}
	if len(encoded) > llm.MaxExactPreparedModelInputBytes {
		return "", fmt.Errorf("scoped agent assignment exceeds its context boundary")
	}
	var instruction string
	switch call.Role {
	case codingagent.Plan:
		instruction = "Decompose only the finalized requirements into ordered narrow execution tasks, identified by requirement_ids. Resolve artifact references using the supplied code-owned bindings; spell the actual relevant values in each task rather than leaving unresolved tokens. Honor the supplied construction facts in the relevant tasks. Each task names exact read/write files, narrow acceptance criteria, preceding dependencies, and executable checks that actually test the behavior. Derive acceptance criteria only from the frozen behavioral requirements and explicit construction facts: observable behavior, required artifacts, and meaningful verification. Choosing a development method or implementation order does not authorize an additional acceptance obligation. Require historical process evidence only when the supplied construction facts explicitly require that process. Existing read dependencies must come from the file catalog. When new_files_allowed is true you are authorized to declare new source/test/documentation paths inside the selected workspace, even when the catalog is empty; execution grants are created only after code validates your plan. Planning itself remains read-only. Choose exact file names unless an explicit binding fixes them; do not ask the user to perform planning. Use available executables for offline checks. Tasks receive no original conversation. Include only dependencies and context needed for each task. Return concrete questions only for information or authority absent from this context. Preserve the finalized requirements; do not reinterpret a user request, execute the plan or add product scope. Verification has read-only source and writable disposable caches; network access is disabled."
	case codingagent.Execute:
		instruction = "Fulfill this task completely in its granted files. Edit source using the omnidex_write_file tool, which writes only this task's allowed files through Omnidex. Native shell commands can read source and write disposable tool caches; they cannot edit source. Run the task's checks. Address only the supplied findings. Report concrete questions if required information or authority is missing. Your report is not completion authority."
	case codingagent.Review:
		instruction = "Independently inspect this narrow task's actual files and checks against its task acceptance criteria. Assess observable behavior, required artifacts, and meaningful verification. A development method, preferred order, or historical process is not an acceptance obligation unless the task explicitly requires it. Existing tests do not by themselves require historical evidence of when they were written or run. Do not require sibling tasks to be complete before accepting this task. Validate that checks meaningfully exercise the task; a no-op or superficial check is not proof. Recheck earlier findings. Return only concrete defects bound to an exact task acceptance criterion, or questions. For each finding, select one verbatim requirement from the schema-enumerated criteria for that task; do not paraphrase or combine criterion labels. Put the defect's explanation in observation. Do not invent requirements or change files."
	case codingagent.OverallReview:
		instruction = "Inspect all completed tasks together against the authoritative objective, every frozen parent requirement and workload checks. Check completeness, integration and omitted requested behavior. Assess the requested outcome and verification; do not add a development method, preferred order, or historical process obligation that the authoritative objective does not require. Bind observed defects to the responsible task and an exact task criterion or bound parent outcome. For each finding, select one verbatim requirement from the schema-enumerated criteria for that task; do not paraphrase or combine criterion labels. Put the defect's explanation in observation. Return concrete questions if the frozen requirements omit the objective or correction needs files outside the task grant. Do not add product scope or change files."
	default:
		return "", fmt.Errorf("unknown agent role")
	}
	return instruction + "\n\nAssignment:\n" + string(encoded), nil
}

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	check := object(map[string]any{"id": text, "argv": strings}, "id", "argv")
	checks := map[string]any{"type": "array", "items": check}
	if call.Role == codingagent.Plan {
		task := object(map[string]any{"id": text, "goal": text, "acceptance": strings, "read_files": strings, "write_files": strings, "dependencies": strings, "requirement_ids": strings, "checks": checks}, "id", "goal", "acceptance", "read_files", "write_files", "dependencies", "requirement_ids", "checks")
		return object(map[string]any{"tasks": map[string]any{"type": "array", "items": task}, "checks": checks, "questions": strings}, "tasks", "checks", "questions")
	}
	if call.Role == codingagent.Execute {
		return object(map[string]any{"questions": strings}, "questions")
	}
	tasks := []codingagent.Task{}
	if call.Role == codingagent.Review && call.Task != nil {
		tasks = append(tasks, *call.Task)
	}
	if call.Role == codingagent.OverallReview && call.Workload != nil {
		tasks = call.Workload.Tasks
	}
	variants := []map[string]any{}
	for _, task := range tasks {
		criteria := agentReviewCriteria(task, call)
		variants = append(variants, object(map[string]any{
			"task_id": map[string]any{"type": "string", "enum": []string{task.ID}},
			"file":    text, "requirement": map[string]any{"type": "string", "enum": criteria}, "observation": text,
		}, "task_id", "file", "requirement", "observation"))
	}
	finding := map[string]any{"anyOf": variants}
	if call.Role == codingagent.Review && len(variants) == 1 {
		finding = variants[0]
	}
	return object(map[string]any{"findings": map[string]any{"type": "array", "items": finding}, "questions": strings}, "findings", "questions")
}

func agentReviewCriteria(task codingagent.Task, call codingagent.Assignment) []string {
	criteria, seen := []string{}, map[string]bool{}
	appendCriterion := func(value string) {
		if !seen[value] {
			criteria = append(criteria, value)
			seen[value] = true
		}
	}
	for _, criterion := range task.Acceptance {
		appendCriterion(criterion)
	}
	if call.Role == codingagent.OverallReview {
		bound := map[string]bool{}
		for _, id := range task.RequirementIDs {
			bound[id] = true
		}
		for _, requirement := range call.Requirements {
			if bound[requirement.ID] {
				appendCriterion(requirement.Outcome)
			}
		}
	}
	return criteria
}
