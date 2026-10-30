package codingagent

import (
	"fmt"
)

// Narrow roles receive one finalized step. Parent objectives and aggregate
// workloads remain with their code-owned consumers, never the executor.
func (call Assignment) ValidateTaskContext() error {
	if call.ReviewFeedback != "" && (call.Role != Execute || call.Task == nil || call.Session == nil || call.Session.ThreadID == "") {
		return fmt.Errorf("review feedback requires its existing executor conversation")
	}
	if (call.Role == Review || call.Role == ReviewResponse) && call.Task == nil && call.Objective != nil {
		return nil
	}
	if call.ExecutionReport != nil && ((call.Role != Review && call.Role != Execute) || call.ExecutionReport.CallID < 1 || call.ExecutionReport.Response == "") {
		return fmt.Errorf("execution evidence requires a bound task review or retained execution")
	}
	if !call.Role.IsPlanning() && (call.Continuation != nil || len(call.CompletedTaskIDs) != 0 || call.NewerOwnerDirection != nil) {
		return fmt.Errorf("task-local context excludes continuation planning and completed task inventory")
	}
	if call.Role == ReviewResponse || call.Role == ExecutionResponse {
		if call.Task == nil || call.Response == "" || call.Objective != nil ||
			len(call.ConstructionFacts) != 0 ||
			call.PlanningLedger != nil || len(call.PlanningConversation) != 0 || call.PlanningResponse != "" ||
			len(call.ReadFiles) != 0 || len(call.WriteFiles) != 0 || len(call.DeleteFiles) != 0 ||
			len(call.Findings) != 0 || call.Session != nil || call.Reply != "" || (call.Role == ReviewResponse && len(call.Clarifications) != 0) {
			return fmt.Errorf("response parser requires only its source response and task instructions when applicable")
		}
		return nil
	}
	if call.Role != Execute && call.Role != Review {
		return nil
	}
	if call.Task == nil || call.Objective != nil || len(call.ConstructionFacts) != 0 || call.PlanningLedger != nil || len(call.PlanningConversation) != 0 || call.PlanningResponse != "" {
		return fmt.Errorf("narrow agent context requires one task and excludes parent context")
	}
	if call.Role == Execute && call.Response != "" || call.Role == Review && ((call.Session != nil && !recoverableSession(call.Session) && call.Response == "") || (call.Response == "") != (call.Reply == "")) {
		return fmt.Errorf("task assignment includes unrelated response context")
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
	if call.Role == Execute {
		if len(call.DeleteFiles) != len(call.Task.DeleteFiles) {
			return fmt.Errorf("narrow execution context changed the task's exact delete files")
		}
		for _, file := range call.DeleteFiles {
			if !contains(call.Task.DeleteFiles, file) {
				return fmt.Errorf("narrow execution context includes an unrelated delete file")
			}
		}
	} else if len(call.WriteFiles) != 0 || len(call.DeleteFiles) != 0 {
		return fmt.Errorf("narrow review context received mutation authority")
	}
	return nil
}
