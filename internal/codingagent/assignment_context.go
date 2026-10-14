package codingagent

import "fmt"

// Narrow roles receive one finalized step. Parent objectives and aggregate
// workloads remain with their code-owned consumers, never the executor.
func (call Assignment) ValidateTaskContext() error {
	if call.Role != Execute && call.Role != Review {
		return nil
	}
	if call.Task == nil || call.Objective != nil || call.Workload != nil || len(call.Requirements) != 0 || len(call.ArtifactBindings) != 0 || call.PlanningEnvironment != nil || len(call.ConstructionFacts) != 0 {
		return fmt.Errorf("narrow agent context requires one task and excludes parent context")
	}
	allowed := uniqueFiles(append(append([]string{}, call.Task.ReadFiles...), call.Task.WriteFiles...))
	if call.Role == Execute {
		allowed = uniqueFiles(append(allowed, call.Task.DeleteFiles...))
	}
	if len(call.ReadFiles) != len(allowed) {
		return fmt.Errorf("narrow agent context changed the task's exact read files")
	}
	for _, file := range call.ReadFiles {
		if !contains(allowed, file) {
			return fmt.Errorf("narrow agent context includes an unrelated file")
		}
	}
	for _, finding := range call.Findings {
		findingFiles := uniqueFiles(append(allowed, call.Task.DeleteFiles...))
		if finding.TaskID != call.Task.ID || finding.File != "" && !contains(findingFiles, finding.File) {
			return fmt.Errorf("narrow agent context includes an unrelated finding")
		}
	}
	if call.Role == Execute {
		if len(call.DeleteFiles) != len(call.Task.DeleteFiles) {
			return fmt.Errorf("narrow execution context changed the task's exact delete files")
		}
		for _, file := range call.DeleteFiles {
			if !contains(call.Task.DeleteFiles, file) {
				return fmt.Errorf("narrow execution context includes an unrelated delete file")
			}
		}
	} else if len(call.DeleteFiles) != 0 {
		return fmt.Errorf("narrow review context received delete authority")
	}
	for _, check := range call.Checks {
		known := false
		for _, expected := range call.Task.Checks {
			known = known || expected.ID == check.ID
		}
		if !known {
			return fmt.Errorf("narrow agent context includes an unrelated check")
		}
	}
	return nil
}
