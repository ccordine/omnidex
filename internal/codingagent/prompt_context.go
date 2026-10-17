package codingagent

func PromptContext(call Assignment) any {
	if call.Role == Plan {
		tasks := []string{}
		if call.PlanningLedger != nil {
			for _, task := range call.PlanningLedger.Tasks {
				if task.Disposition == "active" && !contains(call.CompletedTaskIDs, string(task.ID)) {
					tasks = append(tasks, task.Goal)
				}
			}
		}
		failure := ""
		if call.Continuation != nil {
			failure = call.Continuation.Reason
		}
		return struct {
			Tasks            []string            `json:"tasks"`
			Conversation     []PlanningMessage   `json:"conversation,omitempty"`
			IncompleteReason string              `json:"incomplete_reason,omitempty"`
			Clarifications   []TaskClarification `json:"clarifications,omitempty"`
		}{tasks, call.PlanningConversation, failure, call.Clarifications}
	}
	if call.Role == ReviewResponse || call.Role == ExecutionResponse {
		return struct{}{}
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
		var task any
		if call.Task != nil {
			task = struct {
				ReadFiles   []string `json:"read_files,omitempty"`
				WriteFiles  []string `json:"write_files,omitempty"`
				DeleteFiles []string `json:"delete_files,omitempty"`
			}{call.Task.ReadFiles, call.Task.WriteFiles, call.Task.DeleteFiles}
		}
		return struct {
			Task             any                 `json:"task"`
			Findings         []findingText       `json:"findings,omitempty"`
			PreviousResponse string              `json:"previous_response,omitempty"`
			Reply            string              `json:"reply,omitempty"`
			Clarifications   []TaskClarification `json:"clarifications,omitempty"`
		}{task, findings, call.Response, call.Reply, call.Clarifications}
	}
	return struct {
		Objective            *Objective             `json:"objective,omitempty"`
		WorkspaceFiles       []string               `json:"workspace_files,omitempty"`
		ConstructionFacts    []ConstructionFact     `json:"construction_facts,omitempty"`
		ValidationNotes      []string               `json:"validation_notes,omitempty"`
		PlanningLedger       *PlanningLedgerContext `json:"planning_ledger,omitempty"`
		PlanningConversation []PlanningMessage      `json:"conversation,omitempty"`
		Continuation         *TaskContinuation      `json:"continuation,omitempty"`
		Clarifications       []TaskClarification    `json:"clarifications,omitempty"`
	}{call.Objective, call.ReadFiles, call.ConstructionFacts, call.ValidationNotes, ActivePlanningLedgerContext(call.PlanningLedger, call.CompletedTaskIDs...), call.PlanningConversation, call.Continuation, call.Clarifications}
}
