package codingagent

func PromptContext(call Assignment) any {
	if call.Role == Plan {
		tasks := []LedgerTask{}
		if call.PlanningLedger != nil {
			for _, task := range call.PlanningLedger.Tasks {
				if task.Disposition == "active" && !contains(call.CompletedTaskIDs, string(task.ID)) {
					tasks = append(tasks, task)
				}
			}
		}
		failure := ""
		if call.Continuation != nil {
			failure = call.Continuation.Reason
		}
		return struct {
			PrecedingIncompleteTasks []PlanningTaskPrerequisite `json:"preceding_incomplete_tasks,omitempty"`
			ItemScope                []ConstructionFact         `json:"item_scope,omitempty"`
			Tasks                    []LedgerTask               `json:"tasks"`
			Conversation             []PlanningMessage          `json:"conversation,omitempty"`
			IncompleteReason         string                     `json:"incomplete_reason,omitempty"`
			Clarifications           []TaskClarification        `json:"clarifications,omitempty"`
			NewerOwnerDirection      *PlanningDirection         `json:"newer_owner_direction,omitempty"`
			CompletedTaskIDs         []string                   `json:"completed_task_ids,omitempty"`
		}{call.PrecedingIncompleteTasks, call.ConstructionFacts, tasks, call.PlanningConversation, failure, call.Clarifications, call.NewerOwnerDirection, call.CompletedTaskIDs}
	}
	if call.Role == ReviewResponse || call.Role == ExecutionResponse {
		instructions := ""
		if call.Task != nil {
			instructions = call.Task.Instructions()
		} else if call.Objective != nil {
			instructions = call.Objective.Instruction
			for _, clarification := range call.Objective.Clarifications {
				instructions += "\n" + clarification
			}
		}
		var clarifications []TaskClarification
		if call.Role == ExecutionResponse {
			clarifications = call.Clarifications
		}
		return struct {
			TaskInstructions string              `json:"task_instructions"`
			Clarifications   []TaskClarification `json:"clarifications,omitempty"`
		}{instructions, clarifications}
	}
	if call.Role == Execute || call.Role == Review {
		type findingText struct {
			File        string `json:"file,omitempty"`
			Observation string `json:"observation"`
		}
		findings := make([]findingText, 0, len(call.Findings))
		for _, finding := range call.Findings {
			findings = append(findings, findingText{finding.File, finding.Observation})
		}

		return struct {
			ExecutionReport  *ExecutionReport    `json:"execution_report,omitempty"`
			Objective        *Objective          `json:"objective,omitempty"`
			Task             *Task               `json:"task"`
			Findings         []findingText       `json:"findings,omitempty"`
			PreviousResponse string              `json:"previous_response,omitempty"`
			Reply            string              `json:"reply,omitempty"`
			Clarifications   []TaskClarification `json:"clarifications,omitempty"`
		}{call.ExecutionReport, call.Objective, call.Task, findings, call.Response, call.Reply, call.Clarifications}
	}
	return struct {
		PrecedingIncompleteTasks []PlanningTaskPrerequisite `json:"preceding_incomplete_tasks,omitempty"`
		Objective                *Objective                 `json:"objective,omitempty"`
		WorkspaceFiles           []string                   `json:"workspace_files,omitempty"`
		ConstructionFacts        []ConstructionFact         `json:"construction_facts,omitempty"`
		ValidationNotes          []string                   `json:"validation_notes,omitempty"`
		PlanningLedger           *PlanningLedgerContext     `json:"planning_ledger,omitempty"`
		PlanningConversation     []PlanningMessage          `json:"conversation,omitempty"`
		Continuation             *TaskContinuation          `json:"continuation,omitempty"`
		Clarifications           []TaskClarification        `json:"clarifications,omitempty"`
		CompletedTaskIDs         []string                   `json:"completed_task_ids,omitempty"`
	}{call.PrecedingIncompleteTasks, call.Objective, call.ReadFiles, call.ConstructionFacts, call.ValidationNotes, ActivePlanningLedgerContext(call.PlanningLedger, call.CompletedTaskIDs...), call.PlanningConversation, call.Continuation, call.Clarifications, call.CompletedTaskIDs}
}
