package assemblyline

import "fmt"

func firstApplicationWorkloadCycle(draft ApplicationWorkloadDraft, indices map[string]int) int {
	state := make([]uint8, len(draft.Tasks))
	var visit func(int) int
	visit = func(index int) int {
		state[index] = 1
		for _, dependency := range draft.Tasks[index].DependsOn {
			dependencyIndex, exists := indices[dependency]
			if !exists {
				continue
			}
			switch state[dependencyIndex] {
			case 1:
				return index
			case 0:
				if cycle := visit(dependencyIndex); cycle >= 0 {
					return cycle
				}
			}
		}
		state[index] = 2
		return -1
	}
	for index := range draft.Tasks {
		if state[index] == 0 {
			if cycle := visit(index); cycle >= 0 {
				return cycle
			}
		}
	}
	return -1
}

func draftFromFrozenApplicationWorkload(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
) (ApplicationWorkloadDraft, error) {
	if len(frozen.Tasks) != len(input.Requirements) {
		return ApplicationWorkloadDraft{}, fmt.Errorf(
			"frozen application workload requires exactly %d tasks", len(input.Requirements),
		)
	}
	taskRequirements := make(map[string]string, len(frozen.Tasks))
	for index, task := range frozen.Tasks {
		wantTaskID := fmt.Sprintf("task_%03d", index+1)
		if task.ID != wantTaskID {
			return ApplicationWorkloadDraft{}, fmt.Errorf(
				"frozen application task %d identity must be %q", index, wantTaskID,
			)
		}
		wantRequirement := input.Requirements[index]
		if task.RequirementID != wantRequirement.ID || task.RequirementQuote != wantRequirement.SourceQuote {
			return ApplicationWorkloadDraft{}, fmt.Errorf(
				"frozen application task %s differs from accepted requirement authority", task.ID,
			)
		}
		taskRequirements[task.ID] = task.RequirementID
	}
	draft := ApplicationWorkloadDraft{
		Schema: ApplicationWorkloadDraftSchemaV1,
		Tasks:  make([]ApplicationWorkloadTaskDraft, 0, len(frozen.Tasks)),
	}
	for _, task := range frozen.Tasks {
		dependencies := make([]string, 0, len(task.DependsOn))
		for _, taskID := range task.DependsOn {
			requirementID, exists := taskRequirements[taskID]
			if !exists {
				return ApplicationWorkloadDraft{}, fmt.Errorf(
					"frozen application task %s has unknown dependency %q", task.ID, taskID,
				)
			}
			dependencies = append(dependencies, requirementID)
		}
		draft.Tasks = append(draft.Tasks, ApplicationWorkloadTaskDraft{
			RequirementID: task.RequirementID, Objective: task.Objective,
			RequiredBehaviors:  append([]string{}, task.RequiredBehaviors...),
			AcceptanceCriteria: append([]string{}, task.AcceptanceCriteria...),
			DependsOn:          dependencies,
		})
	}
	return draft, nil
}

func BuildApplicationWorkloadWaves(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
) ([][]string, error) {
	if err := ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		return nil, err
	}
	completed := make(map[string]struct{}, len(frozen.Tasks))
	remaining := len(frozen.Tasks)
	waves := make([][]string, 0, len(frozen.Tasks))
	for remaining > 0 {
		wave := make([]string, 0, remaining)
		for _, task := range frozen.Tasks {
			if _, done := completed[task.ID]; done {
				continue
			}
			ready := true
			for _, dependency := range task.DependsOn {
				if _, done := completed[dependency]; !done {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, task.ID)
			}
		}
		if len(wave) == 0 {
			return nil, fmt.Errorf("frozen application workload has no ready task wave")
		}
		for _, taskID := range wave {
			completed[taskID] = struct{}{}
			remaining--
		}
		waves = append(waves, wave)
	}
	return waves, nil
}

func ProjectApplicationTaskContext(
	input ApplicationWorkloadDraftInput,
	frozen FrozenApplicationWorkload,
	taskID string,
) (ApplicationTaskContext, error) {
	var zero ApplicationTaskContext
	if err := ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		return zero, err
	}
	tasks := make(map[string]FrozenApplicationTask, len(frozen.Tasks))
	for _, task := range frozen.Tasks {
		tasks[task.ID] = task
	}
	task, exists := tasks[taskID]
	if !exists {
		return zero, fmt.Errorf("application workload task %q is unknown", taskID)
	}
	dependencies := make([]ApplicationTaskDependencyContext, 0, len(task.DependsOn))
	for _, dependencyID := range task.DependsOn {
		dependency := tasks[dependencyID]
		dependencies = append(dependencies, ApplicationTaskDependencyContext{
			TaskID: dependency.ID, RequirementID: dependency.RequirementID,
			RequirementQuote: dependency.RequirementQuote,
		})
	}
	return ApplicationTaskContext{
		WorkloadSHA256: frozen.SHA256,
		Surface:        frozen.Surface, ProductQuote: frozen.ProductQuote,
		Task: ApplicationTaskContextTask{
			TaskID: task.ID, RequirementID: task.RequirementID,
			RequirementQuote: task.RequirementQuote, Objective: task.Objective,
			RequiredBehaviors:  append([]string{}, task.RequiredBehaviors...),
			AcceptanceCriteria: append([]string{}, task.AcceptanceCriteria...),
		},
		Dependencies: dependencies,
	}, nil
}
