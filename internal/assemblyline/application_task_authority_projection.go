package assemblyline

// ApplicationTaskRuntimeAuthority is the immutable accepted-requirement
// projection available to runtime construction.
type ApplicationTaskRuntimeAuthority struct {
	WorkloadSHA256   string             `json:"workload_sha256"`
	TaskID           string             `json:"task_id"`
	RequirementID    string             `json:"requirement_id"`
	Surface          ApplicationSurface `json:"surface"`
	ProductQuote     string             `json:"product_quote"`
	RequirementQuote string             `json:"requirement_quote"`
}

// ApplicationTaskVerificationAuthority binds verification to the same exact
// accepted requirement without inventing a second acceptance contract.
type ApplicationTaskVerificationAuthority struct {
	WorkloadSHA256   string `json:"workload_sha256"`
	TaskID           string `json:"task_id"`
	RequirementID    string `json:"requirement_id"`
	RequirementQuote string `json:"requirement_quote"`
}

func ProjectApplicationTaskRuntimeAuthority(
	frozen FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskRuntimeAuthority, error) {
	context, err := ProjectApplicationTaskContext(frozen, taskID)
	if err != nil {
		return ApplicationTaskRuntimeAuthority{}, err
	}
	return ApplicationTaskRuntimeAuthority{
		WorkloadSHA256: context.WorkloadSHA256,
		TaskID:         context.Task.TaskID, RequirementID: context.Task.RequirementID,
		Surface: context.Surface, ProductQuote: context.ProductQuote,
		RequirementQuote: context.Task.RequirementQuote,
	}, nil
}

func ProjectApplicationTaskVerificationAuthority(
	frozen FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskVerificationAuthority, error) {
	context, err := ProjectApplicationTaskContext(frozen, taskID)
	if err != nil {
		return ApplicationTaskVerificationAuthority{}, err
	}
	return ApplicationTaskVerificationAuthority{
		WorkloadSHA256: context.WorkloadSHA256,
		TaskID:         context.Task.TaskID, RequirementID: context.Task.RequirementID,
		RequirementQuote: context.Task.RequirementQuote,
	}, nil
}
