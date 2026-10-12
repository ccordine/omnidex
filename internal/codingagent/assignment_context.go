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
	if len(call.ReadFiles) != len(allowed) {
		return fmt.Errorf("narrow agent context changed the task's exact read files")
	}
	for _, file := range call.ReadFiles {
		if !contains(allowed, file) {
			return fmt.Errorf("narrow agent context includes an unrelated file")
		}
	}
	for _, finding := range call.Findings {
		if finding.TaskID != call.Task.ID || finding.File != "" && !contains(allowed, finding.File) {
			return fmt.Errorf("narrow agent context includes an unrelated finding")
		}
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
