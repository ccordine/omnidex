package omni

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func runStructuredCommandDecisionWithConfig(ctx context.Context, prompt string, history []Message, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, cfg structuredCommandDecisionRunConfig) (result CommandDecisionResult, retErr error) {
	if strings.TrimSpace(prompt) == "" {
		return CommandDecisionResult{}, fmt.Errorf("prompt is empty")
	}
	if client == nil && cfg.PromptInterpreter == nil {
		return CommandDecisionResult{}, fmt.Errorf("llm client is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultCommandDecisionTimeout)
	defer cancel()

	startedAt := time.Now()
	result = CommandDecisionResult{StartedAt: startedAt}
	defer func() {
		if result.StartedAt.IsZero() {
			result.StartedAt = startedAt
		}
		result.FinishedAt = time.Now()
		result.Elapsed = result.FinishedAt.Sub(result.StartedAt)
	}()

	evaluator := cfg.Evaluator
	evaluatorThreshold := normalizeStructuredEvaluatorThreshold(cfg.EvaluatorThreshold)
	ledger := []StructuredObjective{}
	minimalContext := MinimalContext{}
	selectedRecipes := []Recipe{}
	referenceHistoryAllowed := false
	worksiteSurvey := BuildWorksiteSurvey(cfg.CurrentWorkingDirectory)
	if mode := normalizeTaskMode(cfg.TaskMode); mode != "" {
		worksiteSurvey.TaskMode = mode
	} else {
		worksiteSurvey.TaskMode = inferTaskMode(prompt, worksiteSurvey)
	}
	result.TaskMode = worksiteSurvey.TaskMode
	result.TargetRoot = structuredPromptWorkingDirectory(cfg.CurrentWorkingDirectory)
	cfg.SessionMemories = filterExecutionSessionMemories(cfg.SessionMemories, prompt, cfg.CurrentWorkingDirectory, len(cfg.SessionMemories))
	result.MinimalContext = minimalContext
	emitStructuredCommandEvent(onEvent, "worksite_survey_completed", "Worksite survey grounded the active workspace", map[string]string{
		"workspace":       worksiteSurvey.WorkspacePath,
		"task_mode":       string(worksiteSurvey.TaskMode),
		"project_state":   worksiteSurvey.ProjectState,
		"package_manager": worksiteSurvey.PackageManager,
		"frameworks":      strings.Join(worksiteSurvey.Frameworks, ","),
	})
	if len(allPrepBriefs(cfg.PrepContext)) > 0 || len(cfg.PrepContext.Evidence) > 0 {
		emitStructuredCommandEvent(onEvent, "prep_context_attached_to_planner", "Preparation context attached to structured planner", map[string]string{
			"briefs":       fmt.Sprintf("%d", len(allPrepBriefs(cfg.PrepContext))),
			"evidence":     fmt.Sprintf("%d", len(cfg.PrepContext.Evidence)),
			"budget_used":  fmt.Sprintf("%d", cfg.PrepContext.ContextBudgetUsed),
			"budget_limit": fmt.Sprintf("%d", cfg.PrepContext.ContextBudgetLimit),
			"role":         "planner",
		})
	}
	if cfg.PromptInterpreter != nil {
		interpretation, err := cfg.PromptInterpreter.InterpretPrompt(ctx, PromptInterpretationInput{
			UserPrompt:              prompt,
			History:                 history,
			CurrentWorkingDirectory: structuredPromptWorkingDirectory(cfg.CurrentWorkingDirectory),
			Recipes:                 cfg.Recipes,
			WorksiteSurvey:          worksiteSurvey,
		})
		if err != nil {
			wrapped := fmt.Errorf("prompt interpreter failed: %w", err)
			emitStructuredCommandEvent(onEvent, "prompt_interpreter_failed", "Prompt interpreter failed; structured execution stopped", map[string]string{
				"error": truncateStructuredTimelineValue(err.Error()),
			})
			return result, wrapped
		} else {
			referenceHistoryAllowed = interpretation.RequiresReferenceHistory
			worksiteSurvey = worksiteSurvey.WithOperation(interpretation.UserOperation)
			if mode := normalizeTaskMode(cfg.TaskMode); mode != "" {
				worksiteSurvey.TaskMode = mode
			} else {
				worksiteSurvey.TaskMode = inferTaskMode(prompt, worksiteSurvey)
			}
			result.TaskMode = worksiteSurvey.TaskMode
			worksiteSurvey.RecommendedRecipeIDs = cleanStringList(append(interpretation.RecommendedRecipeIDs, interpretation.RecipeIDs...))
			worksiteSurvey.ForbiddenRecipeIDs = cleanStringList(append(worksiteSurvey.ForbiddenRecipeIDs, interpretation.ForbiddenRecipeIDs...))
			selectedRecipes = FilterRecipesForWorksiteSurvey(SelectRecipesByID(cfg.Recipes, interpretation.RecipeIDs), worksiteSurvey)
			if len(selectedRecipes) > 0 {
				for _, recipe := range selectedRecipes {
					ledger = mergeStructuredObjectiveLedger(ledger, RecipeObjectiveLedger(recipe))
				}
				emitStructuredCommandEvent(onEvent, "recipe_selected", "Prompt interpreter selected recipe manifest(s)", map[string]string{
					"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
				})
			}
			ledger = mergeStructuredObjectiveLedger(ledger, filterObjectiveLedgerForWorksiteSurvey(interpretation.ObjectiveLedger, worksiteSurvey))
			if len(ledger) == 0 {
				err := fmt.Errorf("prompt interpreter returned no executable objectives for user operation %q", worksiteSurvey.UserOperation)
				emitStructuredCommandEvent(onEvent, "prompt_interpreter_invalid", "Prompt interpreter returned an empty objective ledger; structured execution stopped", map[string]string{
					"error":          truncateStructuredTimelineValue(err.Error()),
					"user_operation": worksiteSurvey.UserOperation,
				})
				return result, err
			}
			result.ObjectiveLedger = ledger
			refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
			emitStructuredCommandEvent(onEvent, "prompt_interpreter_completed", "Prompt interpreter produced objective ledger", map[string]string{
				"objective_count":    fmt.Sprintf("%d", len(ledger)),
				"pending_objectives": pendingStructuredObjectiveIDs(ledger),
				"uses_history":       fmt.Sprintf("%t", referenceHistoryAllowed),
				"user_operation":     worksiteSurvey.UserOperation,
				"project_state":      worksiteSurvey.ProjectState,
			})
			if len(ledger) > 0 && len(pendingStructuredObjectives(ledger)) == 0 {
				result.Command = "SUCCESS_RECONCILIATION"
				result.ExitCode = 0
				result.Answer = "Requested objective already has deterministic evidence."
				emitStructuredCommandEvent(onEvent, "adaptive_roles_collapsed", "Success reconciliation satisfied the task before planner call", map[string]string{
					"skipped": "planner",
				})
				return result, nil
			}
		}
	}
	referenceHistory := []Message(nil)
	if referenceHistoryAllowed {
		referenceHistory = history
	}
	if len(selectedRecipes) > 0 && len(pendingStructuredObjectives(ledger)) > 0 {
		var recipeObservations []StructuredCommandObservation
		ledger, recipeObservations = runSelectedRecipeCompletionProbes(ctx, 0, cfg.CurrentWorkingDirectory, ledger, selectedRecipes, onEvent)
		result.Observations = append(result.Observations, recipeObservations...)
		result.ObjectiveLedger = ledger
		refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
		if len(pendingStructuredObjectives(ledger)) == 0 {
			if !rejectTypedFinalGate(0, cfg.CurrentWorkingDirectory, onEvent, &result) {
				result.Command = "RECIPE_COMPLETION_PROBES"
				result.ExitCode = 0
				result.Answer = "Recipe completion probes passed."
				emitStructuredCommandEvent(onEvent, "adaptive_roles_collapsed", "Deterministic recipe probes satisfied the task before additional specialist calls", map[string]string{
					"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
					"skipped": "context_summarizer,completion_checker,planner",
				})
				emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_recipe_probes", "Deterministic recipe probes satisfied objective ledger", map[string]string{
					"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
				})
				return result, nil
			}
		}
	}
	if cfg.ContextSummarizer != nil {
		summary, err := cfg.ContextSummarizer.SummarizeContext(ctx, MinimalContextInput{
			UserPrompt:              prompt,
			CurrentWorkingDirectory: structuredPromptWorkingDirectory(cfg.CurrentWorkingDirectory),
			ObjectiveLedger:         ledger,
			CompletedActions:        completedActionsFromState(ledger, result.Observations),
			History:                 referenceHistory,
			SessionMemories:         cfg.SessionMemories,
			ExistingContext:         minimalContext,
			WorksiteSurvey:          worksiteSurvey,
		})
		if err != nil {
			wrapped := fmt.Errorf("context summarizer failed: %w", err)
			emitStructuredCommandEvent(onEvent, "minimal_context_failed", "Context summarizer failed; structured execution stopped", map[string]string{
				"error": truncateStructuredTimelineValue(err.Error()),
			})
			return result, wrapped
		} else {
			minimalContext = normalizeMinimalContext(summary)
			result.MinimalContext = minimalContext
			emitStructuredCommandEvent(onEvent, "minimal_context_updated", "Context inventory loaded for active task", map[string]string{
				"facts":       fmt.Sprintf("%d", len(minimalContext.Facts)),
				"constraints": fmt.Sprintf("%d", len(minimalContext.Constraints)),
				"open_items":  fmt.Sprintf("%d", len(minimalContext.OpenItems)),
			})
		}
	}
	if cfg.CompletionChecker != nil && minimalContextHasContent(minimalContext) && len(pendingStructuredObjectives(ledger)) == 0 && typedWorkQueuePassedForCompletion(&result) {
		var validatorAccepted bool
		var checkErr error
		ledger, validatorAccepted, checkErr = runCompletionCheck(ctx, 0, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, nil, minimalContextAnswer(minimalContext), cfg.CompletionChecker, worksiteSurvey, onEvent)
		if checkErr != nil {
			return result, checkErr
		}
		result.ObjectiveLedger = ledger
		refreshStructuredTypedWorkItems(prompt, cfg.CurrentWorkingDirectory, worksiteSurvey, &ledger, &result, onEvent)
		if validatorAccepted && len(pendingStructuredObjectives(ledger)) == 0 {
			result.Command = "MEMORY_CONTEXT"
			result.ExitCode = 0
			result.Answer = minimalContextAnswer(minimalContext)
			emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_context", "Done-check specialist accepted existing context without a command", map[string]string{
				"answer": truncateStructuredTimelineValue(result.Answer),
			})
			return result, nil
		}
	}
	return runStructuredCommandLoop(ctx, prompt, referenceHistory, client, stdout, stderr, onEvent, onAsk, cfg, evaluator, evaluatorThreshold, ledger, minimalContext, selectedRecipes, worksiteSurvey, result)
}
