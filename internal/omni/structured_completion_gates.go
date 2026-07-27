package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func finalStructuredAnswer(payloadAnswer string, latest StructuredCommandObservation) string {
	if answer := strings.TrimSpace(payloadAnswer); answer != "" {
		return answer
	}
	if stdout := strings.TrimSpace(latest.Stdout); stdout != "" {
		return stdout
	}
	return strings.TrimSpace(latest.Stderr)
}

func rejectDoneForFinalAnswer(step int, _ string, answer string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	answer = strings.TrimSpace(answer)
	if structuredFinalAnswerGivesInstructionsInsteadOfCompletion(answer) {
		emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for instructional final answer", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"answer": truncateStructuredTimelineValue(answer),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "done rejected: final answer gives user instructions for an execution request; run the required command and report observed results",
		})
		result.Answer = ""
		return true
	}
	if structuredTextSuggestsFalseCapabilityLimit(answer) {
		emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for false capability limitation", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"answer": truncateStructuredTimelineValue(answer),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:             step,
			ExitCode:         1,
			CapabilityMemory: structuredRealtimeCapabilityMemory,
			Stderr:           "done rejected: final answer claims inability despite successful command evidence; answer from observed evidence or run another command",
		})
		result.Answer = ""
		return true
	}
	if structuredTextDefersEvidenceToFutureCommand(answer) {
		emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for deferred evidence", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"answer": truncateStructuredTimelineValue(answer),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "done rejected: final answer describes commands that should be run instead of using observed evidence; run the missing command or summarize only observed evidence",
		})
		result.Answer = ""
		return true
	}
	return false
}

func rejectDoneForObjectiveLedger(step int, ledger []StructuredObjective, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	pending := pendingStructuredObjectives(ledger)
	if len(pending) == 0 {
		return false
	}
	ids := structuredObjectiveIDs(pending)
	pendingText := strings.Join(ids, ",")
	repeatedCount := repeatedPrematureDoneRejectionCount(result.Observations, pendingText) + 1
	stderr := "done rejected: pending objective(s) remain: " + pendingText + "; run command(s) that satisfy the objective ledger before finishing"
	if repeatedCount >= maxRepeatedPrematureDoneRejections {
		stderr = fmt.Sprintf(
			"anti_loop: planner returned done=true %d times while the same pending objective(s) remain: %s. Stop returning done; choose a command or patch that satisfies the next pending objective.",
			repeatedCount,
			pendingText,
		)
	}
	emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for pending objectives", map[string]string{
		"step":               fmt.Sprintf("%d", step),
		"pending_objectives": pendingText,
		"repeat_count":       fmt.Sprintf("%d", repeatedCount),
	})
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		ExitCode: 1,
		Stderr:   stderr,
	})
	if repeatedCount >= maxRepeatedPrematureDoneRejections {
		emitStructuredCommandEvent(onEvent, "structured_done_loop_blocked", "Repeated premature done loop blocked", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"pending_objectives": pendingText,
			"repeat_count":       fmt.Sprintf("%d", repeatedCount),
		})
	}
	result.Answer = ""
	return true
}

func rejectTypedFinalGate(step int, workingDir string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if result == nil || len(result.WorkItems) == 0 {
		return false
	}
	gate := EvaluateTypedFinalGate(TypedFinalGateInput{
		Items:          result.WorkItems,
		CompletionDone: true,
		EmptyFiles:     findEmptyProjectFiles(workingDir, 12),
	})
	if gate.Passed {
		return false
	}
	emitStructuredCommandEvent(onEvent, "typed_final_gate_rejected", "Typed recursive work queue rejected completion", map[string]string{
		"step":   fmt.Sprintf("%d", step),
		"reason": truncateStructuredTimelineValue(gate.Reason),
	})
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		ExitCode: 1,
		Stderr:   "typed final gate rejected completion: " + gate.Reason,
	})
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	result.Answer = ""
	return true
}

func reconcileStructuredObjectiveLedgerFromWorkItems(ledger []StructuredObjective, items []ObjectiveWorkItem) []StructuredObjective {
	if len(ledger) == 0 || len(items) == 0 {
		return ledger
	}
	passed := map[string]ObjectiveWorkItem{}
	collectPassedObjectiveWorkItems(items, passed)
	if len(passed) == 0 {
		return ledger
	}
	out := mergeStructuredObjectiveLedger(nil, ledger)
	for i := range out {
		if structuredObjectiveSatisfied(out[i]) {
			continue
		}
		item, ok := passed[out[i].ID]
		if !ok {
			continue
		}
		out[i].Status = "satisfied"
		out[i].Evidence = objectiveEvidenceSummaryFromWorkItem(item)
	}
	return out
}

func collectPassedObjectiveWorkItems(items []ObjectiveWorkItem, passed map[string]ObjectiveWorkItem) {
	for _, item := range items {
		if ValidateObjectiveWorkTree(item).Passed {
			passed[item.ID] = item
		}
		collectPassedObjectiveWorkItems(item.Children, passed)
	}
}

func objectiveEvidenceSummaryFromWorkItem(item ObjectiveWorkItem) string {
	parts := []string{}
	for _, evidence := range item.EvidenceRefs {
		switch evidence.Kind {
		case EvidenceKindCommand:
			if strings.TrimSpace(evidence.Command) != "" {
				parts = append(parts, evidence.Command)
			}
		case EvidenceKindFileDiff, EvidenceKindRead, EvidenceKindDeleteSafety:
			if strings.TrimSpace(evidence.Path) != "" {
				parts = append(parts, string(evidence.Kind)+":"+evidence.Path)
			} else if strings.TrimSpace(evidence.Command) != "" {
				parts = append(parts, evidence.Command)
			}
		}
	}
	if len(parts) == 0 {
		return "typed work item passed required evidence"
	}
	return truncateStructuredObservation(strings.Join(parts, "; "))
}

func structuredObjectiveLedgersEqual(a, b []StructuredObjective) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Status != b[i].Status || a[i].Kind != b[i].Kind || a[i].Evidence != b[i].Evidence {
			return false
		}
	}
	return true
}

func typedWorkQueuePassedForCompletion(result *CommandDecisionResult) bool {
	if result == nil || len(result.WorkItems) == 0 {
		return false
	}
	return CanDeclareGoalAchieved(result.WorkItems, true)
}

func requiresTypedWorkQueueCompletion(result *CommandDecisionResult) bool {
	return result != nil && len(result.WorkItems) > 0
}

func rejectCompletionCheckerWithoutTypedWorkQueue(step int, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if !requiresTypedWorkQueueCompletion(result) || typedWorkQueuePassedForCompletion(result) {
		return false
	}
	emitStructuredCommandEvent(onEvent, "completion_check_rejected_for_incomplete_typed_queue", "Completion checker cannot accept done while typed work queue is incomplete", map[string]string{
		"step": fmt.Sprintf("%d", step),
	})
	if result != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "done rejected: typed objective work queue is incomplete; natural-language completion claims cannot override missing evidence",
		})
		result.Answer = ""
	}
	return true
}

func architectObjectiveWorkItemsFromObservations(prompt, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) []ObjectiveWorkItem {
	if !hasImplementationArchitectProgress(observations) {
		return nil
	}
	contract := buildImplementationArchitectContract(prompt, "Implementation architect target root: "+architectTargetRootForWorkQueue(workingDir)+". Create or modify the actual project files.", workingDir, survey, observations)
	if !hasImplementationArchitectContract(contract) || len(contract.WorkQueue) == 0 {
		return nil
	}
	item := ObjectiveWorkItem{
		ID:          "implementation_architect",
		Kind:        WorkItemKindArchitect,
		Scope:       WorkItemScope{Root: filepath.Join(workingDir, contract.TargetRoot)},
		Instruction: "Complete the architect-managed implementation work queue",
		Status:      WorkItemStatusPending,
		Children:    architectChildrenFromContract("implementation_architect", contract, workingDir),
	}
	return ReconcileObjectiveWorkItemsFromObservations([]ObjectiveWorkItem{item}, observations)
}

func rejectDoneForValidator(step int, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) {
	emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected by completion validator", map[string]string{
		"step":   fmt.Sprintf("%d", step),
		"reason": "validator did not accept completion",
	})
	if result != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "done rejected: completion validator did not accept done=true; choose another command, gather missing evidence, or satisfy pending objectives",
		})
		result.Answer = ""
	}
}

func requestDependencyInstallApproval(ctx context.Context, step int, prompt, command string, validationErr error, specialist UserAssistanceSpecialist, onAsk StructuredCommandAskFunc, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if validationErr == nil || !structuredTextSuggestsScopeDrift(validationErr.Error()) || len(structuredDependencyInstallRequests(command)) == 0 {
		return false, nil
	}
	if result != nil && dependencyInstallPreviouslyApproved(command, result.Observations) {
		emitStructuredCommandEvent(onEvent, "structured_user_input_reused", "Reused prior dependency install approval", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
		})
		return true, nil
	}
	question, err := buildDependencyInstallApprovalQuestion(ctx, step, prompt, command, validationErr.Error(), specialist)
	if err != nil {
		return false, err
	}
	emitStructuredCommandEvent(onEvent, "structured_user_input_requested", "Dependency install requires user approval", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"command":  truncateStructuredTimelineValue(command),
		"question": truncateStructuredTimelineValue(question),
		"reason":   truncateStructuredTimelineValue(validationErr.Error()),
	})
	if onAsk == nil {
		return false, UserInputRequiredError{Question: question}
	}
	answer, err := onAsk(ctx, question)
	if err != nil {
		if isStructuredUserInputCancelled(ctx, err) {
			markStructuredUserInputCancelled(step, question, onEvent, result)
		}
		return false, err
	}
	answer = strings.TrimSpace(answer)
	if result != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:         step,
			ExitCode:     0,
			Question:     question,
			UserResponse: truncateStructuredObservation(answer),
		})
	}
	approved := userApprovedDependencyInstall(answer)
	emitStructuredCommandEvent(onEvent, "structured_user_input_received", "Dependency install approval response received", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"approved": fmt.Sprintf("%t", approved),
	})
	return approved, nil
}

func dependencyInstallPreviouslyApproved(command string, observations []StructuredCommandObservation) bool {
	command = normalizeStructuredCommandForComparison(command)
	for _, obs := range observations {
		if strings.TrimSpace(obs.Question) == "" || !userApprovedDependencyInstall(obs.UserResponse) {
			continue
		}
		question := obs.Question
		if strings.Contains(question, "Command: "+command) || strings.Contains(normalizeStructuredCommandForComparison(question), command) {
			return true
		}
	}
	return false
}

func buildDependencyInstallApprovalQuestion(ctx context.Context, step int, prompt, command, reason string, specialist UserAssistanceSpecialist) (string, error) {
	packages := []string{}
	for _, request := range structuredDependencyInstallRequests(command) {
		packages = append(packages, request.Packages...)
	}
	packages = cleanStringList(packages)
	if specialist != nil {
		decision, err := specialist.BuildUserAssistanceQuestion(ctx, UserAssistanceInput{
			Step:       step,
			Kind:       "dependency_install_approval",
			Command:    strings.TrimSpace(command),
			Reason:     strings.TrimSpace(reason),
			Packages:   packages,
			UserPrompt: prompt,
		})
		if err != nil {
			return "", err
		}
		if question := strings.TrimSpace(decision.Question); question != "" {
			return question, nil
		}
	}
	if len(packages) == 0 {
		return fmt.Sprintf("Allow this dependency install command for the current task?\nCommand: %s\nReason: %s", strings.TrimSpace(command), strings.TrimSpace(reason)), nil
	}
	return fmt.Sprintf("Allow Omnidex to install these dependencies for the current task: %s?\nCommand: %s\nReason: %s", strings.Join(packages, ", "), strings.TrimSpace(command), strings.TrimSpace(reason)), nil
}

func userApprovedDependencyInstall(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "approved", "approve", "allow", "allowed", "ok", "okay", "sure", "do it", "proceed":
		return true
	default:
		return false
	}
}

func repairRejectedDoneWithPlanner(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, rejectedResp OllamaChatResponse, rejectedPayload StructuredCommandPayload, checkResult completionCheckRunResult, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, ledger *[]StructuredObjective, result *CommandDecisionResult) (bool, error) {
	if client == nil || !checkResult.Ran || checkResult.Accepted {
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "completion_repair_started", "Planner received completion-check feedback for local repair", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"reason":  truncateStructuredTimelineValue(checkResult.Check.Reason),
		"command": truncateStructuredTimelineValue(rejectedPayload.Command),
	})
	repairReq := buildCompletionRejectedPlannerRepairRequest(baseReq, prompt, rejectedResp.Content, rejectedPayload, checkResult, *ledger, result.Observations, cfg.CurrentWorkingDirectory)
	nextResp, err := requestStructuredCommandPayload(ctx, client, repairReq, step, onEvent)
	if err != nil {
		return false, err
	}
	nextPayload, err := ParseStructuredCommandPayload(nextResp.Content)
	if err != nil {
		return false, err
	}
	nextPayload.Command = normalizeStructuredCommand(nextPayload.Command)
	*ledger = mergePlannerObjectiveLedger(step, *ledger, nextPayload.ObjectiveLedger, result.Observations, cfg.CurrentWorkingDirectory, onEvent)
	result.ObjectiveLedger = *ledger
	emitStructuredCommandEvent(onEvent, "completion_repair_payload_received", "Planner returned completion-repair payload", map[string]string{
		"step":               fmt.Sprintf("%d", step),
		"done":               fmt.Sprintf("%t", nextPayload.Done),
		"ask":                fmt.Sprintf("%t", nextPayload.Ask),
		"tool":               truncateStructuredTimelineValue(nextPayload.Tool),
		"command":            truncateStructuredTimelineValue(nextPayload.Command),
		"pending_objectives": pendingStructuredObjectiveIDs(*ledger),
	})
	if isPatchToolDelegation(nextPayload) {
		if err := validateStructuredCommandForTaskMode(nextPayload.Command, nextPayload.Patch, worksiteSurvey.TaskMode); err != nil {
			appendRejectedShellProposalObservation(step, "patch.apply", err, "record incidental findings without repair in research-only mode", result)
			emitStructuredCommandEvent(onEvent, "research_only_mutation_rejected", "Completion repair patch rejected by research-only mode", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": err.Error(),
			})
			return false, nil
		}
		if err := runStructuredPatchApply(ctx, step, nextPayload.Patch, cfg.CurrentWorkingDirectory, stdout, stderr, onEvent, result); err != nil {
			return false, err
		}
		emitStructuredCommandEvent(onEvent, "completion_repair_accepted", "Completion repair executed patched action", map[string]string{"step": fmt.Sprintf("%d", step)})
		return true, nil
	}
	if isShellToolDelegation(nextPayload) {
		proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, nextPayload.ToolTask, cfg, worksiteSurvey, ledger, onEvent, onAsk, result)
		if err != nil || !ok {
			return ok, err
		}
		if err := runStructuredPayloadCommand(ctx, step, proposal.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
			return false, err
		}
		emitStructuredCommandEvent(onEvent, "completion_repair_accepted", "Completion repair executed delegated shell action", map[string]string{"step": fmt.Sprintf("%d", step)})
		return true, nil
	}
	if nextPayload.Done || strings.TrimSpace(nextPayload.Command) == "" {
		return false, nil
	}
	if err := validateStructuredCommandForRunWithSurvey(nextPayload.Command, result.Observations, cfg.CurrentWorkingDirectory, *ledger, worksiteSurvey); err != nil {
		emitStructuredCommandEvent(onEvent, "completion_repair_rejected", "Completion repair payload rejected by command validation", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(nextPayload.Command),
			"reason":  err.Error(),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:             step,
			RejectedCommand:  truncateStructuredObservation(nextPayload.Command),
			CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(nextPayload.Command, err.Error()),
			ExitCode:         1,
			Stderr:           "completion repair command rejected: " + err.Error(),
		})
		return false, nil
	}
	if err := runStructuredPayloadCommand(ctx, step, nextPayload.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
		return false, err
	}
	emitStructuredCommandEvent(onEvent, "completion_repair_accepted", "Completion repair executed planner command", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"command": truncateStructuredTimelineValue(nextPayload.Command),
	})
	return true, nil
}

func buildCompletionRejectedPlannerRepairRequest(baseReq OllamaChatRequest, prompt, rejectedResponse string, rejectedPayload StructuredCommandPayload, checkResult completionCheckRunResult, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string) OllamaChatRequest {
	req := baseReq
	req.Messages = append([]OllamaMessage(nil), baseReq.Messages...)
	payload := struct {
		CurrentPrompt           string                         `json:"current_prompt"`
		RejectedPayload         string                         `json:"rejected_payload"`
		RejectedDone            bool                           `json:"rejected_done"`
		CandidateAnswer         string                         `json:"candidate_answer"`
		CompletionDone          bool                           `json:"completion_done"`
		CompletionReason        string                         `json:"completion_reason"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		PendingObjectiveIDs     []string                       `json:"pending_objective_ids,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations,omitempty"`
		RepairRules             []string                       `json:"repair_rules"`
	}{
		CurrentPrompt:           prompt,
		RejectedPayload:         truncateStructuredObservation(rejectedResponse),
		RejectedDone:            rejectedPayload.Done,
		CandidateAnswer:         rejectedPayload.Answer,
		CompletionDone:          checkResult.Check.Done,
		CompletionReason:        checkResult.Check.Reason,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(workingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		PendingObjectiveIDs:     structuredObjectiveIDs(pendingStructuredObjectives(ledger)),
		Observations:            compactStructuredObservationsForContext(observations, 8, 650),
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"The completion checker rejected done=true; do not return done=true again unless new command evidence is added first.",
			"Use completion_reason and pending_objective_ids to choose the next concrete command, tool delegation, or patch.",
			"Gather missing evidence, satisfy pending objectives, or verify the latest mutation.",
			"Do not repeat the rejected done payload.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"completion_reason":"repair rejected done=true"}`)
	}
	req.Messages = append(req.Messages,
		OllamaMessage{Role: "assistant", Content: strings.TrimSpace(rejectedResponse)},
		OllamaMessage{Role: "user", Content: string(blob)},
	)
	return req
}

func repeatedPrematureDoneRejectionCount(observations []StructuredCommandObservation, pendingText string) int {
	pendingText = strings.TrimSpace(pendingText)
	if pendingText == "" {
		return 0
	}
	count := 0
	for i := len(observations) - 1; i >= 0; i-- {
		stderr := strings.TrimSpace(observations[i].Stderr)
		if !strings.Contains(stderr, "done rejected: pending objective(s) remain:") &&
			!strings.Contains(stderr, "anti_loop: planner returned done=true") {
			continue
		}
		if !strings.Contains(stderr, pendingText) {
			continue
		}
		count++
	}
	return count
}

func latestPrematureDoneLoopBlocked(observations []StructuredCommandObservation) bool {
	if len(observations) == 0 {
		return false
	}
	latest := observations[len(observations)-1]
	return latest.ExitCode != 0 && strings.Contains(latest.Stderr, "anti_loop: planner returned done=true")
}

func handleStructuredRepeatedCommandValidation(step int, command string, validationErr error, ledger *[]StructuredObjective, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if validationErr == nil || result == nil || ledger == nil {
		return false
	}
	if errors.Is(validationErr, errRepeatedSuccessfulStructuredCommand) {
		previous, ok := previousSuccessfulStructuredCommandObservation(command, result.Observations)
		if !ok {
			return false
		}
		before := pendingStructuredObjectiveIDs(*ledger)
		*ledger = reconcileStructuredObjectiveLedgerFromObservation(step, *ledger, previous, onEvent)
		result.ObjectiveLedger = *ledger
		workingDir := firstNonEmpty(result.TargetRoot, previous.CWD)
		reconciled := RunSuccessReconciliation(SuccessReconciliationInput{
			LatestObservation: &previous,
			ChildJobID:        previous.ChildJobID,
			ObjectiveID:       previous.ObjectiveID,
			ObjectiveLedger:   *ledger,
			ChildJobs:         result.ChildJobs,
			WorkingDirectory:  workingDir,
			Observations:      result.Observations,
		})
		result.ChildJobs = reconciled.ChildJobs
		*ledger = reconciled.ObjectiveLedger
		result.ObjectiveLedger = *ledger
		emitSuccessReconciliationEvents(onEvent, reconciled.Events)
		after := pendingStructuredObjectiveIDs(*ledger)
		emitStructuredCommandEvent(onEvent, "structured_repeat_success_reconciled", "Repeated successful command skipped and used as completion evidence", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"command":            truncateStructuredTimelineValue(command),
			"child_job_id":       previous.ChildJobID,
			"pending_before":     before,
			"pending_objectives": after,
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:            step,
			Command:         "SKIPPED_REPEAT_SUCCESS: " + truncateStructuredObservation(command),
			ExitCode:        0,
			Stdout:          "already_completed: command already succeeded earlier; objective ledger reconciled from prior completed-action evidence",
			RejectedCommand: truncateStructuredObservation(command),
		})
		return true
	}
	if errors.Is(validationErr, errRepeatedFailedStructuredCommand) {
		count := repeatedRejectedCommandCount(command, result.Observations) + 1
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:            step,
			RejectedCommand: truncateStructuredObservation(command),
			ExitCode:        1,
			Stderr: fmt.Sprintf(
				"anti_loop: command rejected again after prior failure/rejection count=%d; this is evidence for correction, not a completed action. Check completed_actions, inspect current files, use patch.apply for source edits, or revise the objective ledger from observed evidence.",
				count,
			),
		})
		emitStructuredCommandEvent(onEvent, "structured_command_loop_blocked", "Repeated failed command routed to recovery by anti-loop guard", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
			"count":   fmt.Sprintf("%d", count),
		})
		return true
	}
	return false
}

func rejectCommandRepeatedByActiveChildJob(step int, command string, childJobs []ChildJob, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, []ChildJob) {
	index := activeChildJobIndex(childJobs)
	if index < 0 {
		index = firstNonTerminalChildJobIndex(childJobs)
	}
	if index < 0 || !ChildJobShouldRejectRepeat(childJobs[index], command) {
		return false, childJobs
	}
	updated := cloneChildJobs(childJobs)
	obs := StructuredCommandObservation{
		Step:            step,
		RejectedCommand: truncateStructuredObservation(command),
		ExitCode:        1,
		Stderr:          "child_job_attempt_rejected: command repeats a failed/rejected attempt for active child job " + updated[index].ID + "; choose a different action that addresses the failure packet",
	}
	updated[index] = AppendChildJobAttemptWithContext(updated[index], obs, "runtime", "child_job_loop", "", "")
	result.Observations = append(result.Observations, obs)
	emitStructuredCommandEvent(onEvent, "child_job_attempt_repeat_rejected", "Child job attempt ledger rejected repeated failed action", map[string]string{
		"step":         fmt.Sprintf("%d", step),
		"child_job_id": updated[index].ID,
		"command":      truncateStructuredTimelineValue(command),
	})
	return true, updated
}
