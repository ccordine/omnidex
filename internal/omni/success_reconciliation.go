package omni

import (
	"fmt"
	"strings"
)

type SuccessReconciliationInput struct {
	LatestObservation       *StructuredCommandObservation
	CommandID               string
	ChildJobID              string
	ObjectiveID             string
	ObjectiveLedger         []StructuredObjective
	WorkQueue               []ObjectiveWorkItem
	ChildJobs               []ChildJob
	WorkingDirectory        string
	Observations            []StructuredCommandObservation
	RouteFiles              TaskRoute
	ProofContractPredicates []string
	ToolchainFeedback       ToolchainFeedback
}

type SuccessReconciliationOutput struct {
	ObjectiveLedger          []StructuredObjective
	WorkQueue                []ObjectiveWorkItem
	ChildJobs                []ChildJob
	SatisfiedObjectives      []string
	PassedEvidencePredicates []string
	FailedEvidencePredicates []string
	StaleRouteInvalidations  []string
	NextRequiredChildJob     *ChildJob
	NextAction               *ChildJobAction
	UnresolvedBlockers       []string
	RouteFiles               TaskRoute
	Events                   []SuccessReconciliationEvent
}

type SuccessReconciliationEvent struct {
	Type    string            `json:"type"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details,omitempty"`
}

func RunSuccessReconciliation(input SuccessReconciliationInput) SuccessReconciliationOutput {
	out := SuccessReconciliationOutput{
		ObjectiveLedger: cloneStructuredObjectiveLedger(input.ObjectiveLedger),
		WorkQueue:       cloneObjectiveWorkItems(input.WorkQueue),
		ChildJobs:       cloneChildJobs(input.ChildJobs),
		RouteFiles:      input.RouteFiles,
	}
	latestCommandID := strings.TrimSpace(input.CommandID)
	if latestCommandID == "" && input.LatestObservation != nil {
		latestCommandID = input.LatestObservation.CommandID
	}
	out.Events = append(out.Events, successReconciliationEvent("success_reconciliation_started", "Deterministic success reconciliation started", map[string]string{
		"command_id":   latestCommandID,
		"child_job_id": input.ChildJobID,
		"objective_id": input.ObjectiveID,
	}))

	for i := range out.ObjectiveLedger {
		objective := normalizeStructuredObjectiveOrOriginal(out.ObjectiveLedger[i])
		if !structuredObjectiveBlocksCompletion(objective) || structuredObjectiveSatisfied(objective) {
			out.ObjectiveLedger[i] = objective
			continue
		}
		passedAll, passed, failed := objectiveEvidenceStatus(objective, input.Observations, input.WorkingDirectory)
		for _, predicate := range passed {
			out.PassedEvidencePredicates = append(out.PassedEvidencePredicates, predicate)
			out.Events = append(out.Events, successReconciliationEvent("evidence_predicate_passed", "Evidence predicate passed", map[string]string{
				"objective_id": objective.ID,
				"predicate":    predicate,
			}))
		}
		for _, predicate := range failed {
			out.FailedEvidencePredicates = append(out.FailedEvidencePredicates, predicate)
			out.Events = append(out.Events, successReconciliationEvent("evidence_predicate_failed", "Evidence predicate failed", map[string]string{
				"objective_id": objective.ID,
				"predicate":    predicate,
			}))
		}
		if passedAll {
			objective.Status = "satisfied"
			objective.Evidence = "success_reconciliation:evidence_predicates"
			out.SatisfiedObjectives = append(out.SatisfiedObjectives, objective.ID)
			out.Events = append(out.Events, successReconciliationEvent("objective_satisfied_from_evidence", "Objective satisfied from deterministic evidence", map[string]string{
				"objective_id":       objective.ID,
				"passed_predicates":  strings.Join(passed, ","),
				"failed_predicates":  strings.Join(failed, ","),
				"command_id":         latestCommandID,
				"pending_objectives": pendingStructuredObjectiveIDs(out.ObjectiveLedger),
			}))
		} else if len(failed) > 0 {
			out.UnresolvedBlockers = append(out.UnresolvedBlockers, objective.ID+":"+strings.Join(failed, ","))
		}
		out.ObjectiveLedger[i] = objective
	}

	if len(out.WorkQueue) > 0 {
		out.WorkQueue = ReconcileObjectiveWorkItemsFromObservations(out.WorkQueue, input.Observations)
	}
	if len(out.ChildJobs) == 0 && len(out.WorkQueue) > 0 {
		out.ChildJobs = BuildChildJobsFromObjectiveWorkItems(out.WorkQueue)
	}
	if input.LatestObservation != nil && childJobObservationShouldCreateFailureAttempt(*input.LatestObservation) {
		if index := focusedChildJobIndexForAttempt(out.ChildJobs, input.ChildJobID); index >= 0 {
			if !childJobAttemptAlreadyRecorded(out.ChildJobs[index], *input.LatestObservation) {
				out.ChildJobs[index] = AppendChildJobAttemptWithContext(out.ChildJobs[index], *input.LatestObservation, "runtime", "child_job_loop", "", input.WorkingDirectory)
			}
		}
	}
	childLoop := RunChildJobLoopOnce(ChildJobLoopInput{
		Jobs:             out.ChildJobs,
		WorkingDirectory: input.WorkingDirectory,
		Observations:     input.Observations,
		ObjectiveLedger:  out.ObjectiveLedger,
	})
	out.ChildJobs = childLoop.Jobs
	out.ObjectiveLedger = childLoop.ObjectiveLedger
	out.NextRequiredChildJob = childLoop.ActiveJob
	out.NextAction = childLoop.NextAction
	out.Events = append(out.Events, successEventsFromChildEvents(childLoop.Events)...)
	for _, job := range childLoop.Jobs {
		if job.Status != ChildJobStatusComplete {
			continue
		}
		out.Events = append(out.Events, successReconciliationEvent("child_job_satisfied_from_evidence", "Child job satisfied from deterministic evidence and scoped review", map[string]string{
			"child_job_id":        job.ID,
			"parent_objective_id": job.ParentObjectiveID,
			"terminal_reason":     job.TerminalReason,
		}))
		nextRoute := RouteFilesAfterChildCompletion(out.RouteFiles, job)
		if !taskRoutesLikelyFilesEqual(out.RouteFiles, nextRoute) {
			out.StaleRouteInvalidations = append(out.StaleRouteInvalidations, job.ID)
			out.Events = append(out.Events, successReconciliationEvent("route_context_invalidated", "Route context invalidated after child job mutation evidence", map[string]string{
				"child_job_id": job.ID,
			}))
			out.RouteFiles = nextRoute
		}
	}
	if out.NextRequiredChildJob != nil && out.NextRequiredChildJob.Status != ChildJobStatusComplete {
		out.Events = append(out.Events, successReconciliationEvent("next_child_job_selected", "Next required child job selected from reconciliation", map[string]string{
			"child_job_id": out.NextRequiredChildJob.ID,
			"status":       string(out.NextRequiredChildJob.Status),
		}))
	}
	out.Events = append(out.Events, successReconciliationEvent("success_reconciliation_completed", "Deterministic success reconciliation completed", map[string]string{
		"satisfied_objectives": strings.Join(out.SatisfiedObjectives, ","),
		"passed_predicates":    fmt.Sprintf("%d", len(out.PassedEvidencePredicates)),
		"failed_predicates":    fmt.Sprintf("%d", len(out.FailedEvidencePredicates)),
		"unresolved_blockers":  fmt.Sprintf("%d", len(out.UnresolvedBlockers)),
	}))
	return out
}

func childJobObservationShouldCreateFailureAttempt(obs StructuredCommandObservation) bool {
	return obs.ExitCode != 0 || strings.TrimSpace(obs.RejectedCommand) != "" || strings.Contains(strings.ToLower(obs.Stderr+"\n"+obs.Stdout), "partial_failure")
}

func childJobAttemptAlreadyRecorded(job ChildJob, obs StructuredCommandObservation) bool {
	commandOrPatch := strings.TrimSpace(firstNonEmpty(obs.Command, obs.RejectedCommand))
	for _, attempt := range job.AttemptLedger {
		if strings.TrimSpace(obs.CommandID) != "" && attempt.AttemptID == obs.CommandID {
			return true
		}
		if commandOrPatch != "" && normalizeStructuredCommandForComparison(attempt.CommandOrPatch) == normalizeStructuredCommandForComparison(commandOrPatch) {
			if attempt.Result == childJobAttemptResult(obs) && attempt.FailureKind == classifyChildJobFailureKind(obs) {
				return true
			}
		}
	}
	return false
}

func focusedChildJobIndexForAttempt(jobs []ChildJob, requestedID string) int {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID != "" {
		for i, job := range jobs {
			if job.ID == requestedID {
				return i
			}
		}
	}
	if i := activeChildJobIndex(jobs); i >= 0 {
		return i
	}
	return firstNonTerminalChildJobIndex(jobs)
}

func objectiveEvidenceStatus(objective StructuredObjective, observations []StructuredCommandObservation, workingDir string) (bool, []string, []string) {
	predicates := cleanStringList(objective.RequiredEvidence)
	if len(predicates) == 0 {
		if latest, ok := latestSuccessfulCommandObservation(observations); ok && structuredObservationSatisfiesObjective(latest, objective) {
			return true, []string{"observation_satisfies_objective:" + objective.ID}, nil
		}
		return false, nil, nil
	}
	passed := []string{}
	failed := []string{}
	for _, predicate := range predicates {
		if structuredEvidencePredicateSatisfied(predicate, observations, workingDir) {
			passed = append(passed, predicate)
		} else {
			failed = append(failed, predicate)
		}
	}
	return len(failed) == 0, passed, failed
}

func normalizeStructuredObjectiveOrOriginal(objective StructuredObjective) StructuredObjective {
	normalized, ok := normalizeStructuredObjective(objective)
	if !ok {
		return objective
	}
	return normalized
}

func successEventsFromChildEvents(events []ChildJobEvent) []SuccessReconciliationEvent {
	out := make([]SuccessReconciliationEvent, 0, len(events))
	for _, event := range events {
		out = append(out, SuccessReconciliationEvent{Type: event.Type, Summary: event.Summary, Details: event.Details})
	}
	return out
}

func emitSuccessReconciliationEvents(onEvent func(StructuredCommandEvent), events []SuccessReconciliationEvent) {
	for _, event := range events {
		emitStructuredCommandEvent(onEvent, event.Type, event.Summary, event.Details)
	}
}

func successReconciliationEvent(eventType, summary string, details map[string]string) SuccessReconciliationEvent {
	if details == nil {
		details = map[string]string{}
	}
	return SuccessReconciliationEvent{Type: eventType, Summary: summary, Details: details}
}

func cloneObjectiveWorkItems(items []ObjectiveWorkItem) []ObjectiveWorkItem {
	out := make([]ObjectiveWorkItem, len(items))
	copy(out, items)
	for i := range out {
		out[i].Scope.Paths = append([]string{}, out[i].Scope.Paths...)
		out[i].RequiredEvidence = append([]EvidenceKind{}, out[i].RequiredEvidence...)
		out[i].EvidencePredicates = append([]string{}, out[i].EvidencePredicates...)
		out[i].EvidenceRefs = append([]WorkItemEvidence{}, out[i].EvidenceRefs...)
		out[i].Children = cloneObjectiveWorkItems(out[i].Children)
	}
	return out
}

func taskRoutesLikelyFilesEqual(a, b TaskRoute) bool {
	left := cleanStringList(a.LikelyFiles)
	right := cleanStringList(b.LikelyFiles)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
