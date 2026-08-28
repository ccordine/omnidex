package assemblyline

// ApplicationTaskRuntimeAuthority is the immutable task projection available
// to runtime construction. Verification criteria are deliberately absent.
type ApplicationTaskRuntimeAuthority struct {
	WorkloadSHA256    string             `json:"workload_sha256"`
	TaskID            string             `json:"task_id"`
	RequirementID     string             `json:"requirement_id"`
	Surface           ApplicationSurface `json:"surface"`
	ProductQuote      string             `json:"product_quote"`
	RequirementQuote  string             `json:"requirement_quote"`
	Objective         string             `json:"objective"`
	RequiredBehaviors []string           `json:"required_behaviors"`
}

// ApplicationTaskVerificationAuthority is the immutable task projection
// available to verification construction. Runtime product semantics are
// deliberately absent; code-owned binding identities connect the criteria to
// the frozen workload.
type ApplicationTaskVerificationAuthority struct {
	WorkloadSHA256     string   `json:"workload_sha256"`
	TaskID             string   `json:"task_id"`
	RequirementID      string   `json:"requirement_id"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

func ProjectApplicationTaskRuntimeAuthority(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskRuntimeAuthority, error) {
	context, err := ProjectApplicationTaskContext(input, frozen, taskID)
	if err != nil {
		return ApplicationTaskRuntimeAuthority{}, err
	}
	return ApplicationTaskRuntimeAuthority{
		WorkloadSHA256: context.WorkloadSHA256,
		TaskID:         context.Task.TaskID, RequirementID: context.Task.RequirementID,
		Surface: context.Surface, ProductQuote: context.ProductQuote,
		RequirementQuote: context.Task.RequirementQuote, Objective: context.Task.Objective,
		RequiredBehaviors: append([]string(nil), context.Task.RequiredBehaviors...),
	}, nil
}

func ProjectApplicationTaskVerificationAuthority(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskVerificationAuthority, error) {
	context, err := ProjectApplicationTaskContext(input, frozen, taskID)
	if err != nil {
		return ApplicationTaskVerificationAuthority{}, err
	}
	return ApplicationTaskVerificationAuthority{
		WorkloadSHA256: context.WorkloadSHA256,
		TaskID:         context.Task.TaskID, RequirementID: context.Task.RequirementID,
		AcceptanceCriteria: append([]string(nil), context.Task.AcceptanceCriteria...),
	}, nil
}
