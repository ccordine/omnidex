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
		instruction = "Decompose only the supplied objective into ordered narrow execution tasks, identified by requirement_ids. Resolve artifact references using the supplied code-owned bindings; spell the actual relevant values in each task rather than leaving unresolved tokens. Honor the supplied construction facts in the relevant tasks. Each task names exact read, create/edit, and delete files, narrow acceptance criteria, preceding dependencies, and one or more executable checks that actually test the behavior. The workload-level checks array must also contain one or more executable integration checks for the completed task set; it cannot be empty. Derive acceptance criteria only from the objective, its clarifications, and explicit construction facts: observable behavior, required artifacts, and meaningful verification. Choosing a development method or implementation order does not authorize an additional acceptance obligation. Existing read and delete dependencies must come from the file catalog. When new_files_allowed is true you are authorized to declare new source/test/documentation paths inside the selected workspace, even when the catalog is empty; execution grants are created only after code validates your plan and the user explicitly approves it. Planning itself remains read-only. Choose exact file names unless an explicit binding fixes them. Use available executables for offline checks. Each execution task receives only its frozen task context. Return concrete questions only for information or authority absent from this context. Preserve the objective; do not execute the plan or add product scope. Verification has read-only source and writable disposable caches; network access is disabled."
	case codingagent.Execute:
		instruction = "Fulfill this task completely in its granted files. Edit or create source using omnidex_write_file and delete only explicitly granted files using omnidex_delete_file. Both tools enforce the frozen task through Omnidex. Native shell commands can read source and write disposable tool caches; they cannot edit source. Run the task's checks. Address only the supplied findings. Report concrete questions if required information or authority is missing. Your report is not completion authority."
	case codingagent.Review:
		instruction = "Independently inspect this narrow task's actual files and checks against its task acceptance criteria. Assess observable behavior, required artifacts, required deletions, and meaningful verification. Execute independent checks for the task's declared input classes and ranges, including relevant representation boundaries, overflow and underflow. Assess formula operation order against the declared behavior and use observed independent results beyond the supplied passing examples. These checks verify existing criteria and add no requirements. A development method, preferred order, or historical process is not an acceptance obligation unless the task explicitly requires it. Existing tests do not by themselves require historical evidence of when they were written or run. Do not require sibling tasks to be complete before accepting this task. Validate that checks meaningfully exercise the task; a no-op or superficial check is not proof. Do not assume version control is present; run Git commands only when a .git directory exists. Recheck earlier findings. Return only concrete defects bound to an exact task acceptance criterion, or questions. For each finding, use the review reference table: put the task reference in task_id and one criterion reference listed for that task in requirement. Put the defect's explanation in observation. Do not invent requirements or change files."
	case codingagent.OverallReview:
		instruction = "Inspect all completed tasks together against the authoritative original objective, every frozen parent requirement and workload check. Check completeness, integration, testing, actual operation, and omitted requested behavior. Execute independent integration checks and assess whether verification covers the declared input classes and ranges across the completed tasks. Do not assume version control is present; run Git commands only when a .git directory exists. Assess the requested outcome and verification; do not add a development method, preferred order, or historical process obligation that the authoritative objective does not require. Bind observed defects to the responsible task and an exact task criterion or bound parent outcome. For each finding, use the review reference table: put the task reference in task_id and one criterion reference listed for that task in requirement. Put the defect's explanation in observation. Return concrete questions if the frozen requirements omit the objective or correction needs files outside the task grant. Do not add product scope or change files."
	default:
		return "", fmt.Errorf("unknown agent role")
	}
	if call.Role == codingagent.Review || call.Role == codingagent.OverallReview {
		instruction += " Omnidex mechanically limits execution source writes to each task's write_files through its validated writer; native source mounts are read-only. Files outside those grants cannot be changed by this job's execution. Assess unchanged-file obligations against that enforced boundary; do not require before/after history for files this execution cannot write. This call runs through Codex native Review mode. Return the exact review object described below directly when permitted. If native Review requires its standard review envelope, put that complete JSON object in overall_explanation and mirror every native finding in its referenced findings array. Do not leave defects only in free-form native findings or comments."
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

func agentOutputSchema(call codingagent.Assignment) map[string]any {
	text := map[string]any{"type": "string"}
	strings := map[string]any{"type": "array", "items": text}
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	check := object(map[string]any{"id": text, "argv": strings}, "id", "argv")
	checks := map[string]any{"type": "array", "items": check}
	if call.Role == codingagent.Plan {
		task := object(map[string]any{"id": text, "goal": text, "acceptance": strings, "read_files": strings, "write_files": strings, "delete_files": strings, "dependencies": strings, "requirement_ids": strings, "checks": checks}, "id", "goal", "acceptance", "read_files", "write_files", "delete_files", "dependencies", "requirement_ids", "checks")
		return object(map[string]any{"tasks": map[string]any{"type": "array", "items": task}, "checks": checks, "questions": strings}, "tasks", "checks", "questions")
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
