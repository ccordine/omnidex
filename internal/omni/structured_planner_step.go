package omni

import (
	"context"
	"fmt"
	"strings"
)

type structuredPlannerStepDisposition uint8

const (
	structuredStepProceed structuredPlannerStepDisposition = iota
	structuredStepRetry
	structuredStepComplete
)

func prepareStructuredPlannerStep(
	ctx context.Context,
	step int,
	prompt string,
	cfg *structuredCommandDecisionRunConfig,
	worksiteSurvey *WorksiteSurvey,
	ledger *[]StructuredObjective,
	selectedRecipes []Recipe,
	evaluator StructuredLLMResponseEvaluator,
	evaluatorThreshold int,
	lastCompletionCheckedObservationCount *int,
	onEvent func(StructuredCommandEvent),
	result *CommandDecisionResult,
) (structuredPlannerStepDisposition, error) {
	if strings.TrimSpace(result.TargetRoot) != "" &&
		structuredPromptWorkingDirectory(cfg.CurrentWorkingDirectory) != structuredPromptWorkingDirectory(result.TargetRoot) {
		cfg.CurrentWorkingDirectory = result.TargetRoot
		previousMode := worksiteSurvey.TaskMode
		previousOperation := worksiteSurvey.UserOperation
		*worksiteSurvey = BuildWorksiteSurvey(cfg.CurrentWorkingDirectory).WithOperation(previousOperation)
		worksiteSurvey.TaskMode = previousMode
		emitStructuredCommandEvent(onEvent, "active_worksite_target_root_applied", "Runtime applied promoted target root for subsequent commands", map[string]string{
			"step":        fmt.Sprintf("%d", step),
			"target_root": structuredPromptWorkingDirectory(cfg.CurrentWorkingDirectory),
		})
	}

	if len(result.Observations) == *lastCompletionCheckedObservationCount ||
		!latestObservationIsSuccessfulCommand(result.Observations) ||
		len(pendingStructuredObjectives(*ledger)) == 0 {
		return structuredStepProceed, nil
	}

	latest, _ := latestSuccessfulCommandObservation(result.Observations)
	result.Answer = finalStructuredAnswer(result.Answer, latest)
	ledgerBeforeProgress := mergeStructuredObjectiveLedger(nil, *ledger)
	*ledger = reconcileStructuredObjectiveLedgerFromObservation(step-1, *ledger, latest, onEvent)
	if len(selectedRecipes) > 0 {
		var recipeObservations []StructuredCommandObservation
		*ledger, recipeObservations = runSelectedRecipeCompletionProbes(
			ctx,
			step-1,
			cfg.CurrentWorkingDirectory,
			*ledger,
			selectedRecipes,
			onEvent,
		)
		result.Observations = append(result.Observations, recipeObservations...)
	}
	result.ObjectiveLedger = *ledger
	refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, *worksiteSurvey, ledger, result, onEvent)
	*lastCompletionCheckedObservationCount = len(result.Observations)

	if len(pendingStructuredObjectives(*ledger)) > 0 {
		acceptPartialCompletionForContinuation(step-1, ledgerBeforeProgress, *ledger, latest, onEvent, result)
		return structuredStepProceed, nil
	}
	if !deterministicCompletionEnforcerAcceptsDone(prompt, *ledger, result.Observations) {
		return structuredStepRetry, nil
	}
	if rejectTypedFinalGate(step-1, cfg.CurrentWorkingDirectory, onEvent, result) {
		return structuredStepRetry, nil
	}
	accepted, err := runFinalBroadEvaluatorAfterTypedCompletion(
		ctx,
		step-1,
		prompt,
		evaluator,
		evaluatorThreshold,
		*cfg,
		*worksiteSurvey,
		*ledger,
		result.Observations,
		result.Answer,
		onEvent,
		result,
	)
	if err != nil {
		return structuredStepProceed, err
	}
	if !accepted {
		return structuredStepRetry, nil
	}
	emitStructuredCommandEvent(onEvent, "adaptive_roles_collapsed", "Deterministic command evidence satisfied the task", map[string]string{
		"step":    fmt.Sprintf("%d", step-1),
		"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
		"skipped": "completion_checker,planner",
	})
	emitStructuredCommandEvent(onEvent, "completion_accepted_from_command_evidence", "Typed command evidence satisfied the objective ledger and completion gates", map[string]string{
		"step": fmt.Sprintf("%d", step-1),
	})
	return structuredStepComplete, nil
}

func finalizeStructuredLoopExhaustion(
	ctx context.Context,
	prompt string,
	cfg structuredCommandDecisionRunConfig,
	worksiteSurvey WorksiteSurvey,
	ledger []StructuredObjective,
	onEvent func(StructuredCommandEvent),
	result *CommandDecisionResult,
) error {
	emitStructuredCommandEvent(onEvent, "structured_loop_exhausted", "Structured command loop exhausted attempts", map[string]string{
		"max_steps": fmt.Sprintf("%d", defaultCommandDecisionMaxSteps),
	})
	if cfg.ThinkingService != nil {
		active := NewActivePromptContext(prompt, "", explicitReactAppAcceptanceCriteria(prompt, ""))
		_, _ = cfg.ThinkingService.Reason(ctx, ThinkingInput{
			TurnID:          cfg.ThoughtTurnID,
			Step:            defaultCommandDecisionMaxSteps,
			Trigger:         "loop_exhausted",
			UserPrompt:      prompt,
			WorkingDir:      cfg.CurrentWorkingDirectory,
			GateReason:      fmt.Sprintf("structured command loop exhausted after %d steps", defaultCommandDecisionMaxSteps),
			Observations:    result.Observations,
			LoopState:       structuredLoopStateFromState(ledger, result.Observations),
			ObjectiveLedger: ledger,
			SessionMemories: cfg.SessionMemories,
			PrepContext:     cfg.PrepContext,
			ProjectFileMap: activeProjectFileMapFromResult(
				prompt,
				mapDrivenArchitectToolTask(cfg.CurrentWorkingDirectory, worksiteSurvey),
				cfg.CurrentWorkingDirectory,
				worksiteSurvey,
				result.Observations,
			),
			ActivePrompt: active,
		}, onEvent)
	}
	if hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0 {
		result.PartialProgress = true
	}
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	return CommandDecisionExhaustedError{MaxSteps: defaultCommandDecisionMaxSteps}
}
