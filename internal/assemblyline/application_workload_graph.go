package assemblyline

import "fmt"

func ProjectApplicationTaskContext(
	workload FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskContext, error) {
	var zero ApplicationTaskContext
	if err := ValidateFrozenApplicationWorkload(workload); err != nil {
		return zero, err
	}
	for _, task := range workload.Tasks {
		if task.ID != taskID {
			continue
		}
		return ApplicationTaskContext{
			WorkloadSHA256: workload.SHA256,
			Surface:        workload.Surface,
			ProductQuote:   workload.ProductQuote,
			Task: ApplicationTaskContextTask{
				TaskID: task.ID, RequirementID: task.RequirementID,
				RequirementQuote: task.RequirementQuote,
			},
		}, nil
	}
	return zero, fmt.Errorf("application workload task %q is unknown", taskID)
}
