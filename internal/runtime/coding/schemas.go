package coding

type CodingPlan struct {
	Goal  string              `json:"goal"`
	Tasks []CodingPlannerTask `json:"tasks"`
}

type CodingPlannerTask struct {
	ID              string   `json:"id"`
	Objective       string   `json:"objective"`
	SuccessCriteria []string `json:"success_criteria"`
	Scope           []string `json:"scope"`
}

type ArchitectQueue struct {
	TaskID      string           `json:"task_id"`
	Reads       []ReadStep       `json:"reads"`
	Tests       []ChangeStep     `json:"tests"`
	Writes      []ChangeStep     `json:"writes"`
	Deletes     []ChangeStep     `json:"deletes"`
	Validations []ValidationStep `json:"validations"`
}

func (q ArchitectQueue) Changes() []ChangeStep {
	out := make([]ChangeStep, 0, len(q.Tests)+len(q.Writes)+len(q.Deletes))
	out = append(out, q.Tests...)
	out = append(out, q.Writes...)
	out = append(out, q.Deletes...)
	return out
}

type ReadStep struct {
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"`
	Intent string `json:"intent"`
}

type ChangeStep struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Path         string `json:"path,omitempty"`
	Intent       string `json:"intent"`
	Validator    string `json:"validator"`
	RequiresTest bool   `json:"requires_test,omitempty"`
}

type ValidationStep struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Intent string `json:"intent"`
}

type ValidationResult struct {
	Status     string   `json:"status"`
	Evidence   []string `json:"evidence"`
	Violations []string `json:"violations"`
	Reason     string   `json:"reason,omitempty"`
}

type EmptyFileReport struct {
	Files []string `json:"files"`
}

type PlannerDisposition struct {
	Complete bool                       `json:"complete"`
	Actions  []PlannerDispositionAction `json:"actions"`
}

type PlannerDispositionAction struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}
