package omni

import (
	"context"
	"fmt"
	"strings"
)

type completionCheckRunResult struct {
	Ledger   []StructuredObjective
	Accepted bool
	Check    CompletionCheck
	Ran      bool
	Err      error
}

func runCompletionCheck(ctx context.Context, step int, prompt, currentWorkingDirectory string, ledger []StructuredObjective, minimalContext MinimalContext, observations []StructuredCommandObservation, candidateAnswer string, checker CompletionChecker, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent)) ([]StructuredObjective, bool, error) {
	result := runCompletionCheckDetailed(ctx, step, prompt, currentWorkingDirectory, ledger, minimalContext, observations, candidateAnswer, checker, worksiteSurvey, onEvent)
	return result.Ledger, result.Accepted, result.Err
}

func runCompletionCheckDetailed(ctx context.Context, step int, prompt, currentWorkingDirectory string, ledger []StructuredObjective, minimalContext MinimalContext, observations []StructuredCommandObservation, candidateAnswer string, checker CompletionChecker, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent)) completionCheckRunResult {
	if checker == nil {
		return completionCheckRunResult{Ledger: ledger}
	}
	check, err := checker.CheckCompletion(ctx, CompletionCheckInput{
		UserPrompt:              prompt,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(currentWorkingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		CompletedActions:        completedActionsFromState(ledger, observations),
		LoopState:               structuredLoopStateFromState(ledger, observations),
		MinimalContext:          normalizeMinimalContext(minimalContext),
		Observations:            observations,
		CandidateAnswer:         candidateAnswer,
		WorksiteSurvey:          worksiteSurvey,
	})
	if err != nil {
		wrapped := fmt.Errorf("completion checker failed: %w", err)
		emitStructuredCommandEvent(onEvent, "completion_check_failed", "Done-check specialist failed; structured execution stopped", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return completionCheckRunResult{Ledger: ledger, Err: wrapped}
	}
	filteredClaims := filterCompletionCheckerObjectiveClaims(step, ledger, filterObjectiveLedgerForWorksiteSurvey(check.ObjectiveLedger, worksiteSurvey), observations, currentWorkingDirectory, onEvent)
	updated := mergeStructuredObjectiveLedger(ledger, filteredClaims)
	validatorAccepted := check.Done && len(pendingStructuredObjectives(updated)) == 0
	if !check.Done && len(pendingStructuredObjectives(updated)) == 0 {
		updated = keepAtLeastOnePreviouslyPendingObjectiveOpen(ledger, updated)
	}
	emitStructuredCommandEvent(onEvent, "completion_check_completed", "Done-check specialist reviewed completion", map[string]string{
		"step":               fmt.Sprintf("%d", step),
		"done":               fmt.Sprintf("%t", check.Done),
		"reason":             truncateStructuredTimelineValue(check.Reason),
		"pending_objectives": pendingStructuredObjectiveIDs(updated),
	})
	return completionCheckRunResult{Ledger: updated, Accepted: validatorAccepted, Check: check, Ran: true}
}

func filterCompletionCheckerObjectiveClaims(step int, current, claims []StructuredObjective, observations []StructuredCommandObservation, workingDir string, onEvent func(StructuredCommandEvent)) []StructuredObjective {
	out := []StructuredObjective{}
	currentByID := map[string]StructuredObjective{}
	for _, objective := range current {
		currentByID[objective.ID] = objective
	}
	for _, claim := range claims {
		existing, known := currentByID[claim.ID]
		if !known {
			out = append(out, claim)
			continue
		}
		if !structuredObjectiveSatisfied(claim) || structuredObjectiveSatisfied(existing) || !structuredObjectiveBlocksCompletion(existing) {
			out = append(out, claim)
			continue
		}
		merged := existing
		merged.Status = claim.Status
		merged.Evidence = claim.Evidence
		if len(claim.RequiredEvidence) > 0 {
			merged.RequiredEvidence = claim.RequiredEvidence
		}
		if completionClaimHasDeterministicEvidence(merged, observations, workingDir) {
			out = append(out, merged)
			continue
		}
		emitStructuredCommandEvent(onEvent, "completion_check_claim_rejected_for_missing_evidence", "Completion-check objective claim rejected because deterministic evidence is missing", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"objective": claim.ID,
			"evidence":  truncateStructuredTimelineValue(claim.Evidence),
		})
		out = append(out, existing)
	}
	return out
}

func completionClaimHasDeterministicEvidence(objective StructuredObjective, observations []StructuredCommandObservation, workingDir string) bool {
	if len(objective.RequiredEvidence) > 0 {
		return structuredObjectiveRequiredEvidenceSatisfied(objective, observations, workingDir)
	}
	for _, obs := range observations {
		if obs.ExitCode == 0 && structuredObservationSatisfiesObjective(obs, objective) {
			return true
		}
	}
	return false
}

func satisfyPendingObjectivesFromValidator(ledger []StructuredObjective, reason string) []StructuredObjective {
	return mergeStructuredObjectiveLedger(nil, ledger)
}

func keepAtLeastOnePreviouslyPendingObjectiveOpen(previous, updated []StructuredObjective) []StructuredObjective {
	pendingBefore := pendingStructuredObjectives(previous)
	if len(pendingBefore) == 0 {
		return updated
	}
	objective := pendingBefore[0]
	objective.Status = "pending"
	objective.Evidence = ""
	return forceStructuredObjectivesPending(updated, []StructuredObjective{objective})
}

func runSelectedRecipeCompletionProbes(ctx context.Context, step int, currentWorkingDirectory string, ledger []StructuredObjective, recipes []Recipe, onEvent func(StructuredCommandEvent)) ([]StructuredObjective, []StructuredCommandObservation) {
	observations := []StructuredCommandObservation{}
	for _, recipe := range recipes {
		results, passed := RunRecipeCompletionProbes(ctx, recipe, currentWorkingDirectory)
		if len(results) == 0 {
			continue
		}
		observations = append(observations, RecipeProbeObservations(results, step, currentWorkingDirectory)...)
		evidence := FormatRecipeProbeEvidence(results)
		emitStructuredCommandEvent(onEvent, "recipe_completion_probes_completed", "Deterministic recipe completion probes ran", map[string]string{
			"recipe": recipe.ID,
			"passed": fmt.Sprintf("%t", passed),
			"checks": fmt.Sprintf("%d", len(results)),
		})
		ledger = ApplyRecipeProbeCompletion(ledger, recipe, passed, evidence)
	}
	return ledger, observations
}

func minimalContextHasContent(context MinimalContext) bool {
	context = normalizeMinimalContext(context)
	return context.Summary != "" || len(context.Facts) > 0 || len(context.Constraints) > 0 || len(context.OpenItems) > 0
}

func minimalContextAnswer(context MinimalContext) string {
	context = normalizeMinimalContext(context)
	parts := []string{}
	if context.Summary != "" {
		parts = append(parts, context.Summary)
	}
	if len(context.Facts) > 0 {
		parts = append(parts, strings.Join(context.Facts, " "))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func reconcileStructuredObjectiveLedgerForDone(step int, ledger []StructuredObjective, latest StructuredCommandObservation, onEvent func(StructuredCommandEvent)) []StructuredObjective {
	pending := pendingStructuredObjectives(ledger)
	if len(pending) != 1 {
		return ledger
	}
	if strings.TrimSpace(latest.Command) == "" || latest.ExitCode != 0 {
		return ledger
	}
	if strings.TrimSpace(latest.Stdout) == "" && strings.TrimSpace(latest.Stderr) == "" {
		return ledger
	}
	reconciled := mergeStructuredObjectiveLedger(ledger, []StructuredObjective{{
		ID:          pending[0].ID,
		Description: pending[0].Description,
		Status:      "satisfied",
		Evidence:    structuredObjectiveEvidenceFromObservation(latest),
	}})
	emitStructuredCommandEvent(onEvent, "objective_ledger_reconciled", "Single pending objective satisfied from command evidence", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"objective": pending[0].ID,
	})
	return reconciled
}

func structuredObjectiveEvidenceFromObservation(obs StructuredCommandObservation) string {
	evidence := strings.TrimSpace(obs.Stdout)
	if evidence == "" {
		evidence = strings.TrimSpace(obs.Stderr)
	}
	if evidence == "" {
		evidence = strings.TrimSpace(obs.Command)
	}
	return truncateStructuredObservation(evidence)
}

func pendingStructuredObjectiveIDs(ledger []StructuredObjective) string {
	ids := structuredObjectiveIDs(pendingStructuredObjectives(ledger))
	if len(ids) == 0 {
		return ""
	}
	return strings.Join(ids, ",")
}

func pendingStructuredObjectives(ledger []StructuredObjective) []StructuredObjective {
	out := []StructuredObjective{}
	for _, objective := range ledger {
		if objective.Status != "satisfied" && structuredObjectiveBlocksCompletion(objective) {
			out = append(out, objective)
		}
	}
	return out
}

func structuredObjectiveSatisfied(objective StructuredObjective) bool {
	status := strings.ToLower(strings.TrimSpace(objective.Status))
	return status == "satisfied" || status == "done" || status == "complete" || status == "completed"
}

func structuredObjectiveBlocksCompletion(objective StructuredObjective) bool {
	source := strings.TrimSpace(objective.Source)
	if source == "" {
		return true
	}
	if !objective.Required {
		return false
	}
	return normalizeStructuredObjectiveSource(source) != structuredObjectiveSourceMemorySuggested
}

func structuredObjectiveIDs(objectives []StructuredObjective) []string {
	ids := make([]string, 0, len(objectives))
	for _, objective := range objectives {
		if strings.TrimSpace(objective.ID) != "" {
			ids = append(ids, objective.ID)
		}
	}
	return ids
}

func mergeStructuredObjectiveLedger(existing, update []StructuredObjective) []StructuredObjective {
	if len(existing) == 0 && len(update) == 0 {
		return nil
	}
	merged := make([]StructuredObjective, 0, len(existing)+len(update))
	index := map[string]int{}
	for _, objective := range existing {
		normalized, ok := normalizeStructuredObjective(objective)
		if !ok {
			continue
		}
		index[normalized.ID] = len(merged)
		merged = append(merged, normalized)
	}
	for _, objective := range update {
		normalized, ok := normalizeStructuredObjective(objective)
		if !ok {
			continue
		}
		if pos, exists := index[normalized.ID]; exists {
			merged[pos] = mergeStructuredObjective(merged[pos], normalized)
			continue
		}
		index[normalized.ID] = len(merged)
		merged = append(merged, normalized)
	}
	return merged
}

func filterObjectiveLedgerForWorksiteSurvey(objectives []StructuredObjective, survey WorksiteSurvey) []StructuredObjective {
	if len(objectives) == 0 {
		return nil
	}
	out := []StructuredObjective{}
	for _, objective := range objectives {
		normalized, ok := normalizeStructuredObjective(objective)
		if !ok {
			continue
		}
		if objectiveForbiddenByWorksiteSurvey(normalized, survey) {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func objectiveForbiddenByWorksiteSurvey(objective StructuredObjective, survey WorksiteSurvey) bool {
	if survey.UserOperation != userOperationModifyExisting && survey.UserOperation != userOperationFixExisting {
		return false
	}
	text := normalizedDependencyText(objective.ID + " " + objective.Description)
	return strings.Contains(text, " create new ") ||
		strings.Contains(text, " new react ") ||
		strings.Contains(text, " scaffold ") ||
		strings.Contains(text, " create_new ")
}

func normalizeStructuredObjective(objective StructuredObjective) (StructuredObjective, bool) {
	id := strings.TrimSpace(objective.ID)
	if id == "" {
		return StructuredObjective{}, false
	}
	status := strings.ToLower(strings.TrimSpace(objective.Status))
	switch status {
	case "satisfied", "done", "complete", "completed":
		status = "satisfied"
	default:
		status = "pending"
	}
	source := strings.TrimSpace(objective.Source)
	required := objective.Required
	normalizedSource := ""
	if source == "" {
		required = true
	} else {
		normalizedSource = normalizeStructuredObjectiveSource(source)
	}
	if normalizedSource == structuredObjectiveSourceMemorySuggested || normalizedSource == structuredObjectiveSourceModelInferred {
		required = false
	}
	return StructuredObjective{
		ID:               id,
		Description:      strings.TrimSpace(objective.Description),
		Status:           status,
		Kind:             string(objectiveWorkItemKindFromStructuredObjective(objective)),
		Evidence:         strings.TrimSpace(objective.Evidence),
		RequiredEvidence: cleanStringList(objective.RequiredEvidence),
		Source:           normalizedSource,
		ParentObjective:  strings.TrimSpace(objective.ParentObjective),
		Required:         required,
		Packages:         cleanStringList(objective.Packages),
	}, true
}

func normalizeStructuredObjectiveSource(source string) string {
	switch strings.TrimSpace(source) {
	case structuredObjectiveSourceUserExplicit, structuredObjectiveSourceRecipeRequired, structuredObjectiveSourceDetectedProject, structuredObjectiveSourceEvidenceRequiredPrerequisite, structuredObjectiveSourceMemorySuggested, structuredObjectiveSourceModelInferred:
		return strings.TrimSpace(source)
	default:
		return structuredObjectiveSourceModelInferred
	}
}

func mergeStructuredObjective(existing, update StructuredObjective) StructuredObjective {
	if strings.TrimSpace(update.Description) != "" {
		existing.Description = update.Description
	}
	if strings.TrimSpace(update.Evidence) != "" {
		existing.Evidence = update.Evidence
	}
	if strings.TrimSpace(update.Kind) != "" {
		existing.Kind = update.Kind
	}
	if strings.TrimSpace(update.Source) != "" && update.Source != structuredObjectiveSourceModelInferred {
		existing.Source = update.Source
	} else if strings.TrimSpace(existing.Source) == "" && strings.TrimSpace(update.Source) != "" {
		existing.Source = normalizeStructuredObjectiveSource(update.Source)
	}
	if strings.TrimSpace(update.ParentObjective) != "" {
		existing.ParentObjective = strings.TrimSpace(update.ParentObjective)
	}
	if update.Required {
		existing.Required = true
	}
	if len(update.Packages) > 0 {
		existing.Packages = cleanStringList(append(existing.Packages, update.Packages...))
	}
	if update.Status == "satisfied" {
		existing.Status = "satisfied"
	} else if existing.Status != "satisfied" {
		existing.Status = "pending"
	}
	return existing
}

func structuredFinalAnswerGivesInstructionsInsteadOfCompletion(answer string) bool {
	lower := strings.ToLower(answer)
	instructionMarkers := 0
	for _, phrase := range []string{
		"you can follow these steps",
		"follow these steps",
		"open your terminal",
		"navigate to",
		"run the following command",
		"use the following command",
		"mkdir ",
		"nano ",
		"vim ",
		"save and close",
		"verify that",
	} {
		if strings.Contains(lower, phrase) {
			instructionMarkers++
		}
	}
	if strings.Contains(lower, "1.") && strings.Contains(lower, "2.") && strings.Contains(lower, "3.") {
		instructionMarkers++
	}
	return instructionMarkers >= 2
}

func latestRealCommandSucceeded(observations []StructuredCommandObservation) bool {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) == "" {
			continue
		}
		return observations[i].ExitCode == 0
	}
	return false
}

func latestObservationIsSuccessfulCommand(observations []StructuredCommandObservation) bool {
	if len(observations) == 0 {
		return false
	}
	latest := observations[len(observations)-1]
	return strings.TrimSpace(latest.Command) != "" && latest.ExitCode == 0
}

func runtimeSourceVerificationObservation(obs StructuredCommandObservation) bool {
	if obs.ExitCode != 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(obs.EvidenceKind), "source_verification") {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(obs.GeneratedBy), "runtime") {
		return false
	}
	return strings.TrimSpace(obs.VerifierID) != "" && len(obs.CheckedFiles) > 0 && len(obs.CheckedPredicates) > 0
}

func isShellToolDelegation(payload StructuredCommandPayload) bool {
	tool := strings.ToLower(strings.TrimSpace(payload.Tool))
	return !payload.Done &&
		!payload.Ask &&
		strings.TrimSpace(payload.Command) == "" &&
		strings.TrimSpace(payload.ToolTask) != "" &&
		(tool == "shell" || tool == "terminal" || tool == "system")
}

func isPatchToolDelegation(payload StructuredCommandPayload) bool {
	tool := strings.ToLower(strings.TrimSpace(payload.Tool))
	return !payload.Done &&
		!payload.Ask &&
		strings.TrimSpace(payload.Command) == "" &&
		strings.TrimSpace(payload.Patch) != "" &&
		(tool == "patch.apply" || tool == "patch")
}
