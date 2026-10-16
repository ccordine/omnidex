package codingagent

func PromptContext(call Assignment) any {
	if call.Role == Plan {
		// The current owner message is sent verbatim as the new native turn,
		// not minified or duplicated inside the shared-history projection.
		call.Objective = nil
	}
	if call.Role == ReviewResponse {
		return struct {
			ValidationNotes []string `json:"validation_notes,omitempty"`
		}{call.ValidationNotes}
	}
	if call.Role == ExecutionResponse {
		return struct{}{}
	}
	if call.Role == PlanningInitial || call.Role == PlanningMutation {
		// The new response is the parser's exact current input. Only its
		// existing ledger and owner-turn background belong in shared context.
		call.PlanningResponse = ""
	}
	if call.Role == Execute || call.Role == Review {
		var task *struct {
			ID                   string         `json:"id"`
			Goal                 string         `json:"goal"`
			Guidance             []TaskGuidance `json:"guidance"`
			KnownAffectedSystems []string       `json:"known_affected_systems"`
			Acceptance           []string       `json:"acceptance"`
			ReadFiles            []string       `json:"read_files"`
			WriteFiles           []string       `json:"write_files"`
			DeleteFiles          []string       `json:"delete_files"`
			Checks               []Check        `json:"checks"`
		}
		if call.Task != nil {
			task = &struct {
				ID                   string         `json:"id"`
				Goal                 string         `json:"goal"`
				Guidance             []TaskGuidance `json:"guidance"`
				KnownAffectedSystems []string       `json:"known_affected_systems"`
				Acceptance           []string       `json:"acceptance"`
				ReadFiles            []string       `json:"read_files"`
				WriteFiles           []string       `json:"write_files"`
				DeleteFiles          []string       `json:"delete_files"`
				Checks               []Check        `json:"checks"`
			}{call.Task.ID, call.Task.Goal, call.Task.Guidance, call.Task.KnownAffectedSystems,
				call.Task.Acceptance, call.Task.ReadFiles, call.Task.WriteFiles, call.Task.DeleteFiles, call.Task.Checks}
		}
		return struct {
			Task             any                 `json:"task"`
			Findings         []Finding           `json:"findings,omitempty"`
			Checks           []CheckResult       `json:"checks,omitempty"`
			ValidationNotes  []string            `json:"validation_notes,omitempty"`
			PreviousResponse string              `json:"previous_response,omitempty"`
			Reply            string              `json:"reply,omitempty"`
			Clarifications   []TaskClarification `json:"clarifications,omitempty"`
		}{task, call.Findings, call.Checks, call.ValidationNotes, call.Response, call.Reply, call.Clarifications}
	}
	return struct {
		Requirements         []Requirement          `json:"requirements,omitempty"`
		Objective            *Objective             `json:"objective,omitempty"`
		WorkspaceFiles       []string               `json:"workspace_files,omitempty"`
		ConstructionFacts    []ConstructionFact     `json:"construction_facts,omitempty"`
		ValidationNotes      []string               `json:"validation_notes,omitempty"`
		PlanningLedger       *PlanningLedgerContext `json:"planning_ledger,omitempty"`
		PlanningConversation []PlanningMessage      `json:"conversation,omitempty"`
		PlanningResponse     string                 `json:"planning_response,omitempty"`
		Continuation         *TaskContinuation      `json:"continuation,omitempty"`
		Clarifications       []TaskClarification    `json:"clarifications,omitempty"`
	}{call.Requirements, call.Objective,
		call.ReadFiles, call.ConstructionFacts, call.ValidationNotes,
		ActivePlanningLedgerContext(call.PlanningLedger, call.CompletedTaskIDs...), call.PlanningConversation, call.PlanningResponse, call.Continuation, call.Clarifications}
}
