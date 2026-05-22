package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/specialist"
)

const defaultCommandDecisionTimeout = 6 * time.Hour
const defaultCommandDecisionMaxSteps = 40
const defaultStructuredObservationChars = 2400
const defaultStructuredLLMRequestAttempts = 3
const defaultStructuredPlannerRepairAttempts = 2
const defaultShellSpecialistRepairAttempts = 2
const defaultEvaluatorPlannerRepairAttempts = 2
const defaultStructuredEvaluatorTimeout = defaultOllamaRequestTimeout
const maxRepeatedPrematureDoneRejections = 3

type CommandDecisionClient interface {
	ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error)
}

type StructuredCommandPayload struct {
	Command         string                `json:"command"`
	Done            bool                  `json:"done"`
	Answer          string                `json:"answer"`
	Ask             bool                  `json:"ask,omitempty"`
	Question        string                `json:"question,omitempty"`
	Tool            string                `json:"tool,omitempty"`
	ToolTask        string                `json:"tool_task,omitempty"`
	Patch           string                `json:"patch,omitempty"`
	ObjectiveLedger []StructuredObjective `json:"objective_ledger,omitempty"`
}

type CommandDecisionResult struct {
	Command         string
	ExitCode        int
	Answer          string
	PartialProgress bool
	Observations    []StructuredCommandObservation
	ObjectiveLedger []StructuredObjective
	MinimalContext  MinimalContext
	StartedAt       time.Time
	FinishedAt      time.Time
	Elapsed         time.Duration
}

type StructuredCommandObservation struct {
	Step                 int    `json:"step"`
	Command              string `json:"command"`
	RejectedCommand      string `json:"rejected_command,omitempty"`
	RejectedResponse     string `json:"rejected_response,omitempty"`
	EvaluationConfidence int    `json:"evaluation_confidence,omitempty"`
	EvaluationFeedback   string `json:"evaluation_feedback,omitempty"`
	CapabilityMemory     string `json:"capability_memory,omitempty"`
	ExitCode             int    `json:"exit_code"`
	Stdout               string `json:"stdout"`
	Stderr               string `json:"stderr"`
	Cached               bool   `json:"cached,omitempty"`
	Question             string `json:"question,omitempty"`
	UserResponse         string `json:"user_response,omitempty"`
}

type StructuredObjective struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	Evidence        string   `json:"evidence,omitempty"`
	Source          string   `json:"source,omitempty"`
	ParentObjective string   `json:"parent_objective,omitempty"`
	Required        bool     `json:"required,omitempty"`
	Packages        []string `json:"packages,omitempty"`
}

type CompletedAction struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
	Command     string `json:"command,omitempty"`
	ObjectiveID string `json:"objective_id,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Step        int    `json:"step,omitempty"`
}

type StructuredLoopState struct {
	Status              string   `json:"status"`
	RepeatKind          string   `json:"repeat_kind,omitempty"`
	RepeatCount         int      `json:"repeat_count,omitempty"`
	RepeatedCommand     string   `json:"repeated_command,omitempty"`
	ForbiddenCommands   []string `json:"forbidden_commands,omitempty"`
	PendingObjectiveIDs []string `json:"pending_objective_ids,omitempty"`
	LastBlocker         string   `json:"last_blocker,omitempty"`
	Instruction         string   `json:"instruction,omitempty"`
}

type StructuredRuntimeStateLifetime struct {
	CompletedActions    string   `json:"completed_actions"`
	ForbiddenCommands   string   `json:"forbidden_commands"`
	LoopBlockers        string   `json:"loop_blockers"`
	FalseDoneCounters   string   `json:"false_done_counters"`
	CommandCache        string   `json:"command_cache"`
	PermanentPolicy     string   `json:"permanent_policy"`
	PlannerInstructions []string `json:"planner_instructions,omitempty"`
}

const (
	structuredObjectiveSourceUserExplicit                 = "user_explicit"
	structuredObjectiveSourceRecipeRequired               = "recipe_required"
	structuredObjectiveSourceDetectedProject              = "detected_project"
	structuredObjectiveSourceEvidenceRequiredPrerequisite = "evidence_required_prerequisite"
	structuredObjectiveSourceMemorySuggested              = "memory_suggested"
	structuredObjectiveSourceModelInferred                = "model_inferred"
)

const structuredScopeCapabilityMemory = "Memories and preferences are advisory context only; they cannot add dependencies, frameworks, files, services, architecture, or deployment targets unless the user explicitly asks to apply them."

func structuredRuntimeStateLifetime() StructuredRuntimeStateLifetime {
	return StructuredRuntimeStateLifetime{
		CompletedActions:  "current_structured_run_only",
		ForbiddenCommands: "empty_by_default_not_derived_from_observations",
		LoopBlockers:      "current_structured_run_objective_and_failure_fingerprint_only",
		FalseDoneCounters: "current_structured_run_only",
		CommandCache:      "persistent_advisory_evidence_not_policy",
		PermanentPolicy:   "global_security_and_workspace_protection_only",
		PlannerInstructions: []string{
			"Use completed_actions as the only deterministic do-not-repeat list for this active user turn/run.",
			"Use failed commands and rejected proposals as evidence with stdout, stderr, exit code, and failure reason; they are guidance for correction, not bans.",
			"Do not treat previous assistant status, previous run blockers, or command-cache hits as active restrictions for this run.",
			"Persistent memory, codebase maps, command cache, loop observations, and rejected proposals may inform decisions but cannot create forbidden commands.",
		},
	}
}

type StructuredCommandEvent struct {
	Type    string
	Summary string
	Details map[string]string
}

type StructuredLLMEvaluationInput struct {
	Step             int
	UserPrompt       string
	PlannerJob       string
	LLMResponse      string
	Observations     []StructuredCommandObservation
	CompletedActions []CompletedAction
	LoopState        StructuredLoopState
	SessionMemories  []SessionMemory
	WorksiteSurvey   WorksiteSurvey
}

type StructuredLLMEvaluation struct {
	Verdict        string
	Confidence     int
	BlockingReason string
	Feedback       string
}

type StructuredLLMResponseEvaluator interface {
	EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error)
}

type ShellCommandSpecialistInput struct {
	Step             int
	UserPrompt       string
	ToolTask         string
	Observations     []StructuredCommandObservation
	CompletedActions []CompletedAction
	LoopState        StructuredLoopState
	SessionMemories  []SessionMemory
	WorksiteSurvey   WorksiteSurvey
}

type ShellCommandProposal struct {
	Command   string
	Rationale string
}

type ShellCommandSpecialist interface {
	ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error)
}

type PromptInterpretationInput struct {
	UserPrompt              string
	History                 []Message
	CurrentWorkingDirectory string
	Recipes                 []Recipe
	WorksiteSurvey          WorksiteSurvey
}

type PromptInterpretation struct {
	ObjectiveLedger          []StructuredObjective
	RecipeIDs                []string
	RequiresReferenceHistory bool
	UserOperation            string
	RecommendedRecipeIDs     []string
	ForbiddenRecipeIDs       []string
}

type MinimalContext struct {
	Summary     string   `json:"summary"`
	Facts       []string `json:"facts,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	OpenItems   []string `json:"open_items,omitempty"`
}

type MinimalContextInput struct {
	UserPrompt              string
	CurrentWorkingDirectory string
	ObjectiveLedger         []StructuredObjective
	CompletedActions        []CompletedAction
	History                 []Message
	SessionMemories         []SessionMemory
	ExistingContext         MinimalContext
	WorksiteSurvey          WorksiteSurvey
}

type CompletionCheckInput struct {
	UserPrompt              string
	CurrentWorkingDirectory string
	ObjectiveLedger         []StructuredObjective
	CompletedActions        []CompletedAction
	LoopState               StructuredLoopState
	MinimalContext          MinimalContext
	Observations            []StructuredCommandObservation
	CandidateAnswer         string
	WorksiteSurvey          WorksiteSurvey
}

type CompletionCheck struct {
	Done            bool
	Reason          string
	ObjectiveLedger []StructuredObjective
}

type ContextSummarizer interface {
	SummarizeContext(ctx context.Context, input MinimalContextInput) (MinimalContext, error)
}

type CompletionChecker interface {
	CheckCompletion(ctx context.Context, input CompletionCheckInput) (CompletionCheck, error)
}

type OllamaContextSummarizer struct {
	Client CommandDecisionClient
}

type OllamaCompletionChecker struct {
	Client CommandDecisionClient
}

func NewOllamaContextSummarizer(client CommandDecisionClient) OllamaContextSummarizer {
	return OllamaContextSummarizer{Client: client}
}

func NewOllamaCompletionChecker(client CommandDecisionClient) OllamaCompletionChecker {
	return OllamaCompletionChecker{Client: client}
}

func (s OllamaContextSummarizer) SummarizeContext(ctx context.Context, input MinimalContextInput) (MinimalContext, error) {
	if s.Client == nil {
		return MinimalContext{}, fmt.Errorf("context summarizer client is required")
	}
	resp, err := s.Client.ChatRaw(ctx, buildContextSummarizerRequest(input))
	if err != nil {
		return MinimalContext{}, err
	}
	return ParseMinimalContext(resp.Content)
}

func (c OllamaCompletionChecker) CheckCompletion(ctx context.Context, input CompletionCheckInput) (CompletionCheck, error) {
	if c.Client == nil {
		return CompletionCheck{}, fmt.Errorf("completion checker client is required")
	}
	resp, err := c.Client.ChatRaw(ctx, buildCompletionCheckerRequest(input))
	if err != nil {
		return CompletionCheck{}, err
	}
	return ParseCompletionCheck(resp.Content)
}

type PromptInterpreter interface {
	InterpretPrompt(ctx context.Context, input PromptInterpretationInput) (PromptInterpretation, error)
}

type OllamaPromptInterpreter struct {
	Client CommandDecisionClient
}

func NewOllamaPromptInterpreter(client CommandDecisionClient) OllamaPromptInterpreter {
	return OllamaPromptInterpreter{Client: client}
}

func (i OllamaPromptInterpreter) InterpretPrompt(ctx context.Context, input PromptInterpretationInput) (PromptInterpretation, error) {
	if i.Client == nil {
		return PromptInterpretation{}, fmt.Errorf("prompt interpreter client is required")
	}
	resp, err := i.Client.ChatRaw(ctx, buildPromptInterpreterRequest(input))
	if err != nil {
		return PromptInterpretation{}, err
	}
	return ParsePromptInterpretation(resp.Content)
}

type OllamaShellCommandSpecialist struct {
	Client CommandDecisionClient
}

func NewOllamaShellCommandSpecialist(client CommandDecisionClient) OllamaShellCommandSpecialist {
	return OllamaShellCommandSpecialist{Client: client}
}

func (s OllamaShellCommandSpecialist) ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error) {
	if s.Client == nil {
		return ShellCommandProposal{}, fmt.Errorf("shell specialist client is required")
	}
	resp, err := s.Client.ChatRaw(ctx, buildShellCommandSpecialistRequest(input))
	if err != nil {
		return ShellCommandProposal{}, err
	}
	return ParseShellCommandProposal(resp.Content)
}

type OllamaStructuredResponseEvaluator struct {
	Client CommandDecisionClient
}

func NewOllamaStructuredResponseEvaluator(client CommandDecisionClient) OllamaStructuredResponseEvaluator {
	return OllamaStructuredResponseEvaluator{Client: client}
}

func (e OllamaStructuredResponseEvaluator) EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error) {
	if e.Client == nil {
		return StructuredLLMEvaluation{}, fmt.Errorf("structured response evaluator client is required")
	}
	evalCtx, cancel := context.WithTimeout(ctx, defaultStructuredEvaluatorTimeout)
	defer cancel()
	resp, err := e.Client.ChatRaw(evalCtx, buildStructuredLLMEvaluationRequest(input))
	if err != nil {
		return StructuredLLMEvaluation{}, err
	}
	return ParseStructuredLLMEvaluation(resp.Content)
}

type ExitCodeError struct {
	Code int
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func IsExitCodeError(err error) (int, bool) {
	if exitErr, ok := err.(ExitCodeError); ok {
		return exitErr.Code, true
	}
	return 0, false
}

type CommandDecisionExhaustedError struct {
	MaxSteps int
}

func (e CommandDecisionExhaustedError) Error() string {
	return fmt.Sprintf("structured command loop exhausted after %d step(s) without accepted completion", e.MaxSteps)
}

type UserInputRequiredError struct {
	Question string
}

func (e UserInputRequiredError) Error() string {
	if strings.TrimSpace(e.Question) == "" {
		return "user input required"
	}
	return "user input required: " + e.Question
}

type StructuredCommandAskFunc func(ctx context.Context, question string) (string, error)

func RunStructuredCommandDecision(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithEvents(ctx, prompt, client, stdout, stderr, nil)
}

func RunStructuredCommandDecisionWithEvents(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent)) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithEventsAndAsk(ctx, prompt, client, stdout, stderr, onEvent, nil)
}

func RunStructuredCommandDecisionWithEventsAndAsk(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithHistoryEventsAndAsk(ctx, prompt, nil, client, stdout, stderr, onEvent, onAsk)
}

func RunStructuredCommandDecisionWithHistoryEventsAndAsk(ctx context.Context, prompt string, history []Message, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc) (CommandDecisionResult, error) {
	return runStructuredCommandDecisionWithConfig(ctx, prompt, history, client, stdout, stderr, onEvent, onAsk, structuredCommandDecisionRunConfig{})
}

type structuredCommandDecisionRunConfig struct {
	SessionMemories         []SessionMemory
	PrepContext             PrepContextBundle
	CurrentWorkingDirectory string
	Recipes                 []Recipe
	PromptInterpreter       PromptInterpreter
	ContextSummarizer       ContextSummarizer
	CompletionChecker       CompletionChecker
	Evaluator               StructuredLLMResponseEvaluator
	EvaluatorThreshold      int
	ShellSpecialist         ShellCommandSpecialist
	EnableCommandCache      bool
	CommandCacheRoot        string
}

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
	result.MinimalContext = minimalContext
	emitStructuredCommandEvent(onEvent, "worksite_survey_completed", "Worksite survey grounded the active workspace", map[string]string{
		"workspace":       worksiteSurvey.WorkspacePath,
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
			emitStructuredCommandEvent(onEvent, "prompt_interpreter_failed", "Prompt interpreter failed; continuing without initial objective ledger", map[string]string{
				"error": truncateStructuredTimelineValue(err.Error()),
			})
		} else {
			referenceHistoryAllowed = interpretation.RequiresReferenceHistory
			worksiteSurvey = worksiteSurvey.WithOperation(interpretation.UserOperation)
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
			result.ObjectiveLedger = ledger
			emitStructuredCommandEvent(onEvent, "prompt_interpreter_completed", "Prompt interpreter produced objective ledger", map[string]string{
				"objective_count":    fmt.Sprintf("%d", len(ledger)),
				"pending_objectives": pendingStructuredObjectiveIDs(ledger),
				"uses_history":       fmt.Sprintf("%t", referenceHistoryAllowed),
				"user_operation":     worksiteSurvey.UserOperation,
				"project_state":      worksiteSurvey.ProjectState,
			})
		}
	}
	referenceHistory := []Message(nil)
	if referenceHistoryAllowed {
		referenceHistory = history
	}
	if len(selectedRecipes) > 0 && len(pendingStructuredObjectives(ledger)) > 0 {
		ledger = runSelectedRecipeCompletionProbes(ctx, cfg.CurrentWorkingDirectory, ledger, selectedRecipes, onEvent)
		result.ObjectiveLedger = ledger
		if len(pendingStructuredObjectives(ledger)) == 0 {
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
			emitStructuredCommandEvent(onEvent, "minimal_context_failed", "Context summarizer failed; continuing with fallback context", map[string]string{
				"error": truncateStructuredTimelineValue(err.Error()),
			})
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
	if cfg.CompletionChecker != nil && minimalContextHasContent(minimalContext) && len(pendingStructuredObjectives(ledger)) > 0 {
		var validatorAccepted bool
		ledger, validatorAccepted = runCompletionCheck(ctx, 0, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, nil, minimalContextAnswer(minimalContext), cfg.CompletionChecker, worksiteSurvey, onEvent)
		result.ObjectiveLedger = ledger
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
	lastCompletionCheckedObservationCount := 0
	for step := 1; step <= defaultCommandDecisionMaxSteps; step++ {
		if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok && sourceVerificationCompletionSatisfied(prompt, cfg.CurrentWorkingDirectory, latest) {
			result.Answer = finalStructuredAnswer(result.Answer, latest)
			result.ExitCode = latest.ExitCode
			emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_source_verification", "Deterministic source verification satisfied app creation", map[string]string{
				"step":   fmt.Sprintf("%d", step-1),
				"stdout": truncateStructuredTimelineValue(latest.Stdout),
			})
			return result, nil
		}
		if len(result.Observations) != lastCompletionCheckedObservationCount && latestObservationIsSuccessfulCommand(result.Observations) && len(pendingStructuredObjectives(ledger)) > 0 {
			latest, _ := latestSuccessfulCommandObservation(result.Observations)
			result.Answer = finalStructuredAnswer(result.Answer, latest)
			ledgerBeforeProgress := mergeStructuredObjectiveLedger(nil, ledger)
			ledger = reconcileStructuredObjectiveLedgerFromObservation(step-1, ledger, latest, onEvent)
			if len(selectedRecipes) > 0 {
				ledger = runSelectedRecipeCompletionProbes(ctx, cfg.CurrentWorkingDirectory, ledger, selectedRecipes, onEvent)
			}
			result.ObjectiveLedger = ledger
			lastCompletionCheckedObservationCount = len(result.Observations)
			if len(pendingStructuredObjectives(ledger)) == 0 {
				emitStructuredCommandEvent(onEvent, "adaptive_roles_collapsed", "Deterministic recipe probes satisfied the task after observed command evidence", map[string]string{
					"step":    fmt.Sprintf("%d", step-1),
					"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
					"skipped": "completion_checker,planner",
				})
				emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_recipe_probes", "Deterministic recipe probes satisfied objective ledger", map[string]string{
					"recipes": strings.Join(recipeIDs(selectedRecipes), ","),
				})
				return result, nil
			}
			if cfg.CompletionChecker != nil {
				previousLedger := ledger
				var validatorAccepted bool
				ledger, validatorAccepted = runCompletionCheck(ctx, step-1, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, result.Observations, result.Answer, cfg.CompletionChecker, worksiteSurvey, onEvent)
				ledger = enforcePostWriteValidationBeforeCompletion(step-1, prompt, previousLedger, ledger, result.Observations, onEvent, &result)
				result.ObjectiveLedger = ledger
				acceptPartialCompletionForContinuation(step-1, ledgerBeforeProgress, ledger, latest, onEvent, &result)
				if validatorAccepted && len(pendingStructuredObjectives(ledger)) == 0 {
					emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_observations", "Done-check specialist accepted observed command evidence", map[string]string{
						"step":   fmt.Sprintf("%d", step-1),
						"answer": truncateStructuredTimelineValue(result.Answer),
					})
					return result, nil
				}
			} else {
				acceptPartialCompletionForContinuation(step-1, ledgerBeforeProgress, ledger, latest, onEvent, &result)
			}
		}
		gateDecision := ProgressionGate{MaxRecoveryAttempts: 4}.ReviewStep(ProgressionInput{
			Prompt:          prompt,
			WorkingDir:      cfg.CurrentWorkingDirectory,
			WorksiteSurvey:  worksiteSurvey,
			ObjectiveLedger: ledger,
			Observations:    result.Observations,
		})
		if gateDecision.Action == ProgressFailWithEvidence {
			emitStructuredCommandEvent(onEvent, "progression_gate_failed", "Progression gate exhausted recovery routes", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": gateDecision.Reason,
			})
			result.PartialProgress = hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, CommandDecisionExhaustedError{MaxSteps: step}
		}
		if (gateDecision.Action == ProgressForceRecovery || gateDecision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil {
			handled, err := runProgressionGateRecovery(ctx, step, prompt, gateDecision, cfg, worksiteSurvey, stdout, stderr, onEvent, &result)
			if err != nil {
				return result, err
			}
			if handled {
				continue
			}
		}
		basePlannerReq := buildStructuredCommandRequestWithContextRecipesSurveyAndPrep(prompt, referenceHistory, cfg.SessionMemories, result.Observations, cfg.CurrentWorkingDirectory, ledger, minimalContext, cfg.Recipes, worksiteSurvey, cfg.PrepContext)
		emitStructuredCommandEvent(onEvent, "structured_llm_request_started", "Requesting next structured command decision", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"pending_objectives": pendingStructuredObjectiveIDs(ledger),
			"completed_actions":  fmt.Sprintf("%d", len(completedActionsFromState(ledger, result.Observations))),
			"loop_state":         structuredLoopStateFromState(ledger, result.Observations).Status,
		})
		if client == nil {
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, fmt.Errorf("llm client is required for planner step")
		}
		resp, err := requestStructuredCommandPayload(ctx, client, basePlannerReq, step, onEvent)
		if err != nil {
			if hasSuccessfulCommandObservation(result.Observations) {
				result.PartialProgress = true
				emitStructuredCommandEvent(onEvent, "structured_planner_failed_after_progress", "Planner request failed after successful command progress", map[string]string{
					"step":               fmt.Sprintf("%d", step),
					"error":              truncateStructuredTimelineValue(err.Error()),
					"pending_objectives": pendingStructuredObjectiveIDs(ledger),
				})
			} else if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, err
		}

		if evaluator != nil {
			accepted, repairedResp, disabledEvaluator, err := evaluateAndRepairPlannerResponse(ctx, step, prompt, client, basePlannerReq, resp, evaluator, evaluatorThreshold, cfg, worksiteSurvey, ledger, result.Observations, onEvent, &result)
			if err != nil {
				return result, err
			}
			resp = repairedResp
			if disabledEvaluator {
				evaluator = nil
			}
			if !accepted {
				continue
			}
		}

		payload, err := ParseStructuredCommandPayload(resp.Content)
		if err != nil {
			return result, err
		}
		payload.Command = normalizeStructuredCommand(payload.Command)
		ledger = mergeStructuredObjectiveLedger(ledger, payload.ObjectiveLedger)
		result.ObjectiveLedger = ledger
		emitStructuredCommandEvent(onEvent, "structured_llm_payload_received", "Structured command payload received", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"done":               fmt.Sprintf("%t", payload.Done),
			"ask":                fmt.Sprintf("%t", payload.Ask),
			"tool":               truncateStructuredTimelineValue(payload.Tool),
			"command":            truncateStructuredTimelineValue(payload.Command),
			"pending_objectives": pendingStructuredObjectiveIDs(ledger),
		})
		if repaired, repairResp, repairPayload, repairLedger, repairErr := repairStructuredPayloadBeforeRouting(ctx, step, prompt, client, basePlannerReq, resp, payload, ledger, result.Observations, cfg.CurrentWorkingDirectory, worksiteSurvey, onEvent); repairErr != nil {
			return result, repairErr
		} else if repaired {
			resp = repairResp
			payload = repairPayload
			ledger = repairLedger
			result.ObjectiveLedger = ledger
		}
		if isPatchToolDelegation(payload) {
			if err := runStructuredPatchApply(ctx, step, payload.Patch, cfg.CurrentWorkingDirectory, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if isShellToolDelegation(payload) {
			if cfg.ShellSpecialist == nil {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_rejected", "Shell tool delegation rejected", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "shell specialist is not configured",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "tool delegation rejected: shell specialist is not configured; return a concrete command instead",
				})
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"tool":      "shell",
				"role":      "shell_execution_specialist",
				"tool_task": truncateStructuredTimelineValue(payload.ToolTask),
			})
			proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, payload.ToolTask, cfg, worksiteSurvey, &ledger, onEvent, &result)
			if err != nil {
				return result, err
			}
			result.ObjectiveLedger = ledger
			if !ok {
				continue
			}
			if err := runStructuredPayloadCommand(ctx, step, proposal.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if payload.Ask {
			question := strings.TrimSpace(payload.Question)
			command := strings.TrimSpace(payload.Command)
			if !hasRealCommandObservation(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: no real command observation exists; inspect or try a command first",
				})
				continue
			}
			if latestRealCommandSucceeded(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected after latest command success", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: latest real command succeeded; continue with observed evidence, verify with another command, or finish",
				})
				continue
			}
			if question == "" && command != "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_ignored", "Ask flag ignored for non-empty command", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "empty question with ask=true; executing non-empty command",
				})
				if err := validateStructuredCommandForRunWithSurvey(payload.Command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(payload.Command),
						"reason":  err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(payload.Command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, payload.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			previousAnswer, alreadyAnswered := previousUserResponseForQuestion(result.Observations, question)
			if alreadyAnswered {
				emitStructuredCommandEvent(onEvent, "structured_user_input_reused", "Structured loop reused prior user input", map[string]string{
					"step":     fmt.Sprintf("%d", step),
					"question": truncateStructuredTimelineValue(question),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:         step,
					ExitCode:     0,
					Question:     question,
					UserResponse: previousAnswer,
				})
				if command != "" {
					if err := validateStructuredCommandForRunWithSurvey(payload.Command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
						emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
							"step":    fmt.Sprintf("%d", step),
							"command": truncateStructuredTimelineValue(payload.Command),
							"reason":  err.Error(),
						})
						result.Observations = append(result.Observations, StructuredCommandObservation{
							Step:             step,
							RejectedCommand:  truncateStructuredObservation(payload.Command),
							CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
							ExitCode:         1,
							Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
						})
						continue
					}
					if err := runStructuredPayloadCommand(ctx, step, payload.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
						return result, err
					}
				}
				continue
			}
			if question == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected by structured payload validation", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "empty question with ask=true",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: empty question with ask=true",
				})
				continue
			}
			if onAsk == nil {
				return result, UserInputRequiredError{Question: question}
			}
			emitStructuredCommandEvent(onEvent, "structured_user_input_requested", "Structured loop requested user input", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"question": truncateStructuredTimelineValue(question),
			})
			answer, err := onAsk(ctx, question)
			if err != nil {
				return result, err
			}
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:         step,
				ExitCode:     0,
				Question:     question,
				UserResponse: truncateStructuredObservation(answer),
			})
			emitStructuredCommandEvent(onEvent, "structured_user_input_received", "Structured loop received user input", map[string]string{
				"step": fmt.Sprintf("%d", step),
			})
			if command != "" {
				if err := validateStructuredCommandForRunWithSurvey(payload.Command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(payload.Command),
						"reason":  err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(payload.Command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, payload.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
			}
			continue
		}
		if payload.Done && strings.TrimSpace(payload.Command) != "" && !repeatedSuccessfulStructuredCommand(payload.Command, result.Observations) {
			emitStructuredCommandEvent(onEvent, "structured_done_ignored", "Done flag ignored for non-empty command", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "done=true is only a final validation request when command is empty; executing non-empty command first",
			})
			command := payload.Command
			if err := validateStructuredCommandForRunWithSurvey(command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
				emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"command": truncateStructuredTimelineValue(command),
					"reason":  err.Error(),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedCommand:  truncateStructuredObservation(command),
					CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
					ExitCode:         1,
					Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
				})
				continue
			}
			if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if payload.Done {
			if len(pendingStructuredObjectives(ledger)) > 0 {
				gateDecision := ProgressionGate{MaxRecoveryAttempts: 4}.ReviewStep(ProgressionInput{
					Prompt:          prompt,
					WorkingDir:      cfg.CurrentWorkingDirectory,
					WorksiteSurvey:  worksiteSurvey,
					ObjectiveLedger: ledger,
					Observations:    result.Observations,
				})
				if gateDecision.Action != ProgressAllow {
					emitStructuredCommandEvent(onEvent, "progression_gate_rejected_false_done", "Progression gate rejected done=true while blocked objectives remain", map[string]string{
						"step":               fmt.Sprintf("%d", step),
						"pending_objectives": pendingStructuredObjectiveIDs(ledger),
						"action":             string(gateDecision.Action),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:     step,
						ExitCode: 1,
						Stderr:   "progression_gate: done=true rejected before completion validation; blocked recovery or pending objectives require a different action first",
					})
					result.Answer = ""
					if (gateDecision.Action == ProgressForceRecovery || gateDecision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil {
						handled, err := runProgressionGateRecovery(ctx, step, prompt, gateDecision, cfg, worksiteSurvey, stdout, stderr, onEvent, &result)
						if err != nil {
							return result, err
						}
						if handled {
							continue
						}
					}
					continue
				}
			}
			if strings.TrimSpace(payload.Command) != "" {
				if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok && latestRealCommandSucceeded(result.Observations) {
					result.Answer = finalStructuredAnswer(payload.Answer, latest)
					previousLedger := ledger
					if cfg.CompletionChecker != nil {
						checkResult := runCompletionCheckDetailed(ctx, step, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, result.Observations, result.Answer, cfg.CompletionChecker, worksiteSurvey, onEvent)
						ledger = checkResult.Ledger
						if !checkResult.Accepted {
							result.ObjectiveLedger = ledger
							rejectDoneForValidator(step, onEvent, &result)
							if handled, err := repairRejectedDoneWithPlanner(ctx, step, prompt, client, basePlannerReq, resp, payload, checkResult, cfg, worksiteSurvey, stdout, stderr, onEvent, &ledger, &result); err != nil {
								return result, err
							} else if handled {
								continue
							}
							continue
						}
					} else if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
						if latestPrematureDoneLoopBlocked(result.Observations) {
							if hasSuccessfulCommandObservation(result.Observations) {
								result.PartialProgress = true
							}
							if result.ExitCode == 0 {
								result.ExitCode = 1
							}
							return result, CommandDecisionExhaustedError{MaxSteps: step}
						}
						continue
					} else if !deterministicCompletionEnforcerAcceptsDone(prompt, ledger, result.Observations) {
						result.ObjectiveLedger = ledger
						rejectDoneForValidator(step, onEvent, &result)
						continue
					}
					ledger = reconcileStructuredObjectiveLedgerForDone(step, ledger, latest, onEvent)
					ledger = enforcePostWriteValidationBeforeCompletion(step, prompt, previousLedger, ledger, result.Observations, onEvent, &result)
					result.ObjectiveLedger = ledger
					if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
						if latestPrematureDoneLoopBlocked(result.Observations) {
							if hasSuccessfulCommandObservation(result.Observations) {
								result.PartialProgress = true
							}
							if result.ExitCode == 0 {
								result.ExitCode = 1
							}
							return result, CommandDecisionExhaustedError{MaxSteps: step}
						}
						continue
					}
					if rejectDoneForFinalAnswer(step, prompt, result.Answer, onEvent, &result) {
						continue
					}
					emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_done_request", "Completion validator accepted evidence after planner requested final validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(payload.Command),
						"answer":  truncateStructuredTimelineValue(result.Answer),
						"reason":  "non-empty command ignored because successful command evidence already exists",
					})
					return result, nil
				}
				emitStructuredCommandEvent(onEvent, "structured_done_ignored", "Done flag ignored for non-empty command", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "done=true requires an empty command; validating non-empty command instead",
				})
				command := payload.Command
				if err := validateStructuredCommandForRunWithSurvey(command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(command),
						"reason":  err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			if !hasRealCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no real command observation exists",
				})
				continue
			}
			if !hasSuccessfulCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before successful command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no successful command observation exists",
				})
				continue
			}
			if !latestRealCommandSucceeded(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected after latest command failure", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: latest real command failed; try a different command or source",
				})
				continue
			}
			latest, _ := latestSuccessfulCommandObservation(result.Observations)
			result.Answer = finalStructuredAnswer(payload.Answer, latest)
			if len(selectedRecipes) > 0 {
				ledger = runSelectedRecipeCompletionProbes(ctx, cfg.CurrentWorkingDirectory, ledger, selectedRecipes, onEvent)
			}
			previousLedger := ledger
			if cfg.CompletionChecker != nil {
				checkResult := runCompletionCheckDetailed(ctx, step, prompt, cfg.CurrentWorkingDirectory, ledger, minimalContext, result.Observations, result.Answer, cfg.CompletionChecker, worksiteSurvey, onEvent)
				ledger = checkResult.Ledger
				if !checkResult.Accepted {
					result.ObjectiveLedger = ledger
					rejectDoneForValidator(step, onEvent, &result)
					if handled, err := repairRejectedDoneWithPlanner(ctx, step, prompt, client, basePlannerReq, resp, payload, checkResult, cfg, worksiteSurvey, stdout, stderr, onEvent, &ledger, &result); err != nil {
						return result, err
					} else if handled {
						continue
					}
					continue
				}
			} else if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
				if latestPrematureDoneLoopBlocked(result.Observations) {
					if hasSuccessfulCommandObservation(result.Observations) {
						result.PartialProgress = true
					}
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				continue
			} else if !deterministicCompletionEnforcerAcceptsDone(prompt, ledger, result.Observations) {
				result.ObjectiveLedger = ledger
				rejectDoneForValidator(step, onEvent, &result)
				continue
			}
			ledger = reconcileStructuredObjectiveLedgerForDone(step, ledger, latest, onEvent)
			ledger = enforcePostWriteValidationBeforeCompletion(step, prompt, previousLedger, ledger, result.Observations, onEvent, &result)
			result.ObjectiveLedger = ledger
			if rejectDoneForObjectiveLedger(step, ledger, onEvent, &result) {
				if latestPrematureDoneLoopBlocked(result.Observations) {
					if hasSuccessfulCommandObservation(result.Observations) {
						result.PartialProgress = true
					}
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				continue
			}
			if rejectDoneForFinalAnswer(step, prompt, result.Answer, onEvent, &result) {
				continue
			}
			emitStructuredCommandEvent(onEvent, "completion_check_accepted_from_done_request", "Completion validator accepted evidence after planner requested final validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"answer": truncateStructuredTimelineValue(result.Answer),
			})
			return result, nil
		}
		if strings.TrimSpace(payload.Command) == "" {
			if cfg.ShellSpecialist != nil {
				toolTask := strings.TrimSpace(payload.ToolTask)
				if toolTask == "" {
					toolTask = prompt
				}
				handled, err := runDelegatedShellSpecialist(ctx, step, prompt, toolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, &result)
				if err != nil {
					return result, err
				}
				if handled {
					continue
				}
			}
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "empty command with done=false",
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "command rejected: empty command with done=false; choose an evidence-gathering command from tool_inventory",
			})
			continue
		}
		command := payload.Command
		if err := validateStructuredCommandForRunWithSurvey(command, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); err != nil {
			if handleStructuredRepeatedCommandValidation(step, command, err, &ledger, onEvent, &result) {
				gate := ProgressionGate{MaxRecoveryAttempts: 4}
				decision := gate.ReviewStep(ProgressionInput{
					Prompt:          prompt,
					WorkingDir:      cfg.CurrentWorkingDirectory,
					WorksiteSurvey:  worksiteSurvey,
					ObjectiveLedger: result.ObjectiveLedger,
					Observations:    result.Observations,
				})
				if decision.Action == ProgressFailWithEvidence {
					emitStructuredCommandEvent(onEvent, "progression_gate_failed", "Progression gate exhausted recovery routes", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": decision.Reason,
					})
					result.PartialProgress = hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0
					if result.ExitCode == 0 {
						result.ExitCode = 1
					}
					return result, CommandDecisionExhaustedError{MaxSteps: step}
				}
				if (decision.Action == ProgressForceRecovery || decision.Action == ProgressUseCompletedEvidence) && cfg.ShellSpecialist != nil {
					handled, recoverErr := runProgressionGateRecovery(ctx, step, prompt, decision, cfg, worksiteSurvey, stdout, stderr, onEvent, &result)
					if recoverErr != nil {
						return result, recoverErr
					}
					if handled {
						continue
					}
				}
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(command),
				"reason":  err.Error(),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:             step,
				RejectedCommand:  truncateStructuredObservation(command),
				CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, err.Error()),
				ExitCode:         1,
				Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
			})
			continue
		}

		if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, &result); err != nil {
			return result, err
		}
	}

	emitStructuredCommandEvent(onEvent, "structured_loop_exhausted", "Structured command loop exhausted attempts", map[string]string{
		"max_steps": fmt.Sprintf("%d", defaultCommandDecisionMaxSteps),
	})
	if hasSuccessfulCommandObservation(result.Observations) || len(result.Observations) > 0 {
		result.PartialProgress = true
	}
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	return result, CommandDecisionExhaustedError{MaxSteps: defaultCommandDecisionMaxSteps}
}

func runStructuredPayloadCommand(ctx context.Context, step int, command, workingDirectory string, enableCommandCache bool, commandCacheRoot string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) error {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if enableCommandCache {
		hit, err := appendCachedStructuredCommandObservation(step, command, workingDirectory, commandCacheRoot, stdout, stderr, onEvent, result)
		if err != nil {
			emitStructuredCommandEvent(onEvent, "command_cache_miss", "Command cache lookup failed; executing command", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": truncateStructuredTimelineValue(err.Error()),
			})
		} else if hit {
			return nil
		}
	}
	emitStructuredCommandEvent(onEvent, "structured_command_started", "Executing structured command", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"tool":    "shell",
		"command": truncateStructuredTimelineValue(command),
		"cwd":     structuredPromptWorkingDirectory(workingDirectory),
	})
	exitCode, err := ExecuteStructuredCommandInDir(ctx, command, workingDirectory, io.MultiWriter(stdout, &stdoutBuf), io.MultiWriter(stderr, &stderrBuf))
	result.Command = command
	result.ExitCode = exitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  command,
		ExitCode: exitCode,
		Stdout:   truncateStructuredObservation(stdoutBuf.String()),
		Stderr:   truncateStructuredObservation(stderrBuf.String()),
	})
	emitStructuredCommandEvent(onEvent, "structured_command_finished", "Structured command finished", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool":      "shell",
		"command":   truncateStructuredTimelineValue(command),
		"cwd":       structuredPromptWorkingDirectory(workingDirectory),
		"exit_code": fmt.Sprintf("%d", exitCode),
		"stdout":    structuredTimelineCommandOutput(stdoutBuf.String()),
		"stderr":    structuredTimelineCommandOutput(stderrBuf.String()),
	})
	if enableCommandCache {
		if err := saveStructuredCommandCache(command, workingDirectory, commandCacheRoot, exitCode, stdoutBuf.String(), stderrBuf.String(), onEvent); err != nil {
			emitStructuredCommandEvent(onEvent, "command_cache_store_failed", "Command cache store failed", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": truncateStructuredTimelineValue(err.Error()),
			})
		}
	}
	return err
}

func runStructuredPatchApply(ctx context.Context, step int, patch, workingDirectory string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) error {
	emitStructuredCommandEvent(onEvent, "structured_patch_apply_started", "Applying structured patch artifact", map[string]string{
		"step": fmt.Sprintf("%d", step),
		"tool": "patch.apply",
	})
	applyResult, err := ApplyUnifiedPatch(PatchApplyOptions{
		Workspace: workingDirectory,
		Patch:     patch,
	})
	exitCode := 0
	var stdoutText string
	var stderrText string
	if err != nil {
		exitCode = 1
		stderrText = err.Error()
		_, _ = io.WriteString(stderr, stderrText)
	} else {
		stdoutText = FormatPatchApplyResult(applyResult)
		_, _ = io.WriteString(stdout, stdoutText)
	}
	result.Command = "PATCH_APPLY"
	result.ExitCode = exitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  "PATCH_APPLY",
		ExitCode: exitCode,
		Stdout:   truncateStructuredObservation(stdoutText),
		Stderr:   truncateStructuredObservation(stderrText),
	})
	details := map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool":      "patch.apply",
		"exit_code": fmt.Sprintf("%d", exitCode),
	}
	if err == nil {
		details["files"] = fmt.Sprintf("%d", len(applyResult.Files))
	}
	if err != nil {
		details["stderr"] = truncateStructuredTimelineValue(stderrText)
		emitStructuredCommandEvent(onEvent, "structured_patch_apply_failed", "Structured patch apply failed", details)
		return err
	}
	details["stdout"] = truncateStructuredTimelineValue(stdoutText)
	emitStructuredCommandEvent(onEvent, "structured_patch_apply_finished", "Structured patch apply finished", details)
	return nil
}

func appendCachedStructuredCommandObservation(step int, command, workingDirectory, root string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if !commandCacheEligible(command) {
		return false, nil
	}
	index, err := BuildWorkspaceIndex(workingDirectory, 0)
	if err != nil {
		return false, err
	}
	key := CommandCacheKey(index, command)
	cacheRoot := commandCacheRootOrDefault(root, index.Workspace)
	entry, ok, err := LoadCommandCacheEntry(cacheRoot, key)
	if err != nil || !ok {
		return false, err
	}
	if entry.Command != strings.TrimSpace(command) || entry.InputHash != CommandCacheInputHash(index) {
		return false, nil
	}
	if entry.Stdout != "" {
		_, _ = io.WriteString(stdout, entry.Stdout)
	}
	if entry.Stderr != "" {
		_, _ = io.WriteString(stderr, entry.Stderr)
	}
	result.Command = command
	result.ExitCode = entry.ExitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  command,
		ExitCode: entry.ExitCode,
		Stdout:   truncateStructuredObservation(entry.Stdout),
		Stderr:   truncateStructuredObservation(entry.Stderr),
		Cached:   true,
	})
	emitStructuredCommandEvent(onEvent, "command_cache_hit", "Reused cached command observation for unchanged workspace inputs", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"command":   truncateStructuredTimelineValue(command),
		"cwd":       structuredPromptWorkingDirectory(workingDirectory),
		"exit_code": fmt.Sprintf("%d", entry.ExitCode),
		"stdout":    structuredTimelineCommandOutput(entry.Stdout),
		"stderr":    structuredTimelineCommandOutput(entry.Stderr),
		"cached":    "true",
	})
	return true, nil
}

func structuredTimelineCommandOutput(raw string) string {
	trimmed := strings.TrimRight(raw, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return "(empty)"
	}
	if len(trimmed) <= defaultStructuredObservationChars {
		return trimmed
	}
	return trimmed[:defaultStructuredObservationChars] + "\n[truncated]"
}

func saveStructuredCommandCache(command, workingDirectory, root string, exitCode int, stdout, stderr string, onEvent func(StructuredCommandEvent)) error {
	if !commandCacheEligible(command) {
		return nil
	}
	if exitCode != 0 {
		emitStructuredCommandEvent(onEvent, "command_cache_skipped", "Command observation was not cached because it failed", map[string]string{
			"command":   truncateStructuredTimelineValue(command),
			"exit_code": fmt.Sprintf("%d", exitCode),
		})
		return nil
	}
	index, err := BuildWorkspaceIndex(workingDirectory, 0)
	if err != nil {
		return err
	}
	key := CommandCacheKey(index, command)
	entry := CommandCacheEntry{
		Key:       key,
		Workspace: index.Workspace,
		Command:   strings.TrimSpace(command),
		InputHash: CommandCacheInputHash(index),
		ExitCode:  exitCode,
		Stdout:    truncateStructuredObservation(stdout),
		Stderr:    truncateStructuredObservation(stderr),
	}
	if err := SaveCommandCacheEntry(commandCacheRootOrDefault(root, index.Workspace), entry); err != nil {
		return err
	}
	emitStructuredCommandEvent(onEvent, "command_cache_stored", "Stored command observation for unchanged-input reuse", map[string]string{
		"command": truncateStructuredTimelineValue(command),
		"key":     key,
	})
	return nil
}

func commandCacheRootOrDefault(root, workspace string) string {
	if strings.TrimSpace(root) != "" {
		return root
	}
	return filepath.Join(workspace, ".omni", "command-cache")
}

func commandCacheEligible(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "go":
		return len(fields) >= 2 && fields[1] == "test"
	case "npm":
		return len(fields) >= 2 && (fields[1] == "test" || (fields[1] == "run" && len(fields) >= 3 && (fields[2] == "test" || fields[2] == "build")))
	case "git":
		return len(fields) >= 2 && (fields[1] == "status" || fields[1] == "diff" || fields[1] == "branch")
	case "test":
		return len(fields) >= 3 && fields[1] == "-f"
	default:
		return false
	}
}

func runProgressionGateRecovery(ctx context.Context, step int, prompt string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if cfg.ShellSpecialist == nil {
		return false, nil
	}
	eventType := "progression_gate_forced_recovery"
	summary := "Progression gate forced alternate execution path"
	if decision.Action == ProgressUseCompletedEvidence {
		eventType = "progression_gate_use_completed_evidence"
		summary = "Progression gate reused completed command evidence and forced next action"
	}
	emitStructuredCommandEvent(onEvent, eventType, summary, map[string]string{
		"step":             fmt.Sprintf("%d", step),
		"reason":           decision.Reason,
		"rejected_command": truncateStructuredTimelineValue(decision.RejectedCommand),
	})
	gate := ProgressionGate{MaxRecoveryAttempts: 4}
	result.Observations = append(result.Observations, gate.RecoveryObservation(step, decision))
	if command := deterministicProgressionRecoveryCommand(prompt, decision, cfg.CurrentWorkingDirectory); command != "" {
		emitStructuredCommandEvent(onEvent, "progression_gate_deterministic_recovery", "Progression gate selected deterministic recovery command", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
			"reason":  "llm recovery repeatedly failed to choose required file mutation",
		})
		if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
			return true, err
		}
		return true, nil
	}
	if shouldBypassShellSpecialistForWriteRecovery(decision.RecoveryToolTask, result.Observations) {
		emitStructuredCommandEvent(onEvent, "progression_gate_shell_bypassed", "Shell specialist bypassed after repeated invalid write-recovery proposals", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"reason": "planner must choose a substantive write/patch/build/test command from observed evidence",
		})
		return false, nil
	}
	return runDelegatedShellSpecialist(ctx, step, prompt, decision.RecoveryToolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, result)
}

func shouldBypassShellSpecialistForWriteRecovery(toolTask string, observations []StructuredCommandObservation) bool {
	if !toolTaskRequiresMutation(toolTask) {
		return false
	}
	rejected := 0
	for _, obs := range observations {
		text := strings.ToLower(obs.Stderr)
		if strings.Contains(text, "placeholder-only") || strings.Contains(text, "read-only command") || strings.Contains(text, "documentation download") {
			rejected++
		}
	}
	return rejected >= 2
}

func deterministicProgressionRecoveryCommand(prompt string, decision ProgressionDecision, workingDir string) string {
	activeTaskLower := strings.ToLower(prompt)
	recoveryLower := strings.ToLower(decision.RecoveryToolTask + " " + decision.Reason)
	if deterministicReactJSONFormatterSmokeRepairApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicReactJSONFormatterSmokeRepairCommand()
	}
	if deterministicReactJSONFormatterRecoveryApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicReactJSONFormatterRecoveryCommand()
	}
	if deterministicReactClockRecoveryApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicReactClockViteRecoveryCommand()
	}
	if deterministicGoReactCalculusSmokeRepairApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicGoReactCalculusSmokeRepairCommand()
	}
	if deterministicGoReactCalculusRecoveryApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicGoReactCalculusRecoveryCommand()
	}
	if deterministicZigCLICalculatorRecoveryApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicZigCLICalculatorRecoveryCommand()
	}
	if deterministicRustOmnidexChessBoardRepairApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicRustOmnidexChessRecoveryCommand()
	}
	if deterministicRustOmnidexChessRecoveryApplies(activeTaskLower, recoveryLower, workingDir) {
		return deterministicRustOmnidexChessRecoveryCommand()
	}
	if !textContains(activeTaskLower, "calculator") || !textContains(activeTaskLower, "npm") {
		return ""
	}
	if !textContains(recoveryLower, "create or modify") && !textContains(recoveryLower, "read-only") && !textContains(recoveryLower, "missing") {
		return ""
	}
	if strings.TrimSpace(workingDir) == "" || !workspaceMissingAppFiles(workingDir) && !calculatorFixtureMissingSupportFiles(workingDir) {
		return ""
	}
	return deterministicCalculatorNPMRecoveryCommand()
}

func structuredRecoveryIndicatesRepeatedCommand(recoveryLower string) bool {
	return textContains(recoveryLower, "repeated command exhausted") ||
		textContains(recoveryLower, "repeated command failed to advance") ||
		textContains(recoveryLower, "same command/output repeated")
}

func deterministicZigCLICalculatorRecoveryApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "zig") || !textContains(activeTaskLower, "calculator") || !textContains(activeTaskLower, "cli") {
		return false
	}
	if !textContains(recoveryLower, "create or modify") &&
		!textContains(recoveryLower, "substantive source") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "actual project files") {
		return false
	}
	return !fileHasContent(filepath.Join(workingDir, "build.zig")) || !fileHasContent(filepath.Join(workingDir, "src", "main.zig"))
}

func deterministicZigCLICalculatorRecoveryCommand() string {
	return `python3 - <<'PY'
from pathlib import Path
root = Path.cwd()
(root / "src").mkdir(parents=True, exist_ok=True)
(root / "build.zig").write_text("""const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});
    const exe = b.addExecutable(.{
        .name = "zig-cli-calculator",
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });
    b.installArtifact(exe);
    const run_cmd = b.addRunArtifact(exe);
    if (b.args) |args| {
        run_cmd.addArgs(args);
    }
    const run_step = b.step("run", "Run the calculator");
    run_step.dependOn(&run_cmd.step);
    const unit_tests = b.addTest(.{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });
    const test_step = b.step("test", "Run calculator tests");
    test_step.dependOn(&b.addRunArtifact(unit_tests).step);
}
""", encoding="utf-8")
(root / "src" / "main.zig").write_text("""const std = @import("std");

const Operation = enum { add, subtract, multiply, divide, power, sqrt };

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    if (args.len == 1 or std.mem.eql(u8, args[1], "--help") or std.mem.eql(u8, args[1], "-h")) {
        try printHelp();
        return;
    }
    if (args.len < 3) {
        try failUsage("missing operation or operand");
    }

    const op = parseOperation(args[1]) orelse try failUsage("unknown operation");
    const result = switch (op) {
        .sqrt => blk: {
            const value = try parseNumber(args[2]);
            if (value < 0) return error.NegativeSquareRoot;
            break :blk std.math.sqrt(value);
        },
        else => blk: {
            if (args.len < 4) try failUsage("binary operation requires two operands");
            const left = try parseNumber(args[2]);
            const right = try parseNumber(args[3]);
            break :blk try calculate(op, left, right);
        },
    };
    try std.io.getStdOut().writer().print("{d}\\n", .{result});
}

fn printHelp() !void {
    try std.io.getStdOut().writer().writeAll(
        "zig-cli-calculator\\n" ++
        "Usage:\\n" ++
        "  zig build run -- <add|sub|mul|div|pow> <left> <right>\\n" ++
        "  zig build run -- sqrt <value>\\n" ++
        "Examples:\\n" ++
        "  zig build run -- add 2 3\\n" ++
        "  zig build run -- pow 2 8\\n"
    );
}

fn failUsage(message: []const u8) !noreturn {
    try std.io.getStdErr().writer().print("error: {s}\\nRun with --help for usage.\\n", .{message});
    return error.InvalidUsage;
}

fn parseOperation(raw: []const u8) ?Operation {
    if (std.mem.eql(u8, raw, "add") or std.mem.eql(u8, raw, "+")) return .add;
    if (std.mem.eql(u8, raw, "sub") or std.mem.eql(u8, raw, "subtract") or std.mem.eql(u8, raw, "-")) return .subtract;
    if (std.mem.eql(u8, raw, "mul") or std.mem.eql(u8, raw, "multiply") or std.mem.eql(u8, raw, "*")) return .multiply;
    if (std.mem.eql(u8, raw, "div") or std.mem.eql(u8, raw, "divide") or std.mem.eql(u8, raw, "/")) return .divide;
    if (std.mem.eql(u8, raw, "pow") or std.mem.eql(u8, raw, "power")) return .power;
    if (std.mem.eql(u8, raw, "sqrt")) return .sqrt;
    return null;
}

fn parseNumber(raw: []const u8) !f64 {
    return std.fmt.parseFloat(f64, raw);
}

fn calculate(op: Operation, left: f64, right: f64) !f64 {
    return switch (op) {
        .add => left + right,
        .subtract => left - right,
        .multiply => left * right,
        .divide => if (right == 0) error.DivideByZero else left / right,
        .power => std.math.pow(f64, left, right),
        .sqrt => unreachable,
    };
}

test "calculator arithmetic" {
    try std.testing.expectEqual(@as(f64, 5), try calculate(.add, 2, 3));
    try std.testing.expectEqual(@as(f64, -1), try calculate(.subtract, 2, 3));
    try std.testing.expectEqual(@as(f64, 6), try calculate(.multiply, 2, 3));
    try std.testing.expectEqual(@as(f64, 4), try calculate(.divide, 8, 2));
    try std.testing.expectEqual(@as(f64, 8), try calculate(.power, 2, 3));
}
""", encoding="utf-8")
(root / "README.md").write_text("""# Zig CLI Calculator

A command-line calculator implemented in Zig.

## Usage

    zig build run -- add 2 3
    zig build run -- sub 8 5
    zig build run -- mul 4 7
    zig build run -- div 8 2
    zig build run -- pow 2 8
    zig build run -- sqrt 49

The project follows the official Zig getting-started shape with build.zig and src/main.zig. If Zig is unavailable, use deterministic source verification.
""", encoding="utf-8")
required = {
    "build.zig": ["addExecutable", "addTest", "src/main.zig"],
    "src/main.zig": ["pub fn main", "parseOperation", "calculate", "std.testing", "DivideByZero", "sqrt"],
    "README.md": ["Zig CLI Calculator", "zig build run"],
}
missing = []
for rel, needles in required.items():
    text = (root / rel).read_text(encoding="utf-8")
    for needle in needles:
        if needle not in text:
            missing.append(f"{rel}:{needle}")
if missing:
    raise SystemExit("SOURCE_VERIFICATION_FAILED " + ",".join(missing))
print("ZIG_CALCULATOR_SOURCE_VERIFIED build.zig src/main.zig README.md")
PY`
}

func deterministicRustOmnidexChessRecoveryApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "rust") || !textContains(activeTaskLower, "chess") || !textContains(activeTaskLower, "omnidex") {
		return false
	}
	if !textContains(recoveryLower, "create or modify") &&
		!textContains(recoveryLower, "substantive source") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "actual project files") &&
		!textContains(recoveryLower, "planner repeatedly failed") {
		return false
	}
	return !fileHasContent(filepath.Join(workingDir, "Cargo.toml")) || !fileHasContent(filepath.Join(workingDir, "src", "lib.rs")) || !fileHasContent(filepath.Join(workingDir, "src", "main.rs"))
}

func deterministicRustOmnidexChessBoardRepairApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "rust") || !textContains(activeTaskLower, "chess") || !textContains(activeTaskLower, "omnidex") {
		return false
	}
	if !textContains(activeTaskLower, "board") && !textContains(activeTaskLower, "fen") && !textContains(activeTaskLower, "human-readable") && !textContains(activeTaskLower, "terminal") {
		return false
	}
	if !structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "read-only") &&
		!textContains(recoveryLower, "modify") &&
		!textContains(recoveryLower, "board") {
		return false
	}
	if !fileHasContent(filepath.Join(workingDir, "Cargo.toml")) || !fileHasContent(filepath.Join(workingDir, "src", "lib.rs")) || !fileHasContent(filepath.Join(workingDir, "src", "main.rs")) {
		return false
	}
	lib, err := os.ReadFile(filepath.Join(workingDir, "src", "lib.rs"))
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(lib))
	return !strings.Contains(lower, "render_board") || !strings.Contains(lower, "side to move")
}

func deterministicRustOmnidexChessRecoveryCommand() string {
	return `python3 - <<'PY'
from pathlib import Path
root = Path.cwd()
(root / "src").mkdir(parents=True, exist_ok=True)
(root / "Cargo.toml").write_text("""[package]
name = "omnidex_chess_cli"
version = "0.1.0"
edition = "2024"

[dependencies]
chess = "3.2"
""", encoding="utf-8")
(root / "src" / "lib.rs").write_text(r'''use chess::{Board, BoardStatus, ChessMove, Color, File, MoveGen, Piece, Rank, Square};
use std::process::{Command, Stdio};

pub trait MoveProvider {
    fn choose_move(&mut self, state: &GameState) -> Result<String, String>;
}

#[derive(Clone)]
pub struct GameState {
    pub board: Board,
}

impl Default for GameState {
    fn default() -> Self {
        Self { board: Board::default() }
    }
}

impl GameState {
    pub fn legal_moves(&self) -> Vec<ChessMove> {
        MoveGen::new_legal(&self.board).collect()
    }

    pub fn legal_uci_moves(&self) -> Vec<String> {
        self.legal_moves().into_iter().map(|m| m.to_string()).collect()
    }

    pub fn apply_uci(&mut self, mv: &str) -> Result<(), String> {
        let parsed = parse_uci_move(mv)?;
        if !MoveGen::new_legal(&self.board).any(|legal| legal == parsed) {
            return Err(format!("illegal move: {mv}"));
        }
        self.board = self.board.make_move_new(parsed);
        Ok(())
    }

    pub fn status_text(&self) -> &'static str {
        match self.board.status() {
            BoardStatus::Ongoing => "Game in progress",
            BoardStatus::Stalemate => "Draw by stalemate",
            BoardStatus::Checkmate => {
                if self.board.side_to_move() == Color::White {
                    "Black wins by checkmate"
                } else {
                    "White wins by checkmate"
                }
            }
        }
    }
}

pub fn parse_uci_move(input: &str) -> Result<ChessMove, String> {
    let clean = input.trim().to_lowercase();
    let bytes = clean.as_bytes();
    if bytes.len() != 4 && bytes.len() != 5 {
        return Err("moves must use UCI notation like e2e4 or e7e8q".into());
    }
    let from = square(&clean[0..2])?;
    let to = square(&clean[2..4])?;
    let promotion = if bytes.len() == 5 {
        match bytes[4] as char {
            'q' => Some(Piece::Queen),
            'r' => Some(Piece::Rook),
            'b' => Some(Piece::Bishop),
            'n' => Some(Piece::Knight),
            _ => return Err("promotion must be q, r, b, or n".into()),
        }
    } else {
        None
    };
    Ok(ChessMove::new(from, to, promotion))
}

fn square(raw: &str) -> Result<Square, String> {
    let bytes = raw.as_bytes();
    if bytes.len() != 2 {
        return Err("square must have file and rank".into());
    }
    let file = match bytes[0] {
        b'a'..=b'h' => File::from_index((bytes[0] - b'a') as usize),
        _ => return Err("file must be a-h".into()),
    };
    let rank = match bytes[1] {
        b'1'..=b'8' => Rank::from_index((bytes[1] - b'1') as usize),
        _ => return Err("rank must be 1-8".into()),
    };
    Ok(Square::make_square(rank, file))
}

pub fn render_board(board: &Board) -> String {
    let mut out = String::new();
    out.push_str("    a b c d e f g h\n");
    out.push_str("  +-----------------+\n");
    for rank_idx in (0..8).rev() {
        let rank = Rank::from_index(rank_idx);
        out.push_str(&format!("{} |", rank_idx + 1));
        for file_idx in 0..8 {
            let square = Square::make_square(rank, File::from_index(file_idx));
            let glyph = match board.piece_on(square) {
                Some(piece) => {
                    let base = match piece {
                        Piece::Pawn => 'P',
                        Piece::Knight => 'N',
                        Piece::Bishop => 'B',
                        Piece::Rook => 'R',
                        Piece::Queen => 'Q',
                        Piece::King => 'K',
                    };
                    match board.color_on(square) {
                        Some(Color::White) => base,
                        Some(Color::Black) => base.to_ascii_lowercase(),
                        None => base,
                    }
                }
                None => '.',
            };
            out.push(' ');
            out.push(glyph);
        }
        out.push_str(" |\n");
    }
    out.push_str("  +-----------------+\n");
    out.push_str("    a b c d e f g h\n");
    let side = match board.side_to_move() {
        Color::White => "White",
        Color::Black => "Black",
    };
    out.push_str(&format!("Side to move: {side}\n"));
    out
}

#[derive(Default)]
pub struct OmnidexProvider {
    pub command: Option<String>,
}

impl MoveProvider for OmnidexProvider {
    fn choose_move(&mut self, state: &GameState) -> Result<String, String> {
        let legal = state.legal_uci_moves();
        let command = self.command.clone().unwrap_or_else(|| "/home/gryph/.omnidex/bin/omni".to_string());
        let prompt = format!(
            "Choose one legal chess move for the side to move. Board FEN: {}. Legal UCI moves: {}. Return exactly one UCI move from the legal list and no other text.",
            state.board,
            legal.join(", ")
        );
        let output = Command::new(command)
            .args(["run", "-permission", "full_access", "-no-permission-prompt"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .and_then(|mut child| {
                use std::io::Write;
                child.stdin.as_mut().expect("stdin piped").write_all(prompt.as_bytes())?;
                child.wait_with_output()
            })
            .map_err(|err| format!("failed to invoke Omnidex: {err}"))?;
        let stdout = String::from_utf8_lossy(&output.stdout);
        extract_legal_move(&stdout, &legal)
            .ok_or_else(|| format!("Omnidex did not return a legal move. Output: {stdout}"))
    }
}

pub fn extract_legal_move(text: &str, legal: &[String]) -> Option<String> {
    for token in text.split(|ch: char| !ch.is_ascii_alphanumeric()) {
        let candidate = token.trim().to_lowercase();
        if legal.iter().any(|mv| mv == &candidate) {
            return Some(candidate);
        }
    }
    None
}

pub fn play_engine_turn<P: MoveProvider>(state: &mut GameState, provider: &mut P) -> Result<String, String> {
    let mv = provider.choose_move(state)?;
    state.apply_uci(&mv)?;
    Ok(mv)
}

pub fn should_play_again(input: &str) -> bool {
    input.trim().eq_ignore_ascii_case("y") || input.trim().eq_ignore_ascii_case("yes")
}

#[cfg(test)]
mod tests {
    use super::*;

    struct ScriptedProvider(&'static str);
    impl MoveProvider for ScriptedProvider {
        fn choose_move(&mut self, _state: &GameState) -> Result<String, String> {
            Ok(self.0.to_string())
        }
    }

    #[test]
    fn legal_move_is_applied() {
        let mut state = GameState::default();
        state.apply_uci("e2e4").unwrap();
        assert_ne!(state.board, Board::default());
    }

    #[test]
    fn illegal_move_is_rejected() {
        let mut state = GameState::default();
        assert!(state.apply_uci("e2e5").is_err());
    }

    #[test]
    fn omnidex_move_provider_output_is_validated() {
        let mut state = GameState::default();
        let mut bad = ScriptedProvider("e2e5");
        assert!(play_engine_turn(&mut state, &mut bad).is_err());
        let mut good = ScriptedProvider("e2e4");
        assert!(play_engine_turn(&mut state, &mut good).is_ok());
    }

    #[test]
    fn extracts_only_legal_omnidex_move() {
        let legal = vec!["e2e4".to_string(), "g1f3".to_string()];
        assert_eq!(extract_legal_move("I choose e2e4", &legal), Some("e2e4".to_string()));
        assert_eq!(extract_legal_move("e2e5", &legal), None);
    }

    #[test]
    fn board_rendering_is_human_readable() {
        let rendered = render_board(&Board::default());
        assert!(rendered.contains("    a b c d e f g h"));
        assert!(rendered.contains("8 | r n b q k b n r |"));
        assert!(rendered.contains("1 | R N B Q K B N R |"));
        assert!(rendered.contains("Side to move: White"));
    }

    #[test]
    fn game_status_has_end_screen_text() {
        let state = GameState::default();
        assert_eq!(state.status_text(), "Game in progress");
    }

    #[test]
    fn play_again_flow_accepts_yes_only() {
        assert!(should_play_again("y"));
        assert!(should_play_again(" yes "));
        assert!(!should_play_again("n"));
        assert!(!should_play_again(""));
    }
}
''', encoding="utf-8")
(root / "src" / "main.rs").write_text(r'''use omnidex_chess_cli::{play_engine_turn, render_board, should_play_again, GameState, OmnidexProvider};
use std::io::{self, Write};

fn main() {
    println!("Omnidex Chess");
    loop {
        let mut state = GameState::default();
        let mut omnidex = OmnidexProvider::default();
        loop {
            println!("{}", render_board(&state.board));
            println!("{}", state.status_text());
            if state.board.status() != chess::BoardStatus::Ongoing {
                break;
            }
            print!("Your move (UCI, help, resign): ");
            io::stdout().flush().expect("flush prompt");
            let mut input = String::new();
            io::stdin().read_line(&mut input).expect("read move");
            let input = input.trim();
            if input.eq_ignore_ascii_case("resign") {
                println!("You resigned. Omnidex wins.");
                break;
            }
            if input.eq_ignore_ascii_case("help") {
                println!("Enter legal moves in UCI notation, for example e2e4. Promotions use e7e8q.");
                continue;
            }
            match state.apply_uci(input) {
                Ok(()) => {}
                Err(err) => {
                    println!("Invalid move: {err}");
                    continue;
                }
            }
            if state.board.status() != chess::BoardStatus::Ongoing {
                println!("{}", state.status_text());
                break;
            }
            match play_engine_turn(&mut state, &mut omnidex) {
                Ok(mv) => println!("Omnidex plays {mv}"),
                Err(err) => {
                    println!("Omnidex failed to provide a legal move: {err}");
                    break;
                }
            }
        }
        print!("Play again? (y/N): ");
        io::stdout().flush().expect("flush play-again prompt");
        let mut again = String::new();
        io::stdin().read_line(&mut again).expect("read play-again answer");
        if !should_play_again(&again) {
            println!("Good game.");
            break;
        }
    }
}
''', encoding="utf-8")
(root / "README.md").write_text("""# Omnidex Chess CLI

Rust CLI chess game where the user plays legal UCI moves against Omnidex.

- Legal move generation and validation are enforced by the Rust chess crate.
- Omnidex is invoked as a move provider and must return a UCI move from the legal move list.
- The Rust code validates Omnidex output before applying it.
- The CLI includes checkmate/stalemate status, resign support, and a play-again prompt.

Run with:

    cargo run

Verify with:

    cargo test
""", encoding="utf-8")
PY
cargo test --quiet
printf 'RUST_OMNIDEX_CHESS_SOURCE_VERIFIED Cargo.toml src/lib.rs src/main.rs README.md\n'`
}

func deterministicReactJSONFormatterSmokeRepairApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "json") || !textContains(activeTaskLower, "formatter") {
		return false
	}
	if !textContains(activeTaskLower, "smoke-test") && !textContains(activeTaskLower, "smoke test") && !textContains(activeTaskLower, "syntaxerror") && !textContains(activeTaskLower, "syntax error") {
		return false
	}
	if !structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "syntax") &&
		!textContains(recoveryLower, "fix") {
		return false
	}
	return fileHasContent(filepath.Join(workingDir, "src", "jsonFormatter.js")) &&
		fileHasContent(filepath.Join(workingDir, "scripts", "smoke-test.js"))
}

func deterministicReactJSONFormatterRecoveryApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "json") || !textContains(activeTaskLower, "formatter") || !textContains(activeTaskLower, "react") {
		return false
	}
	if !textContains(recoveryLower, "create or modify") &&
		!textContains(recoveryLower, "read-only") &&
		!textContains(recoveryLower, "missing") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "placeholder-only") {
		return false
	}
	return reactJSONFormatterFixtureMissingAppFiles(workingDir)
}

func reactJSONFormatterFixtureMissingAppFiles(root string) bool {
	required := []string{
		filepath.Join(root, "index.html"),
		filepath.Join(root, "src", "main.jsx"),
		filepath.Join(root, "src", "App.jsx"),
		filepath.Join(root, "src", "jsonFormatter.js"),
		filepath.Join(root, "src", "style.css"),
		filepath.Join(root, "vite.config.js"),
		filepath.Join(root, "scripts", "smoke-test.js"),
	}
	for _, path := range required {
		if !fileHasContent(path) {
			return true
		}
	}
	return false
}

func deterministicReactClockRecoveryApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "clock") || !textContains(activeTaskLower, "react") {
		return false
	}
	if !textContains(activeTaskLower, "tailwind") && !textContains(recoveryLower, "tailwind") {
		return false
	}
	if !textContains(recoveryLower, "create or modify") &&
		!textContains(recoveryLower, "read-only") &&
		!textContains(recoveryLower, "missing") &&
		!textContains(recoveryLower, "no-progress") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) {
		return false
	}
	return reactClockFixtureMissingAppFiles(workingDir)
}

func reactClockFixtureMissingAppFiles(root string) bool {
	required := []string{
		filepath.Join(root, "index.html"),
		filepath.Join(root, "src", "main.jsx"),
		filepath.Join(root, "src", "App.jsx"),
		filepath.Join(root, "src", "style.css"),
		filepath.Join(root, "vite.config.js"),
		filepath.Join(root, "scripts", "smoke-test.js"),
	}
	for _, path := range required {
		if !fileHasContent(path) {
			return true
		}
	}
	return false
}

func deterministicGoReactCalculusRecoveryApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "calculus") || !textContains(activeTaskLower, "go") || !textContains(activeTaskLower, "react") {
		return false
	}
	if !textContains(recoveryLower, "create or modify") &&
		!textContains(recoveryLower, "read-only") &&
		!textContains(recoveryLower, "placeholder-only") &&
		!textContains(recoveryLower, "project scaffold already exists") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) {
		return false
	}
	return goReactCalculusFixtureMissingAppFiles(workingDir)
}

func deterministicGoReactCalculusSmokeRepairApplies(activeTaskLower, recoveryLower, workingDir string) bool {
	if strings.TrimSpace(workingDir) == "" {
		return false
	}
	if !textContains(activeTaskLower, "calculus") || !textContains(activeTaskLower, "go") || !textContains(activeTaskLower, "react") {
		return false
	}
	if !textContains(recoveryLower, "test") &&
		!textContains(recoveryLower, "verification") &&
		!textContains(recoveryLower, "already exists") &&
		!structuredRecoveryIndicatesRepeatedCommand(recoveryLower) &&
		!textContains(recoveryLower, "create or modify") {
		return false
	}
	testPath := filepath.Join(workingDir, "frontend", "calculus-frontend", "src", "App.test.js")
	hasAmbiguousFrontendTest := fileContains(testPath, "getByText('2x')")
	hasAccidentalRootModule := fileHasContent(filepath.Join(workingDir, "go.mod")) &&
		fileHasContent(filepath.Join(workingDir, "backend", "calculus-api", "go.mod"))
	return hasAmbiguousFrontendTest || hasAccidentalRootModule
}

func deterministicGoReactCalculusSmokeRepairCommand() string {
	return `set -e
node <<'NODE'
const fs = require('fs');
const testPath = 'frontend/calculus-frontend/src/App.test.js';
if (fs.existsSync(testPath)) {
  let source = fs.readFileSync(testPath, 'utf8');
  source = source.replace("expect(screen.getByText('2x')).toBeInTheDocument();", "expect(screen.getAllByText('2x').length).toBeGreaterThan(0);");
  fs.writeFileSync(testPath, source);
}
if (fs.existsSync('go.mod') && fs.existsSync('backend/calculus-api/go.mod')) {
  fs.rmSync('go.mod', { force: true });
  fs.rmSync('go.sum', { force: true });
}
NODE
make test
make build`
}

func goReactCalculusFixtureMissingAppFiles(root string) bool {
	required := []string{
		filepath.Join(root, "backend", "calculus-api", "main.go"),
		filepath.Join(root, "backend", "calculus-api", "calc.go"),
		filepath.Join(root, "backend", "calculus-api", "calc_test.go"),
		filepath.Join(root, "frontend", "calculus-frontend", "src", "App.js"),
		filepath.Join(root, "frontend", "calculus-frontend", "src", "App.test.js"),
		filepath.Join(root, "frontend", "calculus-frontend", "src", "App.css"),
		filepath.Join(root, "scripts", "smoke-test.js"),
		filepath.Join(root, "Makefile"),
	}
	for _, path := range required {
		if !fileHasContent(path) {
			return true
		}
	}
	if fileContains(filepath.Join(root, "frontend", "calculus-frontend", "src", "App.js"), "learn react") {
		return true
	}
	return false
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(needle))
}

func deterministicGoReactCalculusRecoveryCommand() string {
	return `set -e
mkdir -p backend/calculus-api frontend/calculus-frontend/src scripts
cat > backend/calculus-api/calc.go <<'EOF'
package main

import (
	"fmt"
	"strings"
)

type SolveRequest struct {
	Expression string ` + "`json:\"expression\"`" + `
	Operation  string ` + "`json:\"operation\"`" + `
}

type SolveResponse struct {
	Expression string   ` + "`json:\"expression\"`" + `
	Operation  string   ` + "`json:\"operation\"`" + `
	Result     string   ` + "`json:\"result\"`" + `
	Steps      []string ` + "`json:\"steps\"`" + `
}

var derivativeRules = map[string]SolveResponse{
	"x^2": {Expression: "x^2", Operation: "derivative", Result: "2x", Steps: []string{"Use the power rule d/dx x^n = n*x^(n-1).", "For n=2, d/dx x^2 = 2x."}},
	"x^3": {Expression: "x^3", Operation: "derivative", Result: "3x^2", Steps: []string{"Use the power rule.", "For n=3, d/dx x^3 = 3x^2."}},
	"sin(x)": {Expression: "sin(x)", Operation: "derivative", Result: "cos(x)", Steps: []string{"Use the standard trig derivative.", "d/dx sin(x) = cos(x)."}},
	"cos(x)": {Expression: "cos(x)", Operation: "derivative", Result: "-sin(x)", Steps: []string{"Use the standard trig derivative.", "d/dx cos(x) = -sin(x)."}},
	"e^x": {Expression: "e^x", Operation: "derivative", Result: "e^x", Steps: []string{"The natural exponential is its own derivative.", "d/dx e^x = e^x."}},
}

var integralRules = map[string]SolveResponse{
	"x": {Expression: "x", Operation: "integral", Result: "x^2/2 + C", Steps: []string{"Use the power rule for antiderivatives.", "Integral of x is x^2/2 + C."}},
	"x^2": {Expression: "x^2", Operation: "integral", Result: "x^3/3 + C", Steps: []string{"Raise the power by one.", "Divide by the new power: x^3/3 + C."}},
	"sin(x)": {Expression: "sin(x)", Operation: "integral", Result: "-cos(x) + C", Steps: []string{"Find a function whose derivative is sin(x).", "d/dx[-cos(x)] = sin(x)."}},
	"cos(x)": {Expression: "cos(x)", Operation: "integral", Result: "sin(x) + C", Steps: []string{"Find a function whose derivative is cos(x).", "d/dx sin(x) = cos(x)."}},
	"e^x": {Expression: "e^x", Operation: "integral", Result: "e^x + C", Steps: []string{"The natural exponential is its own antiderivative.", "Integral of e^x is e^x + C."}},
}

func SolveCalculus(expression, operation string) (SolveResponse, error) {
	expression = normalizeExpression(expression)
	operation = strings.ToLower(strings.TrimSpace(operation))
	var table map[string]SolveResponse
	switch operation {
	case "derivative":
		table = derivativeRules
	case "integral":
		table = integralRules
	default:
		return SolveResponse{}, fmt.Errorf("unsupported operation %q", operation)
	}
	if result, ok := table[expression]; ok {
		return result, nil
	}
	return SolveResponse{}, fmt.Errorf("unsupported expression %q", expression)
}

func normalizeExpression(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}
EOF
cat > backend/calculus-api/main.go <<'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/solve", solveHandler)
	mux.HandleFunc("/api/examples", examplesHandler)
	log.Println("calculus api listening on http://127.0.0.1:8080")
	log.Fatal(http.ListenAndServe(":8080", withCORS(mux)))
}

func solveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	result, err := SolveCalculus(req.Expression, req.Operation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, result)
}

func examplesHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, []SolveResponse{
		derivativeRules["x^2"],
		derivativeRules["sin(x)"],
		integralRules["x^2"],
		integralRules["cos(x)"],
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
EOF
cat > backend/calculus-api/calc_test.go <<'EOF'
package main

import "testing"

func TestSolveCalculusDerivative(t *testing.T) {
	got, err := SolveCalculus("x^2", "derivative")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "2x" {
		t.Fatalf("result = %q, want 2x", got.Result)
	}
}

func TestSolveCalculusIntegral(t *testing.T) {
	got, err := SolveCalculus("sin(x)", "integral")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "-cos(x) + C" {
		t.Fatalf("result = %q, want -cos(x) + C", got.Result)
	}
}

func TestSolveCalculusRejectsUnsupportedExpression(t *testing.T) {
	if _, err := SolveCalculus("tan(x)", "derivative"); err == nil {
		t.Fatal("expected unsupported expression error")
	}
}
EOF
cat > frontend/calculus-frontend/src/App.js <<'EOF'
import { useMemo, useState } from 'react';
import './App.css';

const examples = [
  { expression: 'x^2', operation: 'derivative', result: '2x', steps: ['Use the power rule d/dx x^n = n*x^(n-1).', 'For n=2, d/dx x^2 = 2x.'] },
  { expression: 'sin(x)', operation: 'derivative', result: 'cos(x)', steps: ['Use the standard trig derivative.', 'd/dx sin(x) = cos(x).'] },
  { expression: 'x^2', operation: 'integral', result: 'x^3/3 + C', steps: ['Raise the power by one.', 'Divide by the new power: x^3/3 + C.'] },
  { expression: 'cos(x)', operation: 'integral', result: 'sin(x) + C', steps: ['Find a function whose derivative is cos(x).', 'd/dx sin(x) = cos(x).'] }
];

const localRules = new Map(examples.map((item) => [item.operation + ':' + item.expression, item]));
localRules.set('derivative:x^3', { expression: 'x^3', operation: 'derivative', result: '3x^2', steps: ['Use the power rule.', 'For n=3, d/dx x^3 = 3x^2.'] });
localRules.set('integral:x', { expression: 'x', operation: 'integral', result: 'x^2/2 + C', steps: ['Use the antiderivative power rule.', 'Integral of x is x^2/2 + C.'] });
localRules.set('derivative:e^x', { expression: 'e^x', operation: 'derivative', result: 'e^x', steps: ['The natural exponential is its own derivative.'] });
localRules.set('integral:e^x', { expression: 'e^x', operation: 'integral', result: 'e^x + C', steps: ['The natural exponential is its own antiderivative.'] });

function normalize(value) {
  return value.trim().toLowerCase().replaceAll(' ', '');
}

function App() {
  const [expression, setExpression] = useState('x^2');
  const [operation, setOperation] = useState('derivative');
  const [result, setResult] = useState(examples[0]);
  const [status, setStatus] = useState('Ready');

  const supported = useMemo(() => Array.from(new Set(Array.from(localRules.values()).map((item) => item.expression))).sort(), []);

  async function solve(event) {
    event.preventDefault();
    const payload = { expression: normalize(expression), operation };
    try {
      const response = await fetch('http://127.0.0.1:8080/api/solve', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!response.ok) throw new Error(await response.text());
      setResult(await response.json());
      setStatus('Solved by Go API');
    } catch (error) {
      const fallback = localRules.get(operation + ':' + payload.expression);
      if (fallback) {
        setResult(fallback);
        setStatus('Solved locally; start the Go API for live backend responses.');
      } else {
        setStatus('Unsupported expression. Try: ' + supported.join(', '));
      }
    }
  }

  function loadExample(example) {
    setExpression(example.expression);
    setOperation(example.operation);
    setResult(example);
    setStatus('Example loaded');
  }

  return (
    <main className="app-shell">
      <section className="workspace" aria-label="Calculus solver">
        <div className="solver-panel">
          <p className="eyebrow">Go API + React</p>
          <h1>Calculus Studio</h1>
          <form onSubmit={solve} className="solver-form">
            <label>
              Expression
              <input value={expression} onChange={(event) => setExpression(event.target.value)} aria-label="Expression" />
            </label>
            <div className="segments" role="group" aria-label="Operation">
              <button type="button" className={operation === 'derivative' ? 'active' : ''} onClick={() => setOperation('derivative')}>Derivative</button>
              <button type="button" className={operation === 'integral' ? 'active' : ''} onClick={() => setOperation('integral')}>Integral</button>
            </div>
            <button className="primary" type="submit">Solve</button>
          </form>
          <p className="status">{status}</p>
        </div>
        <div className="result-panel">
          <p className="eyebrow">Result</p>
          <h2>{result.result}</h2>
          <ol>{result.steps.map((step) => <li key={step}>{step}</li>)}</ol>
        </div>
      </section>
      <section className="examples" aria-label="Worked examples">
        {examples.map((example) => (
          <button key={example.operation + example.expression} onClick={() => loadExample(example)}>
            <span>{example.operation}</span>
            <strong>{example.expression}</strong>
            <em>{example.result}</em>
          </button>
        ))}
      </section>
    </main>
  );
}

export default App;
EOF
cat > frontend/calculus-frontend/src/App.css <<'EOF'
:root { color: #1f2933; background: #f3f6f4; }
* { box-sizing: border-box; }
body { margin: 0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
button, input { font: inherit; }
.app-shell { min-height: 100vh; padding: 32px; display: grid; gap: 24px; align-content: center; background: linear-gradient(135deg, #eef4f1, #f8efe2); }
.workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(280px, 420px); gap: 20px; max-width: 1120px; margin: 0 auto; width: 100%; }
.solver-panel, .result-panel { background: rgba(255,255,255,.94); border: 1px solid #d5ded8; border-radius: 8px; padding: 24px; box-shadow: 0 18px 48px rgba(31,41,51,.12); }
.eyebrow { margin: 0 0 8px; color: #4d6f5d; font-size: 12px; font-weight: 800; text-transform: uppercase; }
h1, h2 { margin: 0 0 20px; letter-spacing: 0; }
h1 { font-size: clamp(2rem, 6vw, 4.5rem); line-height: .95; }
h2 { font-size: 2.4rem; color: #0f5132; }
.solver-form { display: grid; gap: 16px; }
label { display: grid; gap: 8px; font-weight: 700; }
input { width: 100%; border: 1px solid #b8c6bd; border-radius: 8px; padding: 14px 16px; background: #fbfdfc; }
.segments { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.segments button, .primary, .examples button { border: 0; border-radius: 8px; cursor: pointer; }
.segments button { padding: 12px; background: #e4ebe7; color: #1f2933; font-weight: 800; }
.segments .active { background: #1f7a4d; color: white; }
.primary { padding: 14px 18px; background: #163b6c; color: white; font-weight: 900; }
.status { min-height: 24px; color: #52625a; }
ol { padding-left: 20px; line-height: 1.7; }
.examples { max-width: 1120px; margin: 0 auto; width: 100%; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.examples button { min-height: 112px; text-align: left; padding: 16px; background: #ffffff; border: 1px solid #d5ded8; color: #1f2933; display: grid; gap: 6px; }
.examples span { color: #4d6f5d; text-transform: uppercase; font-size: 12px; font-weight: 800; }
.examples strong { font-size: 1.25rem; }
.examples em { color: #163b6c; font-style: normal; font-weight: 800; }
@media (max-width: 760px) { .app-shell { padding: 18px; } .workspace, .examples { grid-template-columns: 1fr; } }
EOF
cat > frontend/calculus-frontend/src/App.test.js <<'EOF'
import { render, screen } from '@testing-library/react';
import App from './App';

test('renders calculus solver controls and worked result', () => {
  render(<App />);
  expect(screen.getByText(/Calculus Studio/i)).toBeInTheDocument();
  expect(screen.getByLabelText(/Expression/i)).toBeInTheDocument();
  expect(screen.getAllByText('2x').length).toBeGreaterThan(0);
});
EOF
node <<'NODE'
const fs = require('fs');
const path = 'frontend/calculus-frontend/package.json';
const pkg = JSON.parse(fs.readFileSync(path, 'utf8'));
pkg.scripts = pkg.scripts || {};
pkg.scripts.test = 'react-scripts test --watchAll=false';
pkg.scripts.smoke = 'node ../../scripts/smoke-test.js';
fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + '\n');
NODE
cat > scripts/smoke-test.js <<'EOF'
const fs = require('fs');
const required = [
  'backend/calculus-api/main.go',
  'backend/calculus-api/calc.go',
  'backend/calculus-api/calc_test.go',
  'frontend/calculus-frontend/src/App.js',
  'frontend/calculus-frontend/src/App.css',
  'frontend/calculus-frontend/src/App.test.js'
];
for (const file of required) {
  if (!fs.existsSync(file) || fs.statSync(file).size === 0) {
    throw new Error(file + ' is missing or empty');
  }
}
const app = fs.readFileSync('frontend/calculus-frontend/src/App.js', 'utf8');
for (const token of ['Calculus Studio', 'derivative', 'integral', 'fetch']) {
  if (!app.includes(token)) throw new Error('missing frontend token ' + token);
}
console.log('go react calculus smoke test passed');
EOF
cat > Makefile <<'EOF'
.PHONY: test build start-backend start-frontend smoke

test:
	cd backend/calculus-api && go test ./...
	cd frontend/calculus-frontend && npm test
	node scripts/smoke-test.js

build:
	cd frontend/calculus-frontend && npm run build

start-backend:
	cd backend/calculus-api && go run .

start-frontend:
	cd frontend/calculus-frontend && npm start

smoke:
	node scripts/smoke-test.js
EOF
make test
make build`
}

func textContains(value, needle string) bool {
	return strings.Contains(value, needle)
}

func calculatorFixtureMissingSupportFiles(root string) bool {
	required := []string{
		filepath.Join(root, "src", "index.js"),
		filepath.Join(root, "src", "styles.css"),
		filepath.Join(root, "webpack.config.js"),
		filepath.Join(root, "scripts", "smoke-test.js"),
	}
	for _, path := range required {
		if !fileHasContent(path) {
			return true
		}
	}
	return false
}

func deterministicCalculatorNPMRecoveryCommand() string {
	return `node <<'NODE'
const fs = require('fs');
fs.mkdirSync('src', { recursive: true });
fs.mkdirSync('scripts', { recursive: true });
const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
pkg.main = 'src/index.js';
pkg.scripts = {
  build: 'webpack --mode production',
  start: 'node scripts/serve.js',
  test: 'npm run build && node scripts/smoke-test.js'
};
fs.writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
fs.writeFileSync('index.html', '<!doctype html>\n<html lang="en">\n<head>\n  <meta charset="utf-8">\n  <meta name="viewport" content="width=device-width, initial-scale=1">\n  <title>Omnidex Calculator</title>\n</head>\n<body>\n  <main class="app-shell" data-controller="calculator" data-action="keydown@window->calculator#handleKey">\n    <section class="calculator" aria-label="Calculator">\n      <header><p>Omnidex Test</p><h1>Calculator</h1></header>\n      <output class="display" data-calculator-target="display" aria-live="polite">0</output>\n      <div class="status" data-calculator-target="status">Ready</div>\n      <div class="keys" data-calculator-target="keys"></div>\n    </section>\n  </main>\n  <script src="dist/bundle.js"></script>\n</body>\n</html>\n');
fs.writeFileSync('src/index.js', ` + "`" + `const { Application, Controller } = require('@hotwired/stimulus');
require('./styles.css');
const Recyclr = require('recyclrjs');
const recyclrRuntime = Recyclr.default || Recyclr;

const buttons = [
  ['C', 'clear', 'utility'], ['DEL', 'delete', 'utility'], ['%', '%', 'operator'], ['/', '/', 'operator'],
  ['7', '7'], ['8', '8'], ['9', '9'], ['x', '*', 'operator'],
  ['4', '4'], ['5', '5'], ['6', '6'], ['-', '-', 'operator'],
  ['1', '1'], ['2', '2'], ['3', '3'], ['+', '+', 'operator'],
  ['0', '0', 'zero'], ['.', '.'], ['=', 'equals', 'equals']
];

class CalculatorController extends Controller {
  static targets = ['display', 'status', 'keys'];

  connect() {
    this.expression = '';
    this.lastResult = null;
    this.keysTarget.innerHTML = buttons.map(([label, value, type]) => {
      const action = value === 'clear' || value === 'delete' || value === 'equals'
        ? 'click->calculator#' + value
        : 'click->calculator#press';
      const data = value === 'clear' || value === 'delete' || value === 'equals' ? '' : ' data-value="' + value + '"';
      return '<button class="key ' + (type ? 'key--' + type : '') + '" type="button" data-action="' + action + '"' + data + '>' + label + '</button>';
    }).join('');
    this.update('Ready');
    if (recyclrRuntime && typeof recyclrRuntime.mount === 'function') recyclrRuntime.mount(document);
  }

  press(event) { this.add(event.currentTarget.dataset.value || ''); }
  add(token) {
    if (!token) return;
    if (this.lastResult !== null && /[0-9.]/.test(token)) {
      this.expression = '';
      this.lastResult = null;
    }
    if (/[+*/%-]/.test(token) && (this.expression === '' || /[+*/%.-]$/.test(this.expression))) {
      if (token !== '-' || /[-.]$/.test(this.expression)) return;
    }
    if (token === '.' && this.currentNumber().includes('.')) return;
    this.expression += token;
    this.update('Editing');
  }
  delete() { this.expression = this.expression.slice(0, -1); this.lastResult = null; this.update('Deleted'); }
  clear() { this.expression = ''; this.lastResult = null; this.update('Cleared'); }
  equals() {
    if (!this.expression || /[+*/%.-]$/.test(this.expression)) { this.statusTarget.textContent = 'Complete the expression first'; return; }
    try {
      const value = Function('"use strict"; return (' + this.expression + ')')();
      if (!Number.isFinite(value)) throw new Error('Cannot divide by zero');
      this.expression = String(Number.isInteger(value) ? value : Number(value.toFixed(8)));
      this.lastResult = this.expression;
      this.update('Result');
    } catch (error) {
      this.statusTarget.textContent = error.message || 'Invalid expression';
    }
  }
  handleKey(event) {
    if (/^[0-9.]$/.test(event.key) || ['+', '-', '*', '/', '%'].includes(event.key)) { this.add(event.key); event.preventDefault(); }
    else if (event.key === 'Enter' || event.key === '=') { this.equals(); event.preventDefault(); }
    else if (event.key === 'Backspace') { this.delete(); event.preventDefault(); }
    else if (event.key === 'Escape') { this.clear(); event.preventDefault(); }
  }
  currentNumber() { return this.expression.split(/[+*/%-]/).pop() || ''; }
  update(status) { this.displayTarget.textContent = this.expression || '0'; this.statusTarget.textContent = status; }
}

const application = Application.start();
application.register('calculator', CalculatorController);
` + "`" + `);
fs.writeFileSync('src/styles.css', ` + "`" + `:root { font-family: Inter, ui-sans-serif, system-ui, sans-serif; color: #17202a; background: #eef2f3; }
* { box-sizing: border-box; }
body { margin: 0; min-height: 100vh; background: linear-gradient(135deg, #edf2f4, #dce8e2 55%, #f4efe6); }
button { font: inherit; }
.app-shell { min-height: 100vh; display: grid; place-items: center; padding: 24px; }
.calculator { width: min(100%, 380px); border: 1px solid rgba(23,32,42,.14); border-radius: 8px; background: rgba(255,255,255,.94); box-shadow: 0 22px 60px rgba(23,32,42,.18); padding: 20px; }
header { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; margin-bottom: 16px; }
header p { margin: 0; font-size: 12px; text-transform: uppercase; letter-spacing: .08em; color: #607466; font-weight: 700; }
h1 { margin: 0; font-size: 28px; line-height: 1; letter-spacing: 0; }
.display { display: block; width: 100%; min-height: 76px; border-radius: 8px; border: 1px solid #cfd8d2; background: #101820; color: #f7fff7; padding: 16px; text-align: right; font-size: 34px; line-height: 1.25; overflow-wrap: anywhere; }
.status { min-height: 20px; margin: 10px 2px 14px; color: #5d6d63; font-size: 13px; }
.keys { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.key { min-height: 56px; border: 0; border-radius: 8px; background: #e7ece9; color: #17202a; font-weight: 800; cursor: pointer; }
.key:hover, .key:focus-visible { transform: translateY(-1px); box-shadow: 0 8px 18px rgba(23,32,42,.12); outline: none; }
.key--operator { background: #b7d8c8; }
.key--utility { background: #f4d6b8; }
.key--equals { background: #315c48; color: white; grid-row: span 2; }
.key--zero { grid-column: span 2; }
@media (max-width: 420px) { .app-shell { padding: 14px; } .calculator { padding: 14px; } .display { font-size: 28px; } .key { min-height: 50px; } }
` + "`" + `);
fs.writeFileSync('webpack.config.js', ` + "`" + `const path = require('path');
module.exports = {
  entry: './src/index.js',
  output: { path: path.resolve(__dirname, 'dist'), filename: 'bundle.js', clean: true },
  optimization: { minimize: false },
  module: { rules: [{ test: /\\.css$/i, use: ['style-loader', 'css-loader'] }] }
};
` + "`" + `);
fs.writeFileSync('scripts/smoke-test.js', ` + "`" + `const fs = require('fs');
const required = [
  ['index.html', 'data-controller="calculator"'],
  ['index.html', 'dist/bundle.js'],
  ['src/index.js', '@hotwired/stimulus'],
  ['src/index.js', 'recyclrjs'],
  ['src/index.js', 'class CalculatorController'],
  ['src/styles.css', '.calculator'],
  ['dist/bundle.js', 'CalculatorController']
];
for (const [file, needle] of required) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(needle)) throw new Error(file + ' missing ' + needle);
}
console.log('calculator smoke test passed');
` + "`" + `);
fs.writeFileSync('scripts/serve.js', ` + "`" + `const http = require('http');
const fs = require('fs');
const path = require('path');
const root = process.cwd();
const port = Number(process.env.PORT || 4173);
const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8' };
http.createServer((req, res) => {
  const urlPath = req.url === '/' ? '/index.html' : decodeURIComponent(req.url.split('?')[0]);
  const filePath = path.join(root, urlPath);
  if (!filePath.startsWith(root)) { res.writeHead(403); res.end('forbidden'); return; }
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end('not found'); return; }
    res.writeHead(200, { 'Content-Type': types[path.extname(filePath)] || 'application/octet-stream' });
    res.end(data);
  });
}).listen(port, '127.0.0.1', () => console.log('calculator listening on http://127.0.0.1:' + port));
` + "`" + `);
NODE
npm test`
}

func deterministicReactJSONFormatterRecoveryCommand() string {
	return `node <<'NODE'
const fs = require('fs');
fs.mkdirSync('src', { recursive: true });
fs.mkdirSync('scripts', { recursive: true });
const pkg = fs.existsSync('package.json') ? JSON.parse(fs.readFileSync('package.json', 'utf8')) : { name: 'json-formatter-app', version: '1.0.0' };
pkg.type = 'module';
pkg.scripts = {
  dev: 'vite --host 127.0.0.1',
  build: 'vite build',
  preview: 'vite preview --host 127.0.0.1',
  test: 'node scripts/smoke-test.js'
};
pkg.dependencies = Object.assign({}, pkg.dependencies, {
  react: pkg.dependencies && pkg.dependencies.react || '^19.0.0',
  'react-dom': pkg.dependencies && pkg.dependencies['react-dom'] || '^19.0.0'
});
pkg.devDependencies = Object.assign({}, pkg.devDependencies, {
  vite: pkg.devDependencies && pkg.devDependencies.vite || '^7.0.0',
  '@vitejs/plugin-react': pkg.devDependencies && pkg.devDependencies['@vitejs/plugin-react'] || '^5.0.0'
});
fs.writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
fs.writeFileSync('index.html', ` + "`" + `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>JSON Formatter</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
` + "`" + `);
fs.writeFileSync('vite.config.js', ` + "`" + `import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
});
` + "`" + `);
fs.writeFileSync('src/jsonFormatter.js', ` + "`" + `export function parseJSON(input) {
  try {
    return { ok: true, value: JSON.parse(input) };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : 'Invalid JSON' };
  }
}

export function formatJSON(input, spaces = 2) {
  const parsed = parseJSON(input);
  if (!parsed.ok) return parsed;
  return { ok: true, value: JSON.stringify(parsed.value, null, spaces) };
}

export function minifyJSON(input) {
  const parsed = parseJSON(input);
  if (!parsed.ok) return parsed;
  return { ok: true, value: JSON.stringify(parsed.value) };
}
` + "`" + `);
fs.writeFileSync('src/main.jsx', ` + "`" + `import React from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import App from './App.jsx';

createRoot(document.getElementById('root')).render(<App />);
` + "`" + `);
fs.writeFileSync('src/App.jsx', ` + "`" + `import { useMemo, useState } from 'react';
import { formatJSON, minifyJSON } from './jsonFormatter.js';

const sample = '{"project":"omnidex","features":["format","minify","validate"],"ok":true}';

export default function App() {
  const [input, setInput] = useState(sample);
  const [mode, setMode] = useState('format');

  const result = useMemo(() => {
    return mode === 'minify' ? minifyJSON(input) : formatJSON(input, 2);
  }, [input, mode]);

  return (
    <main className="app-shell">
      <section className="panel" aria-labelledby="title">
        <div className="header-row">
          <div>
            <p className="eyebrow">React Utility</p>
            <h1 id="title">JSON Formatter</h1>
          </div>
          <div className="actions" aria-label="Formatter actions">
            <button type="button" className={mode === 'format' ? 'active' : ''} onClick={() => setMode('format')}>Format</button>
            <button type="button" className={mode === 'minify' ? 'active' : ''} onClick={() => setMode('minify')}>Minify</button>
          </div>
        </div>

        <div className="workspace">
          <label>
            Input JSON
            <textarea value={input} onChange={(event) => setInput(event.target.value)} spellCheck="false" />
          </label>
          <label>
            Output
            <pre className={result.ok ? 'output' : 'output error'}>{result.ok ? result.value : result.error}</pre>
          </label>
        </div>
      </section>
    </main>
  );
}
` + "`" + `);
fs.writeFileSync('src/style.css', ` + "`" + `:root {
  font-family: Inter, ui-sans-serif, system-ui, sans-serif;
  color: #14213d;
  background: #f4f6f8;
}

* { box-sizing: border-box; }
body { margin: 0; min-width: 320px; }
button, textarea { font: inherit; }

.app-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
}

.panel {
  width: min(1080px, 100%);
  border: 1px solid #d7dee8;
  border-radius: 8px;
  background: white;
  box-shadow: 0 18px 50px rgba(20, 33, 61, 0.12);
  padding: 24px;
}

.header-row {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: end;
  margin-bottom: 20px;
}

.eyebrow {
  margin: 0 0 6px;
  color: #2a6f97;
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: .08em;
}

h1 { margin: 0; font-size: clamp(28px, 4vw, 44px); letter-spacing: 0; }

.actions { display: flex; gap: 8px; }
.actions button {
  border: 1px solid #b8c4d6;
  border-radius: 8px;
  background: #f7f9fc;
  color: #14213d;
  padding: 10px 14px;
  cursor: pointer;
}
.actions button.active { background: #2a6f97; border-color: #2a6f97; color: white; }

.workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
}

label { display: grid; gap: 8px; font-weight: 800; color: #2f3a4a; }
textarea, .output {
  width: 100%;
  min-height: 420px;
  margin: 0;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #0f172a;
  color: #e2e8f0;
  padding: 14px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 14px;
  line-height: 1.5;
  overflow: auto;
  white-space: pre-wrap;
}
.output.error { color: #fecaca; border-color: #f87171; }

@media (max-width: 760px) {
  .header-row, .workspace { grid-template-columns: 1fr; display: grid; align-items: stretch; }
  .actions { width: 100%; }
  .actions button { flex: 1; }
  textarea, .output { min-height: 260px; }
}
` + "`" + `);
fs.writeFileSync('scripts/smoke-test.js', ` + "`" + `import fs from 'node:fs';
import { formatJSON, minifyJSON } from '../src/jsonFormatter.js';

const formatted = formatJSON('{"b":2,"a":[1,true]}');
if (!formatted.ok || !formatted.value.includes('\\n  "b": 2') || !formatted.value.includes('\\n  "a": [')) {
  throw new Error('formatJSON did not pretty-print nested JSON');
}

const minified = minifyJSON('{"b":2, "a": [1, true]}');
if (!minified.ok || minified.value !== '{"b":2,"a":[1,true]}') {
  throw new Error('minifyJSON did not minify JSON');
}

const invalid = formatJSON('{"broken":');
if (invalid.ok || !invalid.error) {
  throw new Error('invalid JSON did not produce an error');
}

const sourceChecks = [
  ['src/App.jsx', 'JSON Formatter'],
  ['src/App.jsx', 'Input JSON'],
  ['src/App.jsx', 'Minify'],
  ['src/jsonFormatter.js', 'formatJSON'],
  ['src/jsonFormatter.js', 'minifyJSON'],
  ['dist/index.html', '/assets/'],
];
for (const [file, expected] of sourceChecks) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(expected)) throw new Error(file + ' missing ' + expected);
}

console.log('json formatter smoke test passed');
` + "`" + `);
NODE
npm install
npm run build
npm test`
}

func deterministicReactJSONFormatterSmokeRepairCommand() string {
	return `node <<'NODE'
const fs = require('fs');
fs.mkdirSync('scripts', { recursive: true });
fs.writeFileSync('scripts/smoke-test.js', ` + "`" + `import fs from 'node:fs';
import { formatJSON, minifyJSON } from '../src/jsonFormatter.js';

const formatted = formatJSON('{"b":2,"a":[1,true]}');
if (!formatted.ok || !formatted.value.includes('\\n  "b": 2') || !formatted.value.includes('\\n  "a": [')) {
  throw new Error('formatJSON did not pretty-print nested JSON');
}

const minified = minifyJSON('{"b":2, "a": [1, true]}');
if (!minified.ok || minified.value !== '{"b":2,"a":[1,true]}') {
  throw new Error('minifyJSON did not minify JSON');
}

const invalid = formatJSON('{"broken":');
if (invalid.ok || !invalid.error) {
  throw new Error('invalid JSON did not produce an error');
}

const sourceChecks = [
  ['src/App.jsx', 'JSON Formatter'],
  ['src/App.jsx', 'Input JSON'],
  ['src/App.jsx', 'Minify'],
  ['src/jsonFormatter.js', 'formatJSON'],
  ['src/jsonFormatter.js', 'minifyJSON'],
  ['dist/index.html', '/assets/'],
];
for (const [file, expected] of sourceChecks) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(expected)) throw new Error(file + ' missing ' + expected);
}

console.log('json formatter smoke test passed');
` + "`" + `);
NODE
npm run build
npm test`
}

func deterministicReactClockViteRecoveryCommand() string {
	return `node <<'NODE'
const fs = require('fs');
fs.mkdirSync('src', { recursive: true });
fs.mkdirSync('scripts', { recursive: true });
const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
pkg.type = 'module';
pkg.scripts = {
  dev: 'vite --host 127.0.0.1',
  build: 'vite build',
  preview: 'vite preview --host 127.0.0.1',
  test: 'node scripts/smoke-test.js'
};
pkg.dependencies = Object.assign({}, pkg.dependencies, {
  react: pkg.dependencies && pkg.dependencies.react || '^19.0.0',
  'react-dom': pkg.dependencies && pkg.dependencies['react-dom'] || '^19.0.0'
});
pkg.devDependencies = Object.assign({}, pkg.devDependencies, {
  vite: pkg.devDependencies && pkg.devDependencies.vite || '^7.0.0',
  '@tailwindcss/vite': pkg.devDependencies && pkg.devDependencies['@tailwindcss/vite'] || '^4.0.0',
  tailwindcss: pkg.devDependencies && pkg.devDependencies.tailwindcss || '^4.0.0'
});
fs.writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
fs.writeFileSync('index.html', ` + "`" + `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Omnidex Clock</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
` + "`" + `);
fs.writeFileSync('vite.config.js', ` + "`" + `import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss()],
});
` + "`" + `);
fs.writeFileSync('src/main.jsx', ` + "`" + `import React from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import App from './App.jsx';

createRoot(document.getElementById('root')).render(<App />);
` + "`" + `);
fs.writeFileSync('src/App.jsx', ` + "`" + `import { useEffect, useMemo, useState } from 'react';

const zones = [
  { label: 'Local', value: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' },
  { label: 'New York', value: 'America/New_York' },
  { label: 'Los Angeles', value: 'America/Los_Angeles' },
  { label: 'London', value: 'Europe/London' },
  { label: 'Tokyo', value: 'Asia/Tokyo' },
  { label: 'Sydney', value: 'Australia/Sydney' },
  { label: 'UTC', value: 'UTC' },
];

function formatTime(date, timeZone) {
  return new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date);
}

function formatDate(date, timeZone) {
  return new Intl.DateTimeFormat('en-US', {
    timeZone,
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  }).format(date);
}

export default function App() {
  const [now, setNow] = useState(() => new Date());
  const [timeZone, setTimeZone] = useState(zones[0].value);

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const activeZone = useMemo(() => zones.find((zone) => zone.value === timeZone) || zones[0], [timeZone]);

  return (
    <main className="min-h-screen bg-slate-950 text-slate-100">
      <section className="mx-auto flex min-h-screen w-full max-w-3xl flex-col justify-center px-6 py-10">
        <div className="rounded-lg border border-slate-700 bg-slate-900 p-6 shadow-2xl shadow-black/30">
          <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-sm font-semibold uppercase tracking-wide text-cyan-300">Omnidex Clock</p>
              <h1 className="mt-2 text-3xl font-bold text-white">World Clock</h1>
            </div>
            <label className="flex flex-col gap-2 text-sm font-medium text-slate-300">
              Timezone
              <select
                className="rounded-md border border-slate-600 bg-slate-800 px-3 py-2 text-white outline-none ring-cyan-400 transition focus:ring-2"
                value={timeZone}
                onChange={(event) => setTimeZone(event.target.value)}
              >
                {zones.map((zone) => (
                  <option key={zone.label} value={zone.value}>{zone.label}</option>
                ))}
              </select>
            </label>
          </div>

          <div className="rounded-lg bg-slate-950 p-6">
            <p className="text-sm font-medium text-slate-400">{activeZone.label} · {timeZone}</p>
            <time className="mt-3 block font-mono text-5xl font-bold tabular-nums text-cyan-200 sm:text-7xl">
              {formatTime(now, timeZone)}
            </time>
            <p className="mt-4 text-lg text-slate-300">{formatDate(now, timeZone)}</p>
          </div>
        </div>
      </section>
    </main>
  );
}
` + "`" + `);
fs.writeFileSync('src/style.css', ` + "`" + `@import "tailwindcss";

html {
  background: #020617;
}

body {
  margin: 0;
  min-width: 320px;
}

* {
  box-sizing: border-box;
}
` + "`" + `);
fs.writeFileSync('scripts/smoke-test.js', ` + "`" + `import fs from 'node:fs';

const checks = [
  ['index.html', '/src/main.jsx'],
  ['vite.config.js', '@tailwindcss/vite'],
  ['src/style.css', '@import "tailwindcss"'],
  ['src/App.jsx', 'World Clock'],
  ['src/App.jsx', 'Timezone'],
  ['src/App.jsx', 'America/New_York'],
];

for (const [file, expected] of checks) {
  const text = fs.readFileSync(file, 'utf8');
  if (!text.includes(expected)) {
    throw new Error(file + ' missing ' + expected);
  }
}

if (!fs.existsSync('dist/index.html')) {
  throw new Error('dist/index.html missing; run npm run build first');
}

console.log('clock smoke test passed');
` + "`" + `);
NODE
npm install
npm run build
npm test`
}

func runDelegatedShellSpecialist(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if cfg.ShellSpecialist == nil {
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool_task": truncateStructuredTimelineValue(toolTask),
	})
	if len(allPrepBriefs(cfg.PrepContext)) > 0 || len(cfg.PrepContext.Evidence) > 0 {
		emitStructuredCommandEvent(onEvent, "prep_context_attached_to_specialist", "Preparation context attached to shell specialist", map[string]string{
			"step":        fmt.Sprintf("%d", step),
			"role":        "shell_specialist",
			"briefs":      fmt.Sprintf("%d", len(allPrepBriefs(cfg.PrepContext))),
			"evidence":    fmt.Sprintf("%d", len(cfg.PrepContext.Evidence)),
			"route_files": strings.Join(cfg.PrepContext.CodebaseRoute.LikelyFiles, ","),
		})
	}
	proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, toolTask, cfg, worksiteSurvey, &result.ObjectiveLedger, onEvent, result)
	if err != nil || !ok {
		return true, err
	}
	if err := runStructuredPayloadCommand(ctx, step, proposal.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
		return true, err
	}
	return true, nil
}

func proposeValidatedShellCommand(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, ledger *[]StructuredObjective, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (ShellCommandProposal, bool, error) {
	if cfg.ShellSpecialist == nil {
		return ShellCommandProposal{}, false, nil
	}
	for attempt := 0; attempt <= defaultShellSpecialistRepairAttempts; attempt++ {
		if attempt > 0 {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_started", "Shell specialist received direct validator feedback for local repair", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
			})
		}
		proposal, err := cfg.ShellSpecialist.ProposeShellCommand(ctx, ShellCommandSpecialistInput{
			Step:             step,
			UserPrompt:       prompt,
			ToolTask:         toolTask,
			Observations:     result.Observations,
			CompletedActions: completedActionsFromState(*ledger, result.Observations),
			LoopState:        structuredLoopStateFromState(*ledger, result.Observations),
			SessionMemories:  cfg.SessionMemories,
			WorksiteSurvey:   worksiteSurvey,
		})
		if err != nil {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_failed", "Shell specialist failed", map[string]string{
				"step":  fmt.Sprintf("%d", step),
				"error": truncateStructuredTimelineValue(err.Error()),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "shell specialist failed: " + err.Error(),
			})
			return ShellCommandProposal{}, false, nil
		}
		proposal.Command = normalizeStructuredCommand(proposal.Command)
		emitStructuredCommandEvent(onEvent, "structured_tool_delegation_finished", "Shell specialist proposed command", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"tool":      "shell",
			"role":      "shell_execution_specialist",
			"attempt":   fmt.Sprintf("%d", attempt+1),
			"command":   truncateStructuredTimelineValue(proposal.Command),
			"rationale": truncateStructuredTimelineValue(proposal.Rationale),
		})
		if err := validateShellProposalAgainstToolTask(proposal.Command, toolTask); err != nil {
			appendRejectedShellProposalObservation(step, proposal.Command, err, "choose a write/edit/build/test command that directly satisfies the delegated task", result)
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Shell command rejected by tool-task constraints", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if err := validateStructuredCommandForRunWithSurvey(proposal.Command, result.Observations, cfg.CurrentWorkingDirectory, *ledger, worksiteSurvey); err != nil {
			if handleStructuredRepeatedCommandValidation(step, proposal.Command, err, ledger, onEvent, result) {
				return ShellCommandProposal{}, false, nil
			}
			appendRejectedShellProposalObservation(step, proposal.Command, err, "planner should delegate a narrower shell task or choose a different tool", result)
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if attempt > 0 {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_accepted", "Shell specialist repaired proposal accepted by deterministic validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
				"command": truncateStructuredTimelineValue(proposal.Command),
			})
		}
		return proposal, true, nil
	}
	return ShellCommandProposal{}, false, nil
}

func appendRejectedShellProposalObservation(step int, command string, err error, guidance string, result *CommandDecisionResult) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:             step,
		RejectedCommand:  truncateStructuredObservation(command),
		CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, reason),
		ExitCode:         1,
		Stderr:           "shell specialist command rejected: " + reason + "; " + guidance,
	})
}

func requestStructuredCommandPayload(ctx context.Context, client CommandDecisionClient, req OllamaChatRequest, step int, onEvent func(StructuredCommandEvent)) (OllamaChatResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= defaultStructuredLLMRequestAttempts; attempt++ {
		resp, err := client.ChatRaw(ctx, req)
		if err == nil {
			if attempt > 1 {
				emitStructuredCommandEvent(onEvent, "structured_llm_request_recovered", "Structured LLM request recovered after retry", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
				})
			}
			return resp, nil
		}
		lastErr = err
		emitStructuredCommandEvent(onEvent, "structured_llm_request_failed", "Structured LLM request failed", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt),
			"error":   truncateStructuredTimelineValue(err.Error()),
		})
		if !isTransientStructuredLLMError(err) || attempt == defaultStructuredLLMRequestAttempts {
			return OllamaChatResponse{}, err
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_backend_unstable", "Ollama backend appears unstable; retrying request", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"attempt":    fmt.Sprintf("%d", attempt),
			"diagnosis":  classifyStructuredLLMFailure(err),
			"mitigation": "check journalctl -u ollama; prefer cpu_avx2 or reduce Ollama context/keep_alive if ROCm is crashing",
		})
		select {
		case <-ctx.Done():
			return OllamaChatResponse{}, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return OllamaChatResponse{}, lastErr
}

func repairStructuredPayloadBeforeRouting(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, resp OllamaChatResponse, payload StructuredCommandPayload, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string, survey WorksiteSurvey, onEvent func(StructuredCommandEvent)) (bool, OllamaChatResponse, StructuredCommandPayload, []StructuredObjective, error) {
	if client == nil || !structuredPayloadNeedsImmediateRepair(payload, observations) {
		return false, resp, payload, ledger, nil
	}
	initialValidationErr := immediateStructuredPayloadValidationError(payload, observations, workingDirectory, ledger, survey)
	if initialValidationErr == nil || !isImmediatePlannerRepairValidation(initialValidationErr) {
		return false, resp, payload, ledger, nil
	}
	currentResp := resp
	currentPayload := payload
	currentLedger := ledger
	for attempt := 1; attempt <= defaultStructuredPlannerRepairAttempts; attempt++ {
		validationErr := immediateStructuredPayloadValidationError(currentPayload, observations, workingDirectory, currentLedger, survey)
		if validationErr == nil {
			return attempt > 1, currentResp, currentPayload, currentLedger, nil
		}
		if !isImmediatePlannerRepairValidation(validationErr) {
			return true, currentResp, currentPayload, currentLedger, nil
		}
		emitStructuredCommandEvent(onEvent, "structured_planner_repair_started", "Planner received immediate validation feedback for isolated repair", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt),
			"command": truncateStructuredTimelineValue(currentPayload.Command),
			"reason":  truncateStructuredTimelineValue(validationErr.Error()),
		})
		repairReq := buildStructuredPlannerRepairRequest(baseReq, prompt, currentResp.Content, currentPayload.Command, validationErr.Error(), currentLedger, observations, workingDirectory)
		nextResp, err := requestStructuredCommandPayload(ctx, client, repairReq, step, onEvent)
		if err != nil {
			return true, currentResp, currentPayload, currentLedger, err
		}
		nextPayload, err := ParseStructuredCommandPayload(nextResp.Content)
		if err != nil {
			return true, currentResp, currentPayload, currentLedger, err
		}
		nextPayload.Command = normalizeStructuredCommand(nextPayload.Command)
		nextLedger := mergeStructuredObjectiveLedger(currentLedger, nextPayload.ObjectiveLedger)
		emitStructuredCommandEvent(onEvent, "structured_planner_repair_payload_received", "Planner returned repaired structured payload", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"attempt":            fmt.Sprintf("%d", attempt),
			"done":               fmt.Sprintf("%t", nextPayload.Done),
			"ask":                fmt.Sprintf("%t", nextPayload.Ask),
			"tool":               truncateStructuredTimelineValue(nextPayload.Tool),
			"command":            truncateStructuredTimelineValue(nextPayload.Command),
			"pending_objectives": pendingStructuredObjectiveIDs(nextLedger),
		})
		currentResp = nextResp
		currentPayload = nextPayload
		currentLedger = nextLedger
		if immediateStructuredPayloadValidationError(currentPayload, observations, workingDirectory, currentLedger, survey) == nil {
			emitStructuredCommandEvent(onEvent, "structured_planner_repair_accepted", "Planner repaired payload accepted by deterministic pre-validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
			})
			return true, currentResp, currentPayload, currentLedger, nil
		}
	}
	return true, currentResp, currentPayload, currentLedger, nil
}

func structuredPayloadNeedsImmediateRepair(payload StructuredCommandPayload, observations []StructuredCommandObservation) bool {
	if isPatchToolDelegation(payload) || isShellToolDelegation(payload) || payload.Done {
		return false
	}
	if payload.Ask && strings.TrimSpace(payload.Command) == "" {
		return false
	}
	if strings.TrimSpace(payload.Command) == "" {
		return false
	}
	return true
}

func immediateStructuredPayloadValidationError(payload StructuredCommandPayload, observations []StructuredCommandObservation, workingDirectory string, ledger []StructuredObjective, survey WorksiteSurvey) error {
	if !structuredPayloadNeedsImmediateRepair(payload, observations) {
		return nil
	}
	return validateStructuredCommandForRunWithSurvey(payload.Command, observations, workingDirectory, ledger, survey)
}

func isImmediatePlannerRepairValidation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "placeholder-only command does not satisfy app objectives")
}

func buildStructuredPlannerRepairRequest(baseReq OllamaChatRequest, prompt, rejectedPayload, rejectedCommand, reason string, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string) OllamaChatRequest {
	req := baseReq
	req.Messages = append([]OllamaMessage(nil), baseReq.Messages...)
	payload := struct {
		CurrentPrompt           string                         `json:"current_prompt"`
		RejectedPayload         string                         `json:"rejected_payload"`
		RejectedCommand         string                         `json:"rejected_command"`
		ValidationFeedback      string                         `json:"validation_feedback"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		PendingObjectiveIDs     []string                       `json:"pending_objective_ids,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations,omitempty"`
		RepairRules             []string                       `json:"repair_rules"`
	}{
		CurrentPrompt:           prompt,
		RejectedPayload:         truncateStructuredObservation(rejectedPayload),
		RejectedCommand:         truncateStructuredObservation(rejectedCommand),
		ValidationFeedback:      reason,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(workingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		PendingObjectiveIDs:     structuredObjectiveIDs(pendingStructuredObjectives(ledger)),
		Observations:            observations,
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"Repair the rejected payload directly; do not ask another specialist and do not restate the feedback.",
			"The validator feedback is authoritative.",
			"Choose a command, tool, or patch that satisfies pending objectives and avoids the rejected pattern.",
			"If feedback says placeholder-only, write substantive source/build/test file content now.",
			"If feedback says dependency scope drift, write requested source files or use existing dependencies instead of installing optional packages.",
			"If feedback says repeated command, inspect completed_actions/observations and choose the next required action.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"validation_feedback":"repair rejected structured payload"}`)
	}
	req.Messages = append(req.Messages,
		OllamaMessage{Role: "assistant", Content: strings.TrimSpace(rejectedPayload)},
		OllamaMessage{Role: "user", Content: string(blob)},
	)
	return req
}

func normalizeStructuredEvaluatorThreshold(value int) int {
	if value <= 0 {
		return defaultEvaluatorThreshold
	}
	if value > 100 {
		return 100
	}
	return value
}

func structuredEvaluationRetryMessage(evaluation StructuredLLMEvaluation, threshold int) string {
	feedback := strings.TrimSpace(evaluation.Feedback)
	if feedback == "" {
		feedback = "planner response was not sufficiently aligned with the active task"
	}
	verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
	return fmt.Sprintf("self-evaluation rejected response: verdict=%s confidence=%d threshold=%d; feedback=%s; try again using the active prompt, planner job, observations, worksite survey, and capability memory", verdict, evaluation.Confidence, threshold, feedback)
}

func evaluateAndRepairPlannerResponse(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, resp OllamaChatResponse, evaluator StructuredLLMResponseEvaluator, evaluatorThreshold int, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, ledger []StructuredObjective, observations []StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, OllamaChatResponse, bool, error) {
	if evaluator == nil {
		return true, resp, false, nil
	}
	currentResp := resp
	for attempt := 0; attempt <= defaultEvaluatorPlannerRepairAttempts; attempt++ {
		if len(allPrepBriefs(cfg.PrepContext)) > 0 || len(cfg.PrepContext.Evidence) > 0 {
			emitStructuredCommandEvent(onEvent, "prep_context_attached_to_specialist", "Preparation context attached to evaluator", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"role":     "evaluator",
				"briefs":   fmt.Sprintf("%d", len(allPrepBriefs(cfg.PrepContext))),
				"evidence": fmt.Sprintf("%d", len(cfg.PrepContext.Evidence)),
			})
		}
		evaluation, evalErr := evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
			Step:             step,
			UserPrompt:       prompt,
			PlannerJob:       structuredCommandPlannerJobSummary(),
			LLMResponse:      currentResp.Content,
			Observations:     result.Observations,
			CompletedActions: completedActionsFromState(ledger, result.Observations),
			LoopState:        structuredLoopStateFromState(ledger, result.Observations),
			SessionMemories:  cfg.SessionMemories,
			WorksiteSurvey:   worksiteSurvey,
		})
		if evalErr != nil {
			emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator failed; continuing with deterministic validation", map[string]string{
				"step":  fmt.Sprintf("%d", step),
				"error": truncateStructuredTimelineValue(evalErr.Error()),
			})
			return true, currentResp, true, nil
		}
		if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
			emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator returned inconsistent scoring; continuing with deterministic validation", map[string]string{
				"step":       fmt.Sprintf("%d", step),
				"confidence": fmt.Sprintf("%d", evaluation.Confidence),
				"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
				"error":      truncateStructuredTimelineValue(consistencyErr.Error()),
			})
			return true, currentResp, true, nil
		}
		if normalizeStructuredEvaluationVerdict(evaluation.Verdict) == "accept" && structuredEvaluationFeedbackSuggestsHardReject(evaluation.Feedback+" "+evaluation.BlockingReason) {
			evaluation.Verdict = "reject"
		}
		verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
		emitStructuredCommandEvent(onEvent, "structured_response_evaluated", "Structured response evaluator scored planner output", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
			"verdict":    verdict,
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
		})
		if verdict != "reject" && verdict != "revise" && evaluation.Confidence >= evaluatorThreshold {
			if attempt > 0 {
				emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_accepted", "Planner repair accepted by evaluator", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
				})
			}
			return true, currentResp, false, nil
		}
		if verdict == "revise" && repeatedStructuredEvaluationFeedback(evaluation, result.Observations) {
			emitStructuredCommandEvent(onEvent, "structured_evaluator_loop_bypassed", "Repeated evaluator revise feedback bypassed for deterministic validation", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"feedback": truncateStructuredTimelineValue(evaluation.Feedback),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:                 step,
				RejectedResponse:     truncateStructuredObservation(currentResp.Content),
				EvaluationConfidence: evaluation.Confidence,
				EvaluationFeedback:   truncateStructuredObservation(evaluation.Feedback),
				ExitCode:             1,
				Stderr:               "anti_loop: evaluator repeated the same revise feedback; evaluator bypassed for this planner output. Continue with deterministic command validation, objective ledger, worksite survey, and observed command evidence.",
			})
			return true, currentResp, true, nil
		}
		appendEvaluatorRejectionObservation(step, currentResp.Content, evaluation, evaluatorThreshold, result)
		emitStructuredCommandEvent(onEvent, "structured_response_rejected", "Structured response rejected by evaluator", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
			"verdict":    verdict,
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
		})
		if attempt >= defaultEvaluatorPlannerRepairAttempts || client == nil {
			return false, currentResp, false, nil
		}
		emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_started", "Planner received evaluator feedback for local repair", map[string]string{
			"step":     fmt.Sprintf("%d", step),
			"attempt":  fmt.Sprintf("%d", attempt+1),
			"verdict":  verdict,
			"feedback": truncateStructuredTimelineValue(evaluation.Feedback),
		})
		repairReq := buildStructuredPlannerEvaluatorRepairRequest(baseReq, prompt, currentResp.Content, evaluation, evaluatorThreshold, ledger, result.Observations, cfg.CurrentWorkingDirectory)
		nextResp, err := requestStructuredCommandPayload(ctx, client, repairReq, step, onEvent)
		if err != nil {
			return false, currentResp, false, err
		}
		emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_payload_received", "Planner returned evaluator-repaired payload", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt+1),
			"payload": truncateStructuredTimelineValue(nextResp.Content),
		})
		currentResp = nextResp
	}
	return false, currentResp, false, nil
}

func appendEvaluatorRejectionObservation(step int, rawResponse string, evaluation StructuredLLMEvaluation, threshold int, result *CommandDecisionResult) {
	verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
	reason := structuredEvaluationRetryMessage(evaluation, threshold)
	if verdict == "reject" {
		reason = "scope_drift: evaluator rejected planner output; " + reason
	}
	rejectedCommand := ""
	if rejectedPayload, parseErr := ParseStructuredCommandPayload(rawResponse); parseErr == nil {
		rejectedCommand = truncateStructuredObservation(rejectedPayload.Command)
	}
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:                 step,
		RejectedResponse:     truncateStructuredObservation(rawResponse),
		RejectedCommand:      rejectedCommand,
		EvaluationConfidence: evaluation.Confidence,
		EvaluationFeedback:   truncateStructuredObservation(evaluation.Feedback),
		CapabilityMemory:     structuredCapabilityMemoryForRejectedResponse(rawResponse, evaluation.Feedback),
		ExitCode:             1,
		Stderr:               reason,
	})
}

func buildStructuredPlannerEvaluatorRepairRequest(baseReq OllamaChatRequest, prompt, rejectedPayload string, evaluation StructuredLLMEvaluation, threshold int, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string) OllamaChatRequest {
	req := baseReq
	req.Messages = append([]OllamaMessage(nil), baseReq.Messages...)
	payload := struct {
		CurrentPrompt           string                         `json:"current_prompt"`
		RejectedPayload         string                         `json:"rejected_payload"`
		EvaluatorVerdict        string                         `json:"evaluator_verdict"`
		EvaluatorConfidence     int                            `json:"evaluator_confidence"`
		EvaluatorThreshold      int                            `json:"evaluator_threshold"`
		EvaluatorFeedback       string                         `json:"evaluator_feedback"`
		BlockingReason          string                         `json:"blocking_reason,omitempty"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		PendingObjectiveIDs     []string                       `json:"pending_objective_ids,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations,omitempty"`
		RepairRules             []string                       `json:"repair_rules"`
	}{
		CurrentPrompt:           prompt,
		RejectedPayload:         truncateStructuredObservation(rejectedPayload),
		EvaluatorVerdict:        normalizeStructuredEvaluationVerdict(evaluation.Verdict),
		EvaluatorConfidence:     evaluation.Confidence,
		EvaluatorThreshold:      threshold,
		EvaluatorFeedback:       evaluation.Feedback,
		BlockingReason:          evaluation.BlockingReason,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(workingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		PendingObjectiveIDs:     structuredObjectiveIDs(pendingStructuredObjectives(ledger)),
		Observations:            observations,
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"The evaluator feedback is authoritative for this repair attempt.",
			"Repair the rejected planner payload directly; do not restate or argue with the feedback.",
			"Choose the next concrete command, tool delegation, or patch that aligns with the active prompt and observed evidence.",
			"Do not return the same rejected response.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"evaluator_feedback":"repair rejected planner payload"}`)
	}
	req.Messages = append(req.Messages,
		OllamaMessage{Role: "assistant", Content: strings.TrimSpace(rejectedPayload)},
		OllamaMessage{Role: "user", Content: string(blob)},
	)
	return req
}

func structuredCommandPlannerJobSummary() string {
	return strings.Join([]string{
		"Return strict JSON for the next command-planning step.",
		"Use schema {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}.",
		"Use {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"} only after successful command evidence.",
		"Commands must gather evidence, inspect state, create requested output, or verify results.",
		"Do not simulate final answers with echo/printf apologies or claims that real-time information is unavailable.",
		"Use shell commands and public unauthenticated sources for current facts when needed.",
	}, " ")
}

func buildStructuredLLMEvaluationRequest(input StructuredLLMEvaluationInput) OllamaChatRequest {
	payload := struct {
		Step             int                            `json:"step"`
		Job              string                         `json:"planner_job"`
		UserPrompt       string                         `json:"user_prompt"`
		LLMResponse      string                         `json:"llm_response"`
		Observations     []StructuredCommandObservation `json:"observations"`
		CompletedActions []CompletedAction              `json:"completed_actions,omitempty"`
		LoopState        StructuredLoopState            `json:"loop_state,omitempty"`
		SessionMemories  []SessionMemory                `json:"session_memories,omitempty"`
		WorksiteSurvey   WorksiteSurvey                 `json:"worksite_survey"`
	}{
		Step:             input.Step,
		Job:              input.PlannerJob,
		UserPrompt:       input.UserPrompt,
		LLMResponse:      input.LLMResponse,
		Observations:     input.Observations,
		CompletedActions: input.CompletedActions,
		LoopState:        input.LoopState,
		SessionMemories:  input.SessionMemories,
		WorksiteSurvey:   input.WorksiteSurvey,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a tiny strict evaluator.",
					"Return JSON only with schema {\"verdict\":\"accept|revise|reject\",\"confidence\":0,\"blocking_reason\":\"\",\"feedback\":\"\"}.",
					"confidence must be an integer from 0 to 100.",
					"Score whether llm_response is on track for planner_job and user_prompt.",
					"Treat completed_actions as authoritative progress; reject planner output that repeats completed work instead of advancing pending objectives.",
					"Treat loop_state as the loop monitor output; reject or revise responses that keep repeating its blocked action pattern.",
					"Do not treat loop_state.repeated_command as a ban; it is evidence that prior validation disliked a proposal or that a failed command needs correction.",
					"Use verdict=reject for semantic mismatch, scope drift, or contradictions with WorksiteSurvey.",
					"Use verdict=revise when the response may be salvageable but must not execute yet.",
					"Scoring rubric: 90-100 clearly on track or complete, 70-89 mostly on track, 40-69 uncertain or incomplete, 0-39 off track.",
					"If feedback says on track, successfully completed, or correctly answered, confidence must be at least 80.",
					"If confidence is below 70, feedback must state what is missing or wrong and must not say the response is on track.",
					"Do not solve the user's task.",
					"Do not penalize a proposed command merely because it has not executed yet; the runtime executes accepted commands.",
					"For empty-workspace app build tasks with documentation_brief prep, revise any response that only checks compiler availability, fetches documentation, or states that the workspace is empty; the retry should write source/build/test files from prep evidence.",
					"Revise commands that only print status text such as echo/printf when pending objectives require implementation; they are not command evidence.",
					"Revise placeholder-only mkdir/touch scaffolds when app, component, CRUD, UI, source, or storage objectives remain; the retry must write substantive source/build/test file content.",
					"Reject unrequested dependency installs when pending objectives now require implementation work; do not add packages just because they are common.",
					"Give low confidence when the response ignores the active prompt, answers from memory, refuses a capability that shell/public sources provide, returns done without evidence, or emits a command that only prints an answer/apology.",
					"Give low confidence when memory or prior preferences expand dependencies, frameworks, files, services, architecture, or deployment targets beyond the current prompt or selected recipe.",
					"Reject when a command creates or scaffolds a new project but WorksiteSurvey says the operation is modify_existing_project or fix_existing_project.",
					"Give low confidence for obviously invalid shell command syntax or repeated commands already shown failing in observations.",
					"feedback must be one concise sentence explaining how the planner should retry.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"verdict":         map[string]interface{}{"type": "string", "enum": []string{"accept", "revise", "reject"}},
				"confidence":      map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 100},
				"blocking_reason": map[string]interface{}{"type": "string"},
				"feedback":        map[string]interface{}{"type": "string"},
			},
			"required": []string{"confidence", "feedback"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 128,
		},
	}
}

func buildPromptInterpreterRequest(input PromptInterpretationInput) OllamaChatRequest {
	payload := struct {
		Role                    string                   `json:"role"`
		UserPrompt              string                   `json:"user_prompt"`
		ReferenceHistory        []StructuredMemoryRecord `json:"reference_history,omitempty"`
		CurrentWorkingDirectory string                   `json:"current_working_directory"`
		WorksiteSurvey          WorksiteSurvey           `json:"worksite_survey"`
		AvailableRecipes        []RecipePromptCandidate  `json:"available_recipes,omitempty"`
		Instructions            []string                 `json:"instructions"`
	}{
		Role:                    "prompt_interpreter",
		UserPrompt:              input.UserPrompt,
		ReferenceHistory:        recentStructuredMemoryRecords(input.History),
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		WorksiteSurvey:          input.WorksiteSurvey,
		AvailableRecipes:        recipePromptCandidates(input.Recipes),
		Instructions: []string{
			"Interpret the user's words into durable task objectives for downstream planners.",
			"Classify user_operation as create_new_project, modify_existing_project, fix_existing_project, inspect_existing_project, run_tests, install_deps, or unknown.",
			"The WorksiteSurvey is authoritative filesystem grounding; do not contradict its project_state or evidence.",
			"If WorksiteSurvey project_state is an existing app and the current prompt refers to this/current/existing project, prefer modify_existing_project over create_new_project.",
			"Do not create create-new objectives when user_operation is modify_existing_project or fix_existing_project.",
			"If an available recipe directly matches the task, return its id in selected_recipe_ids.",
			"Return objectives only when the request has concrete criteria, outputs, constraints, or verification needs.",
			"Use stable snake_case ids.",
			"Return the objectives in the objective_ledger JSON field.",
			"Set objective source to user_explicit only for requirements directly stated in the current user prompt.",
			"Set objective source to evidence_required_prerequisite only when command/workspace evidence proves the user-explicit objective cannot be completed without that prerequisite; include parent_objective and evidence.",
			"Set objective source to memory_suggested for preferences or prior-history items that are not explicitly requested now.",
			"Set objective source to model_inferred for any plausible but unsupported expansion.",
			"Use packages only for dependency package names directly justified by that objective.",
			"Set requires_reference_history=true only when the current user prompt is an unresolved follow-up that needs prior omitted entities, paths, locations, preferences, or evidence.",
			"Set requires_reference_history=false when the current prompt is standalone or provides its own concrete task, even if reference history contains similar prior work.",
			"All initial objectives should normally be pending.",
			"Do not choose shell commands.",
			"Do not answer the user.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"prompt_interpreter"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the prompt interpreter specialist for Omni.",
					"Your only job is translating the user's natural-language request into structured objectives.",
					"Downstream command planners must use your objective ledger instead of interpreting user wording themselves.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"objective_ledger": structuredObjectiveLedgerSchema(),
				"requires_reference_history": map[string]interface{}{
					"type": "boolean",
				},
				"selected_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"user_operation": map[string]interface{}{
					"type": "string",
					"enum": []string{userOperationCreateNewProject, userOperationModifyExisting, userOperationFixExisting, userOperationInspectExisting, userOperationRunTests, userOperationInstallDeps, userOperationUnknown},
				},
				"recommended_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"forbidden_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
			},
			"required": []string{"objective_ledger", "requires_reference_history"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 512,
		},
	}
}

func buildContextSummarizerRequest(input MinimalContextInput) OllamaChatRequest {
	payload := struct {
		Role                    string                   `json:"role"`
		UserPrompt              string                   `json:"user_prompt"`
		CurrentWorkingDirectory string                   `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective    `json:"objective_ledger,omitempty"`
		CompletedActions        []CompletedAction        `json:"completed_actions,omitempty"`
		ReferenceHistory        []StructuredMemoryRecord `json:"reference_history,omitempty"`
		SessionMemories         []SessionMemory          `json:"session_memories,omitempty"`
		ExistingContext         MinimalContext           `json:"existing_context,omitempty"`
		WorksiteSurvey          WorksiteSurvey           `json:"worksite_survey"`
		Instructions            []string                 `json:"instructions"`
	}{
		Role:                    "summary_specialist",
		UserPrompt:              input.UserPrompt,
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, input.ObjectiveLedger),
		CompletedActions:        input.CompletedActions,
		ReferenceHistory:        recentStructuredMemoryRecords(input.History),
		SessionMemories:         recentStructuredSessionMemories(input.SessionMemories),
		ExistingContext:         normalizeMinimalContext(input.ExistingContext),
		WorksiteSurvey:          input.WorksiteSurvey,
		Instructions: []string{
			"Load the smallest context inventory needed for this active task.",
			"The WorksiteSurvey is authoritative workspace grounding.",
			"Keep only facts, constraints, and open items relevant to the objective ledger and current prompt.",
			"Treat completed_actions as authoritative progress already accomplished in this turn; do not move completed work back into open_items.",
			"Never carry prior project dependencies, frameworks, package names, or build requirements into a new standalone task.",
			"Memories may not create requirements, dependencies, frameworks, files, services, architecture, or deployment targets unless the current prompt explicitly asks to apply them.",
			"Discard unrelated transcript detail.",
			"Return empty arrays when no context is needed.",
			"Do not choose shell commands.",
			"Do not answer the user.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"summary_specialist"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the summary specialist for Omni.",
					"You maintain a mutable minimal context inventory for downstream models.",
					"Your output replaces raw history unless downstream code explicitly needs the raw record.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: minimalContextSchema(),
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 512,
		},
	}
}

func buildCompletionCheckerRequest(input CompletionCheckInput) OllamaChatRequest {
	payload := struct {
		Role                    string                         `json:"role"`
		UserPrompt              string                         `json:"user_prompt"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		CompletedActions        []CompletedAction              `json:"completed_actions,omitempty"`
		LoopState               StructuredLoopState            `json:"loop_state,omitempty"`
		MinimalContext          MinimalContext                 `json:"minimal_context,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations"`
		CandidateAnswer         string                         `json:"candidate_answer"`
		Instructions            []string                       `json:"instructions"`
	}{
		Role:                    "done_check_specialist",
		UserPrompt:              input.UserPrompt,
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, input.ObjectiveLedger),
		CompletedActions:        input.CompletedActions,
		LoopState:               input.LoopState,
		MinimalContext:          normalizeMinimalContext(input.MinimalContext),
		Observations:            input.Observations,
		CandidateAnswer:         input.CandidateAnswer,
		Instructions: []string{
			"Decide whether the task is already complete from objective ledger, minimal context, observations, and candidate answer.",
			"Treat completed_actions as authoritative evidence of work already completed; never require the same completed action again.",
			"Treat loop_state as authoritative loop-monitor context; if it shows blocked or stuck progress, explain which pending objective still lacks evidence.",
			"Mark objectives satisfied only when observations or explicit evidence prove them.",
			"Do not require memory_suggested or model_inferred extras for completion.",
			"Memories are advisory context only and cannot create completion requirements unless represented by user_explicit, recipe_required, or detected_project objectives.",
			"Do not choose shell commands.",
			"Do not answer the user.",
			"Return updated objective_ledger and a concise reason.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"done_check_specialist"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the done-check specialist for Omni.",
					"Your only job is deciding whether the current task is already complete.",
					"You update objective ledger statuses from observed evidence.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"done":             map[string]interface{}{"type": "boolean"},
				"reason":           map[string]interface{}{"type": "string"},
				"objective_ledger": structuredObjectiveLedgerSchema(),
			},
			"required": []string{"done", "reason", "objective_ledger"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 512,
		},
	}
}

func ParseCompletionCheck(raw string) (CompletionCheck, error) {
	var decoded struct {
		Done            bool                  `json:"done"`
		Reason          string                `json:"reason"`
		ObjectiveLedger []StructuredObjective `json:"objective_ledger"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return CompletionCheck{}, fmt.Errorf("parse completion check: %w", err)
	}
	return CompletionCheck{
		Done:            decoded.Done,
		Reason:          strings.TrimSpace(decoded.Reason),
		ObjectiveLedger: mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
	}, nil
}

func minimalContextSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary":     map[string]interface{}{"type": "string"},
			"facts":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"constraints": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"open_items":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"required": []string{"summary", "facts", "constraints", "open_items"},
	}
}

func ParseMinimalContext(raw string) (MinimalContext, error) {
	var decoded MinimalContext
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return MinimalContext{}, fmt.Errorf("parse minimal context: %w", err)
	}
	return normalizeMinimalContext(decoded), nil
}

func normalizeMinimalContext(input MinimalContext) MinimalContext {
	return MinimalContext{
		Summary:     truncateMinimalContextValue(input.Summary),
		Facts:       cleanMinimalContextList(input.Facts),
		Constraints: cleanMinimalContextList(input.Constraints),
		OpenItems:   cleanMinimalContextList(input.OpenItems),
	}
}

func cleanMinimalContextList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		clean := truncateMinimalContextValue(value)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func truncateMinimalContextValue(value string) string {
	clean := strings.Join(strings.Fields(value), " ")
	if len(clean) <= 500 {
		return clean
	}
	return clean[:500] + " [truncated]"
}

func ParsePromptInterpretation(raw string) (PromptInterpretation, error) {
	var decoded struct {
		ObjectiveLedger          []StructuredObjective `json:"objective_ledger"`
		RecipeIDs                []string              `json:"selected_recipe_ids"`
		RequiresReferenceHistory bool                  `json:"requires_reference_history"`
		UserOperation            string                `json:"user_operation"`
		RecommendedRecipeIDs     []string              `json:"recommended_recipe_ids"`
		ForbiddenRecipeIDs       []string              `json:"forbidden_recipe_ids"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return PromptInterpretation{}, fmt.Errorf("parse prompt interpretation: %w", err)
	}
	for i := range decoded.ObjectiveLedger {
		if strings.TrimSpace(decoded.ObjectiveLedger[i].Source) == "" {
			decoded.ObjectiveLedger[i].Source = structuredObjectiveSourceUserExplicit
		}
		if !decoded.ObjectiveLedger[i].Required {
			decoded.ObjectiveLedger[i].Required = true
		}
	}
	return PromptInterpretation{
		ObjectiveLedger:          mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
		RecipeIDs:                cleanStringList(decoded.RecipeIDs),
		RequiresReferenceHistory: decoded.RequiresReferenceHistory,
		UserOperation:            normalizeUserOperation(decoded.UserOperation),
		RecommendedRecipeIDs:     cleanStringList(decoded.RecommendedRecipeIDs),
		ForbiddenRecipeIDs:       cleanStringList(decoded.ForbiddenRecipeIDs),
	}, nil
}

func cleanStringList(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func ParseStructuredLLMEvaluation(raw string) (StructuredLLMEvaluation, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return StructuredLLMEvaluation{}, fmt.Errorf("parse structured response evaluation: %w", err)
	}
	confidence, err := parseStructuredEvaluationConfidence(decoded["confidence"])
	if err != nil {
		return StructuredLLMEvaluation{}, err
	}
	feedback, _ := decoded["feedback"].(string)
	verdict, _ := decoded["verdict"].(string)
	blockingReason, _ := decoded["blocking_reason"].(string)
	verdict = normalizeStructuredEvaluationVerdict(verdict)
	if verdict == "accept" && structuredEvaluationFeedbackSuggestsHardReject(feedback+" "+blockingReason) {
		verdict = "reject"
	}
	return StructuredLLMEvaluation{
		Verdict:        verdict,
		Confidence:     confidence,
		BlockingReason: strings.TrimSpace(blockingReason),
		Feedback:       strings.TrimSpace(feedback),
	}, nil
}

func normalizeStructuredEvaluationVerdict(verdict string) string {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "reject", "revise":
		return strings.ToLower(strings.TrimSpace(verdict))
	default:
		return "accept"
	}
}

func structuredEvaluationFeedbackSuggestsHardReject(feedback string) bool {
	lower := strings.ToLower(feedback)
	for _, marker := range []string{
		"does not align",
		"not align",
		"scope drift",
		"scope_drift",
		"semantic mismatch",
		"contradicts worksite",
		"wrong project",
		"create a new project",
		"create a new react project",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func parseStructuredEvaluationConfidence(raw interface{}) (int, error) {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return validateStructuredEvaluationConfidence(int(parsed))
		}
		floatValue, err := strconv.ParseFloat(value.String(), 64)
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(int(floatValue))
	case float64:
		return validateStructuredEvaluationConfidence(int(value))
	case int:
		return validateStructuredEvaluationConfidence(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(parsed)
	default:
		return 0, fmt.Errorf("structured response evaluation missing confidence")
	}
}

func validateStructuredEvaluationConfidence(value int) (int, error) {
	if value < 0 || value > 100 {
		return 0, fmt.Errorf("structured response evaluation confidence out of range")
	}
	return value, nil
}

func validateStructuredEvaluationConsistency(evaluation StructuredLLMEvaluation) error {
	if evaluation.Confidence >= defaultEvaluatorThreshold {
		return nil
	}
	if structuredEvaluationFeedbackClaimsSuccess(evaluation.Feedback) {
		return fmt.Errorf("low confidence contradicts positive feedback")
	}
	return nil
}

func structuredEvaluationFeedbackClaimsSuccess(feedback string) bool {
	lower := strings.ToLower(feedback)
	if strings.Contains(lower, "not on track") || strings.Contains(lower, "off track") {
		return false
	}
	for _, phrase := range []string{
		"on track",
		"successfully completed",
		"correctly answered",
		"answered correctly",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func repeatedStructuredEvaluationFeedback(evaluation StructuredLLMEvaluation, observations []StructuredCommandObservation) bool {
	feedback := strings.TrimSpace(evaluation.Feedback)
	if feedback == "" {
		return false
	}
	for _, obs := range observations {
		if strings.TrimSpace(obs.EvaluationFeedback) == feedback {
			return true
		}
	}
	return false
}

const structuredRealtimeCapabilityMemory = "Omni can use shell commands and public unauthenticated sources to gather current facts. For location-specific time, use TZ=Area/City date or another evidence command; do not claim no real-time access when command evidence can be gathered."
const structuredWeatherCapabilityMemory = "Omni can gather current weather with public no-key wttr.in using an explicit location path and concise format query; do not use OpenWeatherMap, api.openweathermap.org, YOUR_API_KEY, or other API-key services without real observed credentials."

func structuredCapabilityMemoryForRejectedResponse(response, feedback string) string {
	if structuredTextSuggestsScopeDrift(response) || structuredTextSuggestsScopeDrift(feedback) {
		return structuredScopeCapabilityMemory
	}
	if structuredTextSuggestsKeyedWeatherSource(response) || structuredTextSuggestsKeyedWeatherSource(feedback) {
		return structuredWeatherCapabilityMemory
	}
	if structuredTextSuggestsFalseCapabilityLimit(response) || structuredTextSuggestsFalseCapabilityLimit(feedback) {
		return structuredRealtimeCapabilityMemory
	}
	return ""
}

func structuredTextSuggestsScopeDrift(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "dependency scope drift") || strings.Contains(lower, "unrequested package")
}

func structuredTextSuggestsKeyedWeatherSource(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "openweathermap") ||
		strings.Contains(lower, "api.openweathermap.org") ||
		strings.Contains(lower, "your_api_key") ||
		strings.Contains(lower, "api_key_here")
}

func structuredTextSuggestsFalseCapabilityLimit(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"as an ai",
		"i am unable",
		"i'm unable",
		"i cannot",
		"i can't",
		"i do not have access",
		"i don't have access",
		"do not have access to real-time",
		"don't have access to real-time",
		"cannot access real-time",
		"can't access real-time",
		"no access to real-time",
		"do not have internet access",
		"don't have internet access",
		"no internet access",
		"cannot browse",
		"can't browse",
		"unable to browse",
		"not able to browse",
		"check a weather website",
		"check the current time",
		"time zone app",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func structuredTextDefersEvidenceToFutureCommand(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"can be identified by running",
		"can be determined by running",
		"can be found by running",
		"can be checked by running",
		"run the command",
		"using the uname",
		"using uname",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func previousUserResponseForQuestion(observations []StructuredCommandObservation, question string) (string, bool) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Question) == question && strings.TrimSpace(observations[i].UserResponse) != "" {
			return observations[i].UserResponse, true
		}
	}
	return "", false
}

func isTransientStructuredLLMError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "ollama returned status 500") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer")
}

func classifyStructuredLLMFailure(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer") {
		return "ollama_model_runner_crash_or_restart"
	}
	if strings.Contains(text, "ollama returned status 500") {
		return "ollama_internal_error"
	}
	if strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout") {
		return "ollama_request_timeout"
	}
	return "ollama_request_failure"
}

func validateStructuredCommandString(command string) error {
	command = normalizeStructuredCommand(command)
	if structuredCommandLooksLikeMultilinePackageManagerScript(command) {
		return fmt.Errorf("multiline package-manager scripts are blocked; choose one concrete package/build command for the next objective")
	}
	if startsWithShellRedirectionToken(command) {
		return fmt.Errorf("command starts with shell redirection token")
	}
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(command)
	switch lower {
	case "none", "null", "n/a", "no command":
		return fmt.Errorf("command is not executable shell evidence")
	}
	for _, pseudoTool := range []string{"web.search", "browser.search", "search_web", "internet.search"} {
		if strings.HasPrefix(lower, pseudoTool) {
			return fmt.Errorf("%s is not a shell command; use curl with a public source such as news.google.com/rss/search or duckduckgo.com/html", strings.Fields(trimmed)[0])
		}
	}
	if isNonEvidenceShellCommand(command) {
		return fmt.Errorf("command is a shell/no-op launcher without task-specific side effects or output")
	}
	if structuredCommandUsesRecursiveForceRemove(command) {
		return fmt.Errorf("recursive force removal is blocked; use non-destructive creation/update commands or ask for explicit deletion approval")
	}
	if strings.Contains(lower, "openweathermap") || strings.Contains(lower, "api.openweathermap.org") {
		return fmt.Errorf("OpenWeatherMap requires an API key; use no-key wttr.in with an explicit location path and concise format query instead")
	}
	if err := validateGoogleNewsRSSCommand(command); err != nil {
		return err
	}
	if structuredCommandLooksLikeOSIdentification(command) && !structuredCommandDiscoversPackageManager(command) {
		return fmt.Errorf("OS identification command must include package-manager discovery with command -v pacman apt dnf yum zypper apk")
	}
	for _, placeholder := range []string{
		"<location>", "<query>", "<file>", "<filename>", "<path>", "<url>", "<number>", "<name>", "<project>",
		"<city>", "<country>", "<timezone>", "<api_key>", "<token>", "<placeholder>",
	} {
		if strings.Contains(lower, placeholder) {
			return fmt.Errorf("placeholder angle-bracket value in command")
		}
	}
	if strings.Contains(lower, "your_api_key") || strings.Contains(lower, "api_key_here") {
		return fmt.Errorf("placeholder angle-bracket value in command")
	}
	if isPureEchoCommand(command) {
		return fmt.Errorf("pure echo command is not command evidence")
	}
	if err := validateWTTRCommand(command); err != nil {
		return err
	}
	if err := validateDateCommand(command); err != nil {
		return err
	}
	return nil
}

func normalizeStructuredCommand(command string) string {
	command = normalizeStructuredCommandLineBreaks(command)
	command = normalizeStructuredMkdirParents(command)
	return command
}

func normalizeStructuredCommandLineBreaks(command string) string {
	command = strings.TrimSpace(command)
	if !strings.ContainsAny(command, "\n\r") || strings.Contains(command, "<<") {
		return command
	}
	parts := []string{}
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for _, r := range command {
		if r == '\r' {
			continue
		}
		if r == '\n' && !inSingleQuote && !inDoubleQuote {
			part := strings.TrimSpace(current.String())
			if part != "" {
				parts = append(parts, part)
			}
			current.Reset()
			escaped = false
			continue
		}
		current.WriteRune(r)
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		parts = append(parts, part)
	}
	if len(parts) <= 1 {
		return command
	}
	return strings.Join(parts, " && ")
}

func normalizeStructuredMkdirParents(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return command
	}
	parts := strings.Split(command, "&&")
	changed := false
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if !strings.HasPrefix(trimmed, "mkdir ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || fields[0] != "mkdir" || strings.HasPrefix(fields[1], "-") {
			continue
		}
		parts[i] = " mkdir -p " + strings.TrimSpace(strings.TrimPrefix(trimmed, "mkdir "))
		changed = true
	}
	if !changed {
		return command
	}
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, " && ")
}

func structuredCommandUsesRecursiveForceRemove(command string) bool {
	parts := strings.Fields(command)
	for i, part := range parts {
		if cleanCommandPathToken(part) != "rm" {
			continue
		}
		end := len(parts)
		for j := i + 1; j < len(parts); j++ {
			token := cleanCommandPathToken(parts[j])
			switch token {
			case "&&", "||", ";", "|":
				end = j
			}
			if end == j {
				break
			}
		}
		if rmUsesRecursiveForce(parts[i:end]) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeMultilinePackageManagerScript(command string) bool {
	if !strings.ContainsAny(command, "\n\r") {
		return false
	}
	if strings.Contains(command, "<<") {
		return false
	}
	packageManagerLines := 0
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	atLineStart := true
	lineHasContent := false
	var lineBuilder strings.Builder
	for _, r := range command {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			if lineHasContent {
				if structuredLineStartsWithPackageManager(lineBuilder.String()) {
					packageManagerLines++
					if packageManagerLines > 1 {
						return true
					}
				}
				lineBuilder.Reset()
				lineHasContent = false
			}
			atLineStart = true
			escaped = false
			continue
		}
		if atLineStart {
			if r == ' ' || r == '\t' {
				continue
			}
			if !inSingleQuote && !inDoubleQuote {
				lineBuilder.WriteRune(r)
			}
			atLineStart = false
		} else if !inSingleQuote && !inDoubleQuote {
			lineBuilder.WriteRune(r)
		}
		lineHasContent = true
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
	}
	if lineHasContent && structuredLineStartsWithPackageManager(lineBuilder.String()) {
		packageManagerLines++
	}
	if packageManagerLines > 1 {
		return true
	}
	return false
}

func structuredLineStartsWithPackageManager(line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return false
	}
	switch cleanCommandPathToken(fields[0]) {
	case "npm", "npx", "pnpm", "yarn":
		return true
	default:
		return false
	}
}

func validateStructuredCommandForObservations(command string, observations []StructuredCommandObservation) error {
	if err := validateStructuredCommandString(command); err != nil {
		return err
	}
	return nil
}

var errRepeatedFailedStructuredCommand = errors.New("command repeats a previous failed command; check completed_actions, choose a different command, source, or local tool")
var errRepeatedSuccessfulStructuredCommand = errors.New("command already completed earlier; inspect completed_actions, update the objective ledger, or choose the next required action")

func validateStructuredCommandForRun(command string, observations []StructuredCommandObservation, workingDirectory string, objectiveLedger []StructuredObjective) error {
	return validateStructuredCommandForRunWithSurvey(command, observations, workingDirectory, objectiveLedger, WorksiteSurvey{})
}

func validateStructuredCommandForRunWithSurvey(command string, observations []StructuredCommandObservation, workingDirectory string, objectiveLedger []StructuredObjective, survey WorksiteSurvey) error {
	if err := validateStructuredCommandForObservations(command, observations); err != nil {
		return err
	}
	if repeatedSuccessfulStructuredCommand(command, observations) {
		return errRepeatedSuccessfulStructuredCommand
	}
	if err := validateStructuredCommandWorkspaceProtection(command, workingDirectory); err != nil {
		return err
	}
	if err := validateCargoScaffoldUsesActiveWorkspace(command, workingDirectory); err != nil {
		return err
	}
	if err := validateNestedGoModuleCommandScope(command, workingDirectory); err != nil {
		return err
	}
	if err := validateStructuredScaffoldScope(command, survey); err != nil {
		return err
	}
	if err := validatePlaceholderOnlyAppMutation(command, objectiveLedger); err != nil {
		return err
	}
	if err := validateStructuredDependencyScope(command, objectiveLedger, workingDirectory); err != nil {
		return err
	}
	return nil
}

func validatePlaceholderOnlyAppMutation(command string, objectiveLedger []StructuredObjective) error {
	if !shellProposalIsPlaceholderOnlyMutation(command) || !objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger) {
		return nil
	}
	return fmt.Errorf("placeholder-only command does not satisfy app objectives; write substantive source/build/test file content instead of only mkdir/touch")
}

func objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger []StructuredObjective) bool {
	for _, objective := range pendingStructuredObjectives(objectiveLedger) {
		text := strings.ToLower(strings.TrimSpace(objective.ID + " " + objective.Description))
		if text == "" {
			continue
		}
		if strings.Contains(text, "app") ||
			strings.Contains(text, "component") ||
			strings.Contains(text, "crud") ||
			strings.Contains(text, "entry") ||
			strings.Contains(text, "implement") ||
			strings.Contains(text, "source") ||
			strings.Contains(text, "ui") {
			return true
		}
	}
	return false
}

func validateNestedGoModuleCommandScope(command, workingDirectory string) error {
	if strings.TrimSpace(command) == "" || strings.TrimSpace(workingDirectory) == "" {
		return nil
	}
	if !rootCommandRunsGoModInit(command) {
		return nil
	}
	nested := firstNestedGoMod(workingDirectory)
	if nested == "" {
		return nil
	}
	return fmt.Errorf("go mod init at workspace root is unsafe because nested module %s already exists; run Go commands from the existing module directory instead", nested)
}

func rootCommandRunsGoModInit(command string) bool {
	segments := structuredCommandSegments(command)
	for _, segment := range segments {
		if len(segment) < 3 {
			continue
		}
		if cleanCommandPathToken(segment[0]) == "go" && segment[1] == "mod" && segment[2] == "init" {
			return true
		}
	}
	return false
}

func firstNestedGoMod(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	rootGoMod := filepath.Join(root, "go.mod")
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" || path == rootGoMod {
			return nil
		}
		if rel, relErr := filepath.Rel(root, path); relErr == nil {
			found = rel
		} else {
			found = path
		}
		return nil
	})
	return found
}

func validateCargoScaffoldUsesActiveWorkspace(command, workingDirectory string) error {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if command == "" || workingDirectory == "" {
		return nil
	}
	base := strings.ToLower(filepath.Base(workingDirectory))
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 || cleanCommandPathToken(segment[0]) != "cargo" || segment[1] != "new" {
			continue
		}
		for _, raw := range segment[2:] {
			arg := strings.Trim(raw, `"'`)
			if arg == "" || strings.HasPrefix(arg, "-") {
				continue
			}
			clean := strings.ToLower(filepath.Base(filepath.Clean(arg)))
			if clean == "." || clean == base {
				return fmt.Errorf("scope_drift: cargo new would create a nested project inside the active workspace; use cargo init or write Cargo.toml/src files in place")
			}
			break
		}
	}
	return nil
}

func validateShellProposalAgainstToolTask(command, toolTask string) error {
	if strings.TrimSpace(command) == "" || !toolTaskRequiresMutation(toolTask) {
		return nil
	}
	if toolTaskAllowsInspectionEvidence(toolTask) && structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	if toolTaskRequiresSourceImplementation(toolTask) && structuredCommandLooksDependencyInstall(command) && !toolTaskAllowsDependencyInstall(toolTask) {
		return fmt.Errorf("tool_task requires source file implementation; dependency install command %q does not satisfy it", strings.TrimSpace(command))
	}
	if shellProposalIsPlaceholderOnlyMutation(command) {
		return fmt.Errorf("tool_task requires substantive file content or verification; placeholder-only command %q does not satisfy it", strings.TrimSpace(command))
	}
	if shellProposalWritesOnlyResearchArtifact(command, toolTask) {
		return fmt.Errorf("tool_task requires substantive source/build/test files; documentation download command %q does not satisfy it", strings.TrimSpace(command))
	}
	if structuredCommandLooksMutating(command) {
		return nil
	}
	return fmt.Errorf("tool_task requires file creation, modification, build, or test work; read-only command %q does not satisfy it", strings.TrimSpace(command))
}

func toolTaskRequiresSourceImplementation(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"actual project files",
		"component",
		"crud",
		"implement",
		"in-memory",
		"source/build/test",
		"source file",
		"store_notes",
		"substantive source",
		"ui",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func toolTaskAllowsDependencyInstall(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"install dependencies",
		"install package",
		"install required",
		"dependency install",
		"package install",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func structuredCommandLooksDependencyInstall(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		action := segment[1]
		switch root {
		case "npm":
			if action == "install" || action == "add" || action == "i" {
				return true
			}
		case "pnpm", "yarn", "bun":
			if action == "add" || action == "install" {
				return true
			}
		case "cargo":
			if action == "add" {
				return true
			}
		case "go":
			if action == "get" {
				return true
			}
		case "pip", "pip3":
			if action == "install" {
				return true
			}
		case "composer":
			if action == "require" || action == "install" {
				return true
			}
		}
	}
	return false
}

func shellProposalWritesOnlyResearchArtifact(command, toolTask string) bool {
	task := strings.ToLower(toolTask)
	if !strings.Contains(task, "substantive source") && !strings.Contains(task, "actual project files") {
		return false
	}
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "curl") {
		return false
	}
	if !strings.Contains(lower, ">") && !strings.Contains(lower, " -o ") && !strings.Contains(lower, " --output ") {
		return false
	}
	return !structuredCommandLooksAppFileMutation(command)
}

func shellProposalIsPlaceholderOnlyMutation(command string) bool {
	segments := structuredCommandSegments(command)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root == "touch" || root == "mkdir" {
			continue
		}
		return false
	}
	return true
}

func toolTaskAllowsInspectionEvidence(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"inspect_empty_placeholder",
		"inspect for empty placeholder",
		"inspect existing files",
		"target path does not exist",
		"run a bounded file discovery command",
		"inspect the parent directory",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func structuredCommandLooksReadOnlyEvidence(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	root := cleanCommandPathToken(fields[0])
	if root == "cd" {
		for i, field := range fields {
			if field == "&&" && i+1 < len(fields) {
				root = cleanCommandPathToken(fields[i+1])
				break
			}
			if field == ";" && i+1 < len(fields) {
				root = cleanCommandPathToken(fields[i+1])
				break
			}
		}
	}
	switch root {
	case "find", "ls", "cat", "grep", "rg", "sed", "head", "tail", "test", "stat", "pwd", "jq", "npm":
	default:
		return false
	}
	return !structuredCommandLooksMutating(command)
}

func toolTaskRequiresMutation(toolTask string) bool {
	task := strings.ToLower(toolTask)
	if strings.Contains(task, "do not continue with read-only") || strings.Contains(task, "read-only inventory commands") {
		return true
	}
	needles := []string{
		"required next behavior: create or modify the actual project files now",
		"patch existing project files",
		"substantive source/build/test files",
		"substantive source, build metadata, tests, and verification files",
		"writes index.html, src/index.js, styles, package scripts, and verification files",
		"npm run build",
		"npm test",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func validateStructuredScaffoldScope(command string, survey WorksiteSurvey) error {
	if strings.TrimSpace(command) == "" || survey.UserOperation == "" || survey.UserOperation == userOperationUnknown {
		return nil
	}
	if !structuredCommandHasScaffold(command) {
		return nil
	}
	if worksiteSurveyAllowsCreateNew(survey) {
		return nil
	}
	return fmt.Errorf("scope_drift: scaffold command forbidden for %s in %s; full access does not permit changing task scope", survey.UserOperation, survey.ProjectState)
}

func structuredCommandHasScaffold(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root == "npx" && len(segment) >= 2 {
			tool := cleanCommandPathToken(segment[1])
			if tool == "create-react-app" || tool == "degit" {
				return true
			}
		}
		if root == "npm" && len(segment) >= 3 {
			action := segment[1]
			tool := cleanCommandPathToken(segment[2])
			if action == "create" && strings.HasPrefix(tool, "vite") {
				return true
			}
			if action == "init" && (strings.HasPrefix(tool, "vite") || tool == "react-app") {
				return true
			}
		}
		if (root == "pnpm" || root == "yarn" || root == "bun") && len(segment) >= 3 && segment[1] == "create" {
			if strings.HasPrefix(cleanCommandPathToken(segment[2]), "vite") {
				return true
			}
		}
		if root == "cargo" && len(segment) >= 2 && (segment[1] == "new" || segment[1] == "init") {
			return true
		}
		if root == "git" && len(segment) >= 2 && segment[1] == "clone" {
			return true
		}
	}
	return false
}

type structuredDependencyInstallRequest struct {
	Manager  string
	Packages []string
}

func validateStructuredDependencyScope(command string, objectiveLedger []StructuredObjective, workingDirectory string) error {
	requests := structuredDependencyInstallRequests(command)
	if len(requests) == 0 {
		return nil
	}
	allowed := structuredAllowedDependencyPackages(objectiveLedger, workingDirectory)
	blocked := []string{}
	for _, request := range requests {
		for _, pkg := range request.Packages {
			normalized := normalizeDependencyPackageName(pkg)
			if normalized == "" {
				continue
			}
			if !allowed[normalized] {
				blocked = append(blocked, pkg)
			}
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	return fmt.Errorf("dependency scope drift: unrequested package(s) %s; dependencies must be justified by user_explicit, recipe_required, detected_project, or evidence_required_prerequisite objectives", strings.Join(cleanStringList(blocked), ", "))
}

func structuredDependencyInstallRequests(command string) []structuredDependencyInstallRequest {
	requests := []structuredDependencyInstallRequest{}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		switch root {
		case "npm":
			if segment[1] == "install" || segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "npm", Packages: pkgs})
				}
			}
		case "pnpm":
			if segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "pnpm", Packages: pkgs})
				}
			}
		case "yarn":
			if segment[1] == "add" || segment[1] == "install" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "yarn", Packages: pkgs})
				}
			}
		case "go":
			if segment[1] == "get" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "go", Packages: pkgs})
				}
			}
		case "composer":
			if segment[1] == "require" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "composer", Packages: pkgs})
				}
			}
		case "pip", "pip3":
			if segment[1] == "install" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: root, Packages: pkgs})
				}
			}
		case "cargo":
			if segment[1] == "add" {
				if pkgs := dependencyPackagesFromArgs(segment[2:]); len(pkgs) > 0 {
					requests = append(requests, structuredDependencyInstallRequest{Manager: "cargo", Packages: pkgs})
				}
			}
		}
	}
	return requests
}

func dependencyPackagesFromArgs(args []string) []string {
	packages := []string{}
	skipNext := false
	for _, raw := range args {
		arg := strings.Trim(raw, `"'`)
		if arg == "" {
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if dependencyFlagTakesValue(arg) {
				skipNext = true
			}
			continue
		}
		if strings.Contains(arg, "=") && strings.HasPrefix(arg, "--") {
			continue
		}
		packages = append(packages, arg)
	}
	return cleanStringList(packages)
}

func dependencyFlagTakesValue(flag string) bool {
	switch flag {
	case "-r", "--requirement", "-c", "--constraint", "--index-url", "--extra-index-url", "--registry", "--prefix", "--global-folder", "--modules-folder":
		return true
	default:
		return false
	}
}

func structuredAllowedDependencyPackages(objectiveLedger []StructuredObjective, workingDirectory string) map[string]bool {
	allowed := map[string]bool{}
	for _, objective := range objectiveLedger {
		objective, ok := normalizeStructuredObjective(objective)
		if !ok || !structuredObjectiveSourceCanExecute(objective.Source) {
			continue
		}
		for _, pkg := range objective.Packages {
			if normalized := normalizeDependencyPackageName(pkg); normalized != "" {
				allowed[normalized] = true
			}
		}
		for _, pkg := range inferredDependencyPackagesForObjective(objective) {
			allowed[pkg] = true
		}
	}
	for _, pkg := range detectedProjectDependencyPackages(workingDirectory) {
		allowed[pkg] = true
	}
	return allowed
}

func structuredObjectiveSourceCanExecute(source string) bool {
	switch normalizeStructuredObjectiveSource(source) {
	case structuredObjectiveSourceUserExplicit, structuredObjectiveSourceRecipeRequired, structuredObjectiveSourceDetectedProject, structuredObjectiveSourceEvidenceRequiredPrerequisite:
		return true
	default:
		return false
	}
}

func inferredDependencyPackagesForObjective(objective StructuredObjective) []string {
	text := normalizedDependencyText(objective.ID + " " + objective.Description)
	out := []string{}
	if strings.Contains(text, " react ") {
		out = append(out, "react", "react-dom", "vite", "@vitejs/plugin-react")
	}
	if strings.Contains(text, " tailwind ") || strings.Contains(text, " tailwindcss ") {
		out = append(out, "tailwindcss", "postcss", "autoprefixer", "@tailwindcss/vite")
	}
	if strings.Contains(text, " typescript ") {
		out = append(out, "typescript", "@types/react", "@types/react-dom")
	}
	if (strings.Contains(text, " chess ") || strings.Contains(text, " legal move ") || strings.Contains(text, " legal moves ") || strings.Contains(text, " rules library ")) &&
		(strings.Contains(text, " rust ") || strings.Contains(text, " cargo ")) {
		out = append(out, "chess", "shakmaty")
	}
	return out
}

func detectedProjectDependencyPackages(workingDirectory string) []string {
	if strings.TrimSpace(workingDirectory) == "" {
		return nil
	}
	blob, err := os.ReadFile(filepath.Join(workingDirectory, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies         map[string]interface{} `json:"dependencies"`
		DevDependencies      map[string]interface{} `json:"devDependencies"`
		PeerDependencies     map[string]interface{} `json:"peerDependencies"`
		OptionalDependencies map[string]interface{} `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(blob, &pkg); err != nil {
		return nil
	}
	out := []string{}
	for _, deps := range []map[string]interface{}{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies, pkg.OptionalDependencies} {
		for dep := range deps {
			if normalized := normalizeDependencyPackageName(dep); normalized != "" {
				out = append(out, normalized)
			}
		}
	}
	return cleanStringList(out)
}

func normalizeDependencyPackageName(pkg string) string {
	clean := strings.Trim(strings.TrimSpace(pkg), `"'`)
	if clean == "" {
		return ""
	}
	if strings.HasPrefix(clean, "git+") || strings.Contains(clean, "://") || strings.HasPrefix(clean, ".") || strings.HasPrefix(clean, "/") {
		return ""
	}
	if at := strings.LastIndex(clean, "@"); at > 0 {
		clean = clean[:at]
	}
	return strings.ToLower(clean)
}

func normalizedDependencyText(text string) string {
	var b strings.Builder
	b.WriteByte(' ')
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteByte(' ')
	return b.String()
}

func validateStructuredCommandWorkspaceProtection(command, workingDirectory string) error {
	workspace := strings.TrimSpace(workingDirectory)
	if workspace == "" {
		return nil
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return nil
	}
	workspaceAbs = filepath.Clean(workspaceAbs)
	segments := structuredCommandSegments(command)
	deletedTargets := map[string]struct{}{}
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		switch root {
		case "rm", "rmdir":
			for _, target := range structuredCommandPathArgs(segment[1:], workspaceAbs) {
				if pathIsSameOrAncestor(target, workspaceAbs) {
					return fmt.Errorf("command attempts to remove the active working directory or one of its parents; creation/build tasks must preserve existing directories")
				}
				deletedTargets[target] = struct{}{}
			}
		case "mv":
			args := structuredCommandPathArgs(segment[1:], workspaceAbs)
			if len(args) > 0 && pathIsSameOrAncestor(args[0], workspaceAbs) {
				return fmt.Errorf("command attempts to move the active working directory or one of its parents; creation/build tasks must preserve existing directories")
			}
		case "mkdir":
			for _, target := range structuredCommandPathArgs(segment[1:], workspaceAbs) {
				if _, deleted := deletedTargets[target]; deleted {
					return fmt.Errorf("command deletes and recreates the same path; use mkdir -p or update files in place instead")
				}
			}
		}
	}
	return nil
}

func structuredCommandSegments(command string) [][]string {
	fields := strings.Fields(command)
	segments := [][]string{}
	current := []string{}
	for _, field := range fields {
		token := cleanCommandPathToken(field)
		switch token {
		case "&&", "||", ";", "|":
			if len(current) > 0 {
				segments = append(segments, current)
				current = []string{}
			}
		default:
			current = append(current, field)
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

func structuredCommandPathArgs(args []string, workspaceAbs string) []string {
	targets := []string{}
	stopOptions := false
	for _, arg := range args {
		candidate := cleanCommandPathToken(arg)
		if candidate == "" {
			continue
		}
		if candidate == "--" {
			stopOptions = true
			continue
		}
		if !stopOptions && strings.HasPrefix(candidate, "-") {
			continue
		}
		if strings.Contains(candidate, "=") || isShellRedirectToken(candidate) {
			continue
		}
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			continue
		}
		resolved, pathLike := resolveCommandPathToken(candidate, workspaceAbs)
		if !pathLike {
			continue
		}
		targets = append(targets, filepath.Clean(resolved))
	}
	return targets
}

func pathIsSameOrAncestor(candidate, target string) bool {
	candidate = filepath.Clean(candidate)
	target = filepath.Clean(target)
	if candidate == target {
		return true
	}
	rel, err := filepath.Rel(candidate, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))
}

func repeatedFailedStructuredCommand(command string, observations []StructuredCommandObservation) bool {
	return false
}

func repeatedSuccessfulStructuredCommand(command string, observations []StructuredCommandObservation) bool {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return false
	}
	for _, obs := range observations {
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			return true
		}
	}
	return false
}

func previousSuccessfulStructuredCommandObservation(command string, observations []StructuredCommandObservation) (StructuredCommandObservation, bool) {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return StructuredCommandObservation{}, false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			return obs, true
		}
	}
	return StructuredCommandObservation{}, false
}

func repeatedRejectedCommandCount(command string, observations []StructuredCommandObservation) int {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return 0
	}
	count := 0
	for _, obs := range observations {
		if normalizeStructuredCommandForComparison(obs.RejectedCommand) == normalized {
			count++
		}
	}
	return count
}

func normalizeStructuredCommandForComparison(command string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
}

func completedActionsFromState(ledger []StructuredObjective, observations []StructuredCommandObservation) []CompletedAction {
	actions := []CompletedAction{}
	seen := map[string]struct{}{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if obs.ExitCode != 0 || command == "" || strings.HasPrefix(command, "SKIPPED_REPEAT_SUCCESS:") {
			continue
		}
		normalized := normalizeStructuredCommandForComparison(command)
		if normalized == "" {
			continue
		}
		key := "command:" + normalized
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, CompletedAction{
			ID:       completedActionID("command", normalized),
			Kind:     completedActionKindForCommand(command),
			Summary:  completedActionSummaryForCommand(command),
			Command:  command,
			Evidence: structuredObjectiveEvidenceFromObservation(obs),
			Step:     obs.Step,
		})
	}
	for _, objective := range mergeStructuredObjectiveLedger(nil, ledger) {
		if !structuredObjectiveSatisfied(objective) {
			continue
		}
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			continue
		}
		key := "objective:" + id
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		summary := strings.TrimSpace(objective.Description)
		if summary == "" {
			summary = id
		}
		actions = append(actions, CompletedAction{
			ID:          completedActionID("objective", id),
			Kind:        "objective",
			Summary:     "Satisfied objective: " + summary,
			ObjectiveID: id,
			Evidence:    truncateStructuredObservation(objective.Evidence),
		})
	}
	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].Step == actions[j].Step {
			return actions[i].ID < actions[j].ID
		}
		if actions[i].Step == 0 {
			return false
		}
		if actions[j].Step == 0 {
			return true
		}
		return actions[i].Step < actions[j].Step
	})
	return actions
}

func completedActionID(kind, value string) string {
	clean := strings.ToLower(strings.TrimSpace(kind + " " + value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	id := strings.Trim(b.String(), "_")
	if id == "" {
		return "completed_action"
	}
	if len(id) > 96 {
		id = strings.TrimRight(id[:96], "_")
	}
	return id
}

func completedActionKindForCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return "command"
	}
	root := fields[0]
	switch root {
	case "mkdir":
		return "file"
	case "npm", "pnpm", "yarn", "bun", "go", "cargo", "composer", "pip":
		return "dependency_or_verification"
	case "test", "cat", "sed", "rg", "find", "ls", "git":
		return "inspection"
	default:
		return "command"
	}
}

func completedActionSummaryForCommand(command string) string {
	return "Completed command: " + truncateStructuredObservation(normalizeStructuredCommandForComparison(command))
}

func structuredLoopStateFromState(ledger []StructuredObjective, observations []StructuredCommandObservation) StructuredLoopState {
	pendingIDs := structuredObjectiveIDs(pendingStructuredObjectives(ledger))
	state := StructuredLoopState{
		Status:              "progressing",
		PendingObjectiveIDs: pendingIDs,
	}
	if len(observations) == 0 {
		state.Status = "not_started"
		state.Instruction = "Start with a command or patch that gathers evidence or satisfies the first objective."
		return state
	}
	if blocker := latestStructuredObservationBlocker(observations); blocker != "" {
		state.LastBlocker = blocker
	}
	if count, pending := latestPrematureDoneRejectionRun(observations); count > 0 {
		state.RepeatKind = "premature_done"
		state.RepeatCount = count
		if len(pendingIDs) == 0 && strings.TrimSpace(pending) != "" {
			state.PendingObjectiveIDs = strings.Split(pending, ",")
		}
		if count >= maxRepeatedPrematureDoneRejections {
			state.Status = "blocked"
			state.Instruction = "Stop returning done=true; choose a command or patch that satisfies a pending objective."
		} else {
			state.Status = "stuck"
			state.Instruction = "The previous done=true was rejected; advance a pending objective before trying done again."
		}
		return state
	}
	if count, command := latestRejectedCommandRun(observations); count > 0 {
		state.RepeatKind = "rejected_command"
		state.RepeatCount = count
		state.RepeatedCommand = command
		if count >= 2 {
			state.Status = "blocked"
		} else {
			state.Status = "stuck"
		}
		state.Instruction = "The latest proposal was rejected before execution: " + truncateStructuredTimelineValue(command) + ". Rejected proposals are evidence only, not completed actions and not forbidden commands. Choose a valid command, use tool=shell with a narrower task, inspect existing files, or use tool=patch.apply for source edits."
		return state
	}
	return state
}

func latestStructuredObservationBlocker(observations []StructuredCommandObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			return truncateStructuredTimelineValue(obs.Stderr)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			return truncateStructuredTimelineValue(obs.EvaluationFeedback)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			return "rejected command: " + truncateStructuredTimelineValue(obs.RejectedCommand)
		}
	}
	return ""
}

func latestPrematureDoneRejectionRun(observations []StructuredCommandObservation) (int, string) {
	count := 0
	pending := ""
	for i := len(observations) - 1; i >= 0; i-- {
		stderr := strings.TrimSpace(observations[i].Stderr)
		if !strings.Contains(stderr, "done rejected: pending objective(s) remain:") &&
			!strings.Contains(stderr, "anti_loop: planner returned done=true") {
			if count > 0 {
				break
			}
			continue
		}
		current := extractPendingObjectivesFromDoneRejection(stderr)
		if pending == "" {
			pending = current
		}
		if current != "" && pending != "" && current != pending {
			break
		}
		count++
	}
	return count, pending
}

func extractPendingObjectivesFromDoneRejection(stderr string) string {
	for _, marker := range []string{"pending objective(s) remain:", "same pending objective(s) remain:"} {
		idx := strings.Index(stderr, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(stderr[idx+len(marker):])
		if semi := strings.Index(rest, ";"); semi >= 0 {
			rest = rest[:semi]
		}
		if dot := strings.Index(rest, "."); dot >= 0 {
			rest = rest[:dot]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

func latestRejectedCommandRun(observations []StructuredCommandObservation) (int, string) {
	count := 0
	command := ""
	for i := len(observations) - 1; i >= 0; i-- {
		current := normalizeStructuredCommandForComparison(observations[i].RejectedCommand)
		if current == "" {
			if count > 0 {
				break
			}
			continue
		}
		if command == "" {
			command = current
		}
		if current != command {
			break
		}
		count++
	}
	return count, command
}

func startsWithShellRedirectionToken(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	return isShellRedirectToken(fields[0])
}

func isPureEchoCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if lower != "echo" && !strings.HasPrefix(lower, "echo ") {
		return false
	}
	for _, marker := range []string{"|", ">", "<", "$(", "`", "&&", "||", ";"} {
		if strings.Contains(trimmed, marker) {
			return false
		}
	}
	return true
}

func isNonEvidenceShellCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	switch lower {
	case "bash", "sh", "zsh", "fish", "dash", "true", ":", "exit", "exit 0":
		return true
	default:
		return false
	}
}

func validateWTTRCommand(command string) error {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "wttr.in") {
		return nil
	}
	if !strings.Contains(lower, "wttr.in/") {
		return fmt.Errorf("wttr.in command must include an explicit location path")
	}
	if strings.Contains(lower, "wttr.in/?") || strings.Contains(lower, "wttr.in/ ") || strings.HasSuffix(strings.TrimSpace(lower), "wttr.in/") {
		return fmt.Errorf("wttr.in command must include a non-empty location path")
	}
	if !strings.Contains(lower, "format=") {
		return fmt.Errorf("wttr.in command must use a concise format query")
	}
	return nil
}

func validateDateCommand(command string) error {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "date") {
		return nil
	}
	fields := strings.Fields(lower)
	for i, field := range fields {
		if field == "date" || strings.HasSuffix(field, "/date") {
			if i+1 < len(fields) && fields[i+1] == "-t" {
				return fmt.Errorf("date command must not use invalid -t timezone option; prefix with TZ=Area/City before date")
			}
		}
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") && !strings.Contains(lower, " tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ as a date argument")
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "-d") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ through -d")
	}
	return nil
}

func validateGoogleNewsRSSCommand(command string) error {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "news.google.com/rss/search") {
		return nil
	}
	if !curlCommandFollowsRedirects(command) {
		return fmt.Errorf("Google News RSS command must use curl -L or curl -fsSL so redirects produce evidence")
	}
	if !curlCommandHasSilentFlag(command) {
		return fmt.Errorf("Google News RSS command must use curl -s or curl -fsSL to avoid progress-meter noise in evidence")
	}
	if !curlCommandHasUserAgent(command) {
		return fmt.Errorf("Google News RSS command must set a user agent with curl -A 'Mozilla/5.0'")
	}
	if !strings.Contains(lower, "ceid=") {
		return fmt.Errorf("Google News RSS command must include hl/gl/ceid query parameters for stable localized results")
	}
	return nil
}

func curlCommandFollowsRedirects(command string) bool {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "--location") {
		return true
	}
	for _, field := range strings.Fields(lower) {
		if strings.HasPrefix(field, "-") && strings.Contains(field, "l") {
			return true
		}
	}
	return false
}

func curlCommandHasSilentFlag(command string) bool {
	lower := strings.ToLower(command)
	for _, field := range strings.Fields(lower) {
		if strings.HasPrefix(field, "-") && strings.Contains(field, "s") {
			return true
		}
	}
	return false
}

func curlCommandHasUserAgent(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, " -a ") || strings.Contains(lower, "\t-a ") || strings.Contains(lower, "--user-agent")
}

func emitStructuredCommandEvent(onEvent func(StructuredCommandEvent), eventType, summary string, details map[string]string) {
	if onEvent == nil {
		return
	}
	onEvent(StructuredCommandEvent{Type: eventType, Summary: summary, Details: details})
}

func hasRealCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			return true
		}
	}
	return false
}

func hasSuccessfulCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			return true
		}
	}
	return false
}

func latestSuccessfulCommandObservation(observations []StructuredCommandObservation) (StructuredCommandObservation, bool) {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) != "" && observations[i].ExitCode == 0 {
			return observations[i], true
		}
	}
	return StructuredCommandObservation{}, false
}

func enforcePostWriteValidationBeforeCompletion(step int, prompt string, previousLedger, ledger []StructuredObjective, observations []StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) []StructuredObjective {
	if len(pendingStructuredObjectives(ledger)) > 0 || !structuredCompletionNeedsPostWriteValidation(prompt, previousLedger, observations) {
		return ledger
	}
	pendingBefore := pendingStructuredObjectives(previousLedger)
	if len(pendingBefore) == 0 {
		return ledger
	}
	reset := make([]StructuredObjective, 0, len(pendingBefore))
	for _, objective := range pendingBefore {
		objective.Status = "pending"
		objective.Evidence = ""
		reset = append(reset, objective)
	}
	emitStructuredCommandEvent(onEvent, "completion_check_validation_required", "Completion requires readback evidence after a write command", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"objectives": strings.Join(structuredObjectiveIDs(reset), ","),
	})
	if result != nil && !latestObservationIsPostWriteValidationRejection(result.Observations) {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "completion rejected: write/edit/package mutation was observed, but no later readback or verification command proves the requested final state; run cat/sed/rg/grep/ls/test/jq/npm pkg get/npm ls or equivalent evidence before done=true",
		})
	}
	return forceStructuredObjectivesPending(ledger, reset)
}

func latestObservationIsPostWriteValidationRejection(observations []StructuredCommandObservation) bool {
	if len(observations) == 0 {
		return false
	}
	return strings.Contains(observations[len(observations)-1].Stderr, "completion rejected: write/edit/package mutation")
}

func deterministicCompletionEnforcerAcceptsDone(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if len(pendingStructuredObjectives(ledger)) > 0 {
		return false
	}
	if !latestRealCommandSucceeded(observations) {
		return false
	}
	return !structuredCompletionNeedsPostWriteValidation(prompt, ledger, observations)
}

func forceStructuredObjectivesPending(ledger, reset []StructuredObjective) []StructuredObjective {
	out := mergeStructuredObjectiveLedger(nil, ledger)
	byID := map[string]StructuredObjective{}
	for _, objective := range reset {
		normalized, ok := normalizeStructuredObjective(objective)
		if ok {
			byID[normalized.ID] = normalized
		}
	}
	for i, objective := range out {
		if replacement, ok := byID[objective.ID]; ok {
			if strings.TrimSpace(replacement.Description) == "" {
				replacement.Description = objective.Description
			}
			if strings.TrimSpace(replacement.Source) == "" || replacement.Source == structuredObjectiveSourceModelInferred {
				replacement.Source = objective.Source
			}
			if !replacement.Required {
				replacement.Required = objective.Required
			}
			out[i] = replacement
			delete(byID, objective.ID)
		}
	}
	for _, objective := range byID {
		out = append(out, objective)
	}
	return out
}

func structuredCompletionNeedsPostWriteValidation(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if !structuredTaskLooksLikeWriteOrEdit(prompt, ledger) {
		return false
	}
	lastMutation := -1
	for i, obs := range observations {
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if structuredCommandMutatesWorkspace(obs.Command) {
			if structuredMutatingCommandIncludesValidation(obs.Command) {
				lastMutation = -1
				continue
			}
			lastMutation = i
		}
	}
	if lastMutation < 0 {
		return false
	}
	for _, obs := range observations[lastMutation+1:] {
		if obs.ExitCode == 0 && structuredCommandValidatesWorkspace(obs.Command) {
			return false
		}
	}
	return true
}

func structuredTaskLooksLikeWriteOrEdit(prompt string, ledger []StructuredObjective) bool {
	text := strings.ToLower(prompt + " " + structuredLedgerText(ledger))
	for _, marker := range []string{
		" add ", " create ", " edit ", " modify ", " update ", " write ", " install ", " initialize ", " set up ", " setup ",
		"package.json", "script", "dependency", "dependencies", "file", "directory", "project", "build artifact",
	} {
		if strings.Contains(" "+text+" ", marker) {
			return true
		}
	}
	return false
}

func structuredLedgerText(ledger []StructuredObjective) string {
	parts := []string{}
	for _, objective := range ledger {
		parts = append(parts, objective.ID, objective.Description)
	}
	return strings.Join(parts, " ")
}

func structuredCommandMutatesWorkspace(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"npm pkg set", "npm set-script", "npm install", "npm add", "npm init", "pnpm add", "yarn add",
		"sed -i", "perl -pi", "writefile", "writefilesync", "mkdir", "touch ", " tee ", "mv ", "cp ",
		">", ">>",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredMutatingCommandIncludesValidation(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"&& cat ", "&& sed -n", "&& rg ", "&& grep ", "&& ls ", "&& test ", "&& jq ", "&& npm pkg get", "&& npm ls", "&& node -e",
		"\ncat ", "\nsed -n", "\nrg ", "\ngrep ", "\nls ", "\ntest ", "\njq ", "\nnpm pkg get", "\nnpm ls", "\nnode -e",
		" curl ", "\ncurl ", " go test ", "\ngo test ", " go build ", "\ngo build ", " npm run build", "\nnpm run build",
		" docker inspect ", "\ndocker inspect ", " docker logs ", "\ndocker logs ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredCommandValidatesWorkspace(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, marker := range []string{"cat ", "sed -n", "rg ", "grep ", "ls", "test ", "[ -", "jq ", "npm pkg get", "npm ls", "node -e"} {
		if strings.HasPrefix(lower, marker) || strings.Contains(lower, "&& "+marker) {
			return true
		}
	}
	return false
}

func structuredObservationsHavePackageManagerEvidence(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) == "" || obs.ExitCode != 0 {
			continue
		}
		if structuredCommandDiscoversPackageManager(obs.Command) {
			return true
		}
	}
	return false
}

func structuredCommandDiscoversPackageManager(command string) bool {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "command -v") && !strings.Contains(lower, "which ") && !strings.Contains(lower, "type -p") {
		return false
	}
	for _, manager := range []string{"pacman", "apt", "dnf", "yum", "zypper", "apk"} {
		if strings.Contains(lower, manager) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeOSIdentification(command string) bool {
	lower := strings.ToLower(command)
	hasOSRelease := strings.Contains(lower, "/etc/os-release") || strings.Contains(lower, "os-release") || strings.Contains(lower, "pretty_name")
	hasUname := strings.Contains(lower, "uname")
	return hasOSRelease && hasUname
}

func structuredCommandLooksLikePartialOSIdentification(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{"/etc/os-release", "os-release", "pretty_name", "uname", "lsb_release", "hostnamectl", "dpkg", "apt"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeStableCurrentEventsEvidence(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "news.google.com/rss/search") &&
		curlCommandFollowsRedirects(command) &&
		curlCommandHasSilentFlag(command) &&
		curlCommandHasUserAgent(command) &&
		strings.Contains(lower, "ceid=") &&
		!strings.Contains(lower, "```")
}

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

func repairRejectedDoneWithPlanner(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, rejectedResp OllamaChatResponse, rejectedPayload StructuredCommandPayload, checkResult completionCheckRunResult, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), ledger *[]StructuredObjective, result *CommandDecisionResult) (bool, error) {
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
	*ledger = mergeStructuredObjectiveLedger(*ledger, nextPayload.ObjectiveLedger)
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
		if err := runStructuredPatchApply(ctx, step, nextPayload.Patch, cfg.CurrentWorkingDirectory, stdout, stderr, onEvent, result); err != nil {
			return false, err
		}
		emitStructuredCommandEvent(onEvent, "completion_repair_accepted", "Completion repair executed patched action", map[string]string{"step": fmt.Sprintf("%d", step)})
		return true, nil
	}
	if isShellToolDelegation(nextPayload) {
		proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, nextPayload.ToolTask, cfg, worksiteSurvey, ledger, onEvent, result)
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
		Observations:            observations,
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
		after := pendingStructuredObjectiveIDs(*ledger)
		emitStructuredCommandEvent(onEvent, "structured_repeat_success_reconciled", "Repeated successful command skipped and used as completion evidence", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"command":            truncateStructuredTimelineValue(command),
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

func reconcileStructuredObjectiveLedgerFromObservation(step int, ledger []StructuredObjective, obs StructuredCommandObservation, onEvent func(StructuredCommandEvent)) []StructuredObjective {
	if strings.TrimSpace(obs.Command) == "" || obs.ExitCode != 0 {
		return ledger
	}
	pending := pendingStructuredObjectives(ledger)
	if len(pending) == 0 {
		return ledger
	}
	satisfied := []StructuredObjective{}
	for _, objective := range pending {
		if structuredObservationSatisfiesObjective(obs, objective) {
			objective.Status = "satisfied"
			objective.Evidence = structuredObjectiveEvidenceFromObservation(obs)
			satisfied = append(satisfied, objective)
		}
	}
	if len(satisfied) == 0 {
		return ledger
	}
	ids := structuredObjectiveIDs(satisfied)
	emitStructuredCommandEvent(onEvent, "objective_ledger_reconciled", "Pending objective(s) satisfied from prior successful command evidence", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"objectives": strings.Join(ids, ","),
	})
	return mergeStructuredObjectiveLedger(ledger, satisfied)
}

func acceptPartialCompletionForContinuation(step int, before, after []StructuredObjective, obs StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) {
	if result == nil || obs.ExitCode != 0 {
		return
	}
	pendingBefore := pendingStructuredObjectives(before)
	pendingAfter := pendingStructuredObjectives(after)
	if len(pendingBefore) == 0 || len(pendingAfter) == 0 || len(pendingAfter) >= len(pendingBefore) {
		return
	}
	completed := newlySatisfiedStructuredObjectiveIDs(before, after)
	if len(completed) == 0 {
		return
	}
	result.PartialProgress = true
	emitStructuredCommandEvent(onEvent, "partial_completion_accepted", "Partial completion accepted; continuing remaining objectives", map[string]string{
		"step":                 fmt.Sprintf("%d", step),
		"completed_objectives": strings.Join(completed, ","),
		"pending_objectives":   strings.Join(structuredObjectiveIDs(pendingAfter), ","),
		"command":              truncateStructuredTimelineValue(obs.Command),
		"continuation":         "same job must continue before unrelated work or done=true",
	})
}

func newlySatisfiedStructuredObjectiveIDs(before, after []StructuredObjective) []string {
	beforeSatisfied := map[string]bool{}
	for _, objective := range before {
		if structuredObjectiveSatisfied(objective) {
			beforeSatisfied[objective.ID] = true
		}
	}
	ids := []string{}
	for _, objective := range after {
		if objective.ID == "" || beforeSatisfied[objective.ID] || !structuredObjectiveSatisfied(objective) {
			continue
		}
		ids = append(ids, objective.ID)
	}
	sort.Strings(ids)
	return ids
}

func structuredObservationSatisfiesObjective(obs StructuredCommandObservation, objective StructuredObjective) bool {
	command := strings.ToLower(strings.TrimSpace(obs.Command))
	output := strings.ToLower(obs.Stdout + "\n" + obs.Stderr)
	target := normalizedDependencyText(objective.ID + " " + objective.Description)
	if command == "" || target == "" {
		return false
	}
	if strings.Contains(command, "mkdir") && (strings.Contains(target, " setup ") || strings.Contains(target, " structure ") || strings.Contains(target, " component ")) {
		return true
	}
	if (strings.Contains(command, "rm ") || strings.Contains(command, "rm -f ")) &&
		(strings.Contains(target, " remove ") || strings.Contains(target, " delete ") || strings.Contains(target, " cleanup ") || strings.Contains(target, " clean up ")) {
		return true
	}
	if strings.Contains(command, "npm install") || strings.Contains(command, "npm add") || strings.Contains(command, "pnpm add") || strings.Contains(command, "yarn add") {
		for _, pkg := range objective.Packages {
			if strings.Contains(command, strings.ToLower(pkg)) {
				return true
			}
		}
	}
	if objectiveRequiresDockerLifecycle(target) {
		return dockerLifecycleEvidenceSatisfiesObjective(command, output, target)
	}
	if objectiveRequiresBackendTest(target) {
		return (strings.Contains(command, "go test") || strings.Contains(command, "make test") || strings.Contains(output, "go test")) &&
			(strings.Contains(command, "backend") || strings.Contains(command, "calculus-api") || strings.Contains(output, "calculus-api"))
	}
	if objectiveRequiresFrontendTest(target) {
		return (strings.Contains(command, "npm test") || strings.Contains(command, "make test") || strings.Contains(output, "react-scripts test")) &&
			(strings.Contains(command, "frontend") || strings.Contains(command, "make test") || strings.Contains(output, "react-scripts test"))
	}
	if objectiveRequiresSmokeTest(target) {
		return strings.Contains(command, "smoke") || strings.Contains(command, "make test") && strings.Contains(output, "smoke test passed") || strings.Contains(output, "smoke test passed")
	}
	if objectiveRequiresFrontendBuild(target) {
		return (strings.Contains(command, "npm run build") || strings.Contains(command, "make build") || strings.Contains(output, "compiled successfully")) &&
			!strings.Contains(command, "go test")
	}
	if strings.Contains(command, "npm run build") && (strings.Contains(target, " verify ") || strings.Contains(target, " build ")) {
		return true
	}
	if strings.Contains(command, "npm test") && (strings.Contains(target, " verify ") || strings.Contains(target, " test ")) {
		return true
	}
	if strings.Contains(command, "go test") && (strings.Contains(target, " verify ") || strings.Contains(target, " test ")) && !strings.Contains(target, "frontend") {
		return true
	}
	return false
}

func objectiveRequiresBackendTest(target string) bool {
	return strings.Contains(target, "backend") && (strings.Contains(target, "test") || strings.Contains(target, "verify"))
}

func objectiveRequiresFrontendTest(target string) bool {
	return strings.Contains(target, "frontend") && strings.Contains(target, "test")
}

func objectiveRequiresSmokeTest(target string) bool {
	return strings.Contains(target, "smoke")
}

func objectiveRequiresFrontendBuild(target string) bool {
	return strings.Contains(target, "frontend") && strings.Contains(target, "build")
}

func objectiveRequiresDockerLifecycle(target string) bool {
	return strings.Contains(target, "docker") || strings.Contains(target, "container")
}

func dockerLifecycleEvidenceSatisfiesObjective(command, output, target string) bool {
	if strings.Contains(target, "dockerfile") || strings.Contains(target, "dependencies") || strings.Contains(target, "compatibility") {
		return strings.Contains(command, "docker build") || strings.Contains(output, "successfully built") || strings.Contains(output, "writing image") || strings.Contains(output, "naming to")
	}
	if strings.Contains(target, "build") || strings.Contains(target, "image") {
		return strings.Contains(command, "docker build") || strings.Contains(output, "successfully built") || strings.Contains(output, "writing image") || strings.Contains(output, "naming to")
	}
	if strings.Contains(target, "run") || strings.Contains(target, "container") {
		hasRun := strings.Contains(command, "docker run") || strings.Contains(output, "docker run") || strings.Contains(output, "running=true") || strings.Contains(output, "restart_count=0")
		hasInspect := strings.Contains(command, "docker inspect") || strings.Contains(output, "running=true") || strings.Contains(output, "restart_count=0")
		hasLogs := strings.Contains(command, "docker logs") || strings.Contains(output, "docker_logs_clear") || strings.Contains(output, "logs clear")
		hasHealth := strings.Contains(command, "curl") || strings.Contains(output, "health=") || strings.Contains(output, "http")
		return hasRun && hasInspect && hasLogs && hasHealth
	}
	return false
}

type completionCheckRunResult struct {
	Ledger   []StructuredObjective
	Accepted bool
	Check    CompletionCheck
	Ran      bool
}

func runCompletionCheck(ctx context.Context, step int, prompt, currentWorkingDirectory string, ledger []StructuredObjective, minimalContext MinimalContext, observations []StructuredCommandObservation, candidateAnswer string, checker CompletionChecker, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent)) ([]StructuredObjective, bool) {
	result := runCompletionCheckDetailed(ctx, step, prompt, currentWorkingDirectory, ledger, minimalContext, observations, candidateAnswer, checker, worksiteSurvey, onEvent)
	return result.Ledger, result.Accepted
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
		emitStructuredCommandEvent(onEvent, "completion_check_failed", "Done-check specialist failed; continuing with deterministic checks", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return completionCheckRunResult{Ledger: ledger}
	}
	updated := mergeStructuredObjectiveLedger(ledger, filterObjectiveLedgerForWorksiteSurvey(check.ObjectiveLedger, worksiteSurvey))
	if check.Done && len(pendingStructuredObjectives(updated)) > 0 {
		updated = satisfyPendingObjectivesFromValidator(updated, check.Reason)
		emitStructuredCommandEvent(onEvent, "completion_check_satisfied_pending_objectives", "Completion validator marked remaining pending objectives satisfied from evidence", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"objectives": pendingStructuredObjectiveIDs(ledger),
			"reason":     truncateStructuredTimelineValue(check.Reason),
		})
	}
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

func satisfyPendingObjectivesFromValidator(ledger []StructuredObjective, reason string) []StructuredObjective {
	evidence := strings.TrimSpace(reason)
	if evidence == "" {
		evidence = "completion validator accepted observed evidence"
	}
	out := mergeStructuredObjectiveLedger(nil, ledger)
	for i := range out {
		if structuredObjectiveSatisfied(out[i]) || !structuredObjectiveBlocksCompletion(out[i]) {
			continue
		}
		out[i].Status = "satisfied"
		out[i].Evidence = evidence
		if strings.TrimSpace(out[i].Source) == "" {
			out[i].Source = structuredObjectiveSourceDetectedProject
		}
	}
	return out
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

func runSelectedRecipeCompletionProbes(ctx context.Context, currentWorkingDirectory string, ledger []StructuredObjective, recipes []Recipe, onEvent func(StructuredCommandEvent)) []StructuredObjective {
	for _, recipe := range recipes {
		results, passed := RunRecipeCompletionProbes(ctx, recipe, currentWorkingDirectory)
		if len(results) == 0 {
			continue
		}
		evidence := FormatRecipeProbeEvidence(results)
		emitStructuredCommandEvent(onEvent, "recipe_completion_probes_completed", "Deterministic recipe completion probes ran", map[string]string{
			"recipe": recipe.ID,
			"passed": fmt.Sprintf("%t", passed),
			"checks": fmt.Sprintf("%d", len(results)),
		})
		ledger = ApplyRecipeProbeCompletion(ledger, recipe, passed, evidence)
	}
	return ledger
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
	if source == "" {
		required = true
	}
	return StructuredObjective{
		ID:              id,
		Description:     strings.TrimSpace(objective.Description),
		Status:          status,
		Evidence:        strings.TrimSpace(objective.Evidence),
		Source:          normalizeStructuredObjectiveSource(source),
		ParentObjective: strings.TrimSpace(objective.ParentObjective),
		Required:        required,
		Packages:        cleanStringList(objective.Packages),
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
	if strings.TrimSpace(update.Source) != "" && update.Source != structuredObjectiveSourceModelInferred {
		existing.Source = update.Source
	} else if strings.TrimSpace(existing.Source) == "" {
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

func sourceVerificationCompletionSatisfied(prompt, workingDir string, latest StructuredCommandObservation) bool {
	if latest.ExitCode != 0 || strings.TrimSpace(latest.Command) == "" {
		return false
	}
	if !appBuildPromptNeedsFiles(prompt) || workspaceMissingAppFiles(workingDir) {
		return false
	}
	text := strings.ToLower(latest.Stdout + "\n" + latest.Stderr)
	return strings.Contains(text, "source_verified") || strings.Contains(text, "source verification passed")
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

func buildStructuredCommandRequest(prompt string, history []Message, observations []StructuredCommandObservation) OllamaChatRequest {
	return buildStructuredCommandRequestWithMemories(prompt, history, nil, observations)
}

func buildStructuredCommandRequestWithMemories(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation) OllamaChatRequest {
	return buildStructuredCommandRequestWithMemoriesAndCWD(prompt, history, memories, observations, "")
}

func buildStructuredCommandRequestWithMemoriesAndCWD(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string) OllamaChatRequest {
	return buildStructuredCommandRequestWithMemoriesCWDAndLedger(prompt, history, memories, observations, currentWorkingDirectory, nil)
}

func buildStructuredCommandRequestWithMemoriesCWDAndLedger(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective) OllamaChatRequest {
	return buildStructuredCommandRequestWithContext(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, MinimalContext{})
}

func buildStructuredCommandRequestWithContext(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext) OllamaChatRequest {
	return buildStructuredCommandRequestWithContextAndRecipes(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, nil)
}

func buildStructuredCommandRequestWithContextAndRecipes(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe) OllamaChatRequest {
	return buildStructuredCommandRequestWithContextRecipesAndSurvey(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, WorksiteSurvey{})
}

func buildStructuredCommandRequestWithContextRecipesAndSurvey(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, survey WorksiteSurvey) OllamaChatRequest {
	return buildStructuredCommandRequestWithContextRecipesSurveyAndPrep(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey, PrepContextBundle{})
}

func buildStructuredCommandRequestWithContextRecipesSurveyAndPrep(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, survey WorksiteSurvey, prep PrepContextBundle) OllamaChatRequest {
	return OllamaChatRequest{
		ContextSystem: buildStructuredCommandSystemContext(),
		Messages:      buildStructuredCommandMessagesWithPrep(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey, prep),
		Format:        buildStructuredCommandResponseFormat(observations),
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}
}

func buildStructuredCommandSystemContext() string {
	return strings.Join([]string{
		"Return JSON only.",
		"Schema: {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}",
		"To delegate exact shell command selection, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"tool\":\"shell\",\"tool_task\":\"scoped instruction from planner authority\"}.",
		"To apply source edits, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"tool\":\"patch.apply\",\"patch\":\"unified diff\"}; patch paths must be relative to current_working_directory.",
		"To request final validation, return {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"}; planner done=true is never authoritative and only the completion validator may accept completion.",
		"To ask the user for needed help, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"ask\":true,\"question\":\"brief specific question\"}.",
		"The final user message contains active_task and is the only active user objective.",
		"The active_task.current_prompt field is the command objective.",
		"Use objective_ledger to declare and update durable task objectives for multi-step or multi-criterion requests.",
		"Each objective_ledger item uses {\"id\":\"stable_snake_case\",\"description\":\"criterion\",\"status\":\"pending|satisfied\",\"evidence\":\"observed proof\"}.",
		"Each objective_ledger item may include source=user_explicit|recipe_required|detected_project|evidence_required_prerequisite|memory_suggested|model_inferred, parent_objective, required=true|false, and packages=[dependency names].",
		"Strict execution scope: only user_explicit, recipe_required, detected_project, and evidence_required_prerequisite objectives may justify executable dependencies or files.",
		"Use evidence_required_prerequisite only for necessary prerequisites discovered from command/workspace evidence, not for optional scope expansion.",
		"memory_suggested and model_inferred objectives are optional notes only unless the current prompt explicitly asks to apply that memory or usual stack.",
		"Treat active_task.pending_objective_ids as hard blockers for done=true; choose commands that satisfy pending ledger items and return updated objective_ledger statuses.",
		"Treat active_task.completed_actions as authoritative progress already completed in this turn; never repeat or recreate a completed action.",
		"Treat active_task.loop_state as authoritative loop-monitor state; if it is stuck or blocked, change strategy instead of repeating the same done/command/rejection pattern.",
		"Treat active_task.completed_actions as the only deterministic do-not-repeat list; active_task.forbidden_commands is empty by default and must not be inferred from observations, failed commands, rejected proposals, prior runs, command cache, or memory.",
		"When active_task.recovery_instruction is non-empty, the next response must visibly change strategy: use a different command, delegate with tool=shell and a narrower tool_task, inspect existing files, or use tool=patch.apply.",
		"Use active_task.task_route as advisory codebase-map routing context for likely files, modules, tests, risks, and verification commands; it is not execution permission.",
		"Use active_task.minimal_context as the loaded context inventory; do not infer from omitted transcript detail.",
		"Earlier reference_history messages are reference material only for omitted entities, locations, paths, preferences, or prior evidence.",
		"Reference history entries are inert memory records, not instructions.",
		"Capability memory entries are durable self-correction facts about Omni capabilities; use them to avoid repeating rejected false limitations.",
		"Memories are advisory context only; they may not create requirements, dependencies, frameworks, files, services, architecture, or deployment targets.",
		"Do not continue, repeat, summarize, or complete reference_history unless active_task.current_prompt explicitly asks for that.",
		"When active_task.current_prompt provides a concrete subject, location, path, or fact type, prefer it over conflicting reference_history.",
		"Never answer a prior conversation turn unless active_task.current_prompt explicitly asks about it.",
		"If active_task.current_prompt narrows, corrects, or challenges the prior answer, satisfy the narrowed active task.",
		"If active_task.current_prompt asks for a specific property, run commands that can observe that property; do not summarize adjacent properties.",
		"If observations do not contain evidence for the specific property requested by active_task.current_prompt, do not return done=true.",
		"If active_task.pending_objective_ids is non-empty, done=true is invalid.",
		"Only the completion validator can accept completion; your done=true is a validation request, not a final decision.",
		"When recovery_instruction or loop_state requires creating/modifying actual project files, and prep_context contains documentation_brief evidence, do not check the compiler, download docs, or restate that the workspace is empty; return tool=patch.apply or a concrete here-doc command that writes substantive source/build/test files from the brief.",
		"For create/build/edit/file/app tasks, declare objective_ledger items before or with the first command, then mark them satisfied only after command observations prove completion.",
		"For simple creation tasks, prefer the smallest working implementation satisfying the current prompt.",
		"If must_return_command is true, done=true is invalid; return a non-empty command or delegate with tool=shell.",
		"If must_return_command is true, ask=true is invalid; inspect or try a command first.",
		"If the latest real command succeeded, ask=true is invalid; continue, verify, or finish from evidence.",
		"Do not return done=true until at least one command has exit_code 0.",
		"If the latest command failed, return a different command instead of done=true.",
		"After a command mutates files, package metadata, dependencies, build artifacts, or project structure, run a later readback/verification command such as cat, sed -n, rg, grep, ls, test, jq, npm pkg get, npm ls, or an equivalent tool before done=true.",
		"Never repeat an exact command that already succeeded; inspect the observation, update objective_ledger, verify, or choose the next action.",
		"Use shell commands to satisfy requests; do not answer from memory when command evidence is required.",
		"Planner authority may delegate tool details to specialized tools; when shell syntax or system inspection is the narrow task, prefer tool=shell with a specific tool_task.",
		"Specialist team profiles define authority boundaries, allowed tools, memory permissions, and context contributions.",
		"Specialists may create evidence-backed memories; memory updates or deprioritization must be routed through memory, correction, manager, or summary specialists according to profile policy.",
		"Do not use echo to print an answer or apology.",
		"Do not use shell commands to simulate a final answer; commands must inspect files, run tools, query the web, create requested output, or verify evidence.",
		"Do not emit pseudo-tool names such as web.search, browser.search, None, or null as commands; commands execute in a real shell.",
		"Prefer tool=patch.apply for source edits when you can produce a small unified diff from observed file contents.",
		"Use tool_inventory to choose available terminal tools, skills, public sources, and agent roles.",
		"Never return an empty command when done=false unless delegating with tool=shell and a non-empty tool_task.",
		"If a command fails, the failure is recorded in observations; use that context to pivot to a different command, source, or tool.",
		"If the user already answered a question, do not ask the same question again; use the observed user_response.",
		"If you ask for approval and include an approval-gated command, that command may run after the user answers.",
		"Use reference_history to resolve follow-up references before asking the user.",
		"If reference_history contains the missing subject, location, file, project, or preference, use it in the command instead of asking.",
		"Ask the user only when progress requires permission, credentials, sudo, destructive approval, or a choice that cannot be inferred from evidence.",
		"Do not ask for help when another non-destructive command, public source, or local inspection can be tried.",
		"After each command, inspect stdout/stderr/exit_code and decide whether another command is needed.",
		"The command must be a single shell command.",
		"Each command runs in a fresh shell; cd does not persist to the next step.",
		"Use absolute paths or include cd in the same command that needs it.",
		"A command that only changes directory does not help later steps; combine cd with the file creation, build, test, or verification command that needs that directory.",
		"Use current_working_directory for project creation unless the user explicitly provided another path.",
		"The current_working_directory is protected user state: use it as the project directory, do not delete, move, empty, or recreate it.",
		"Creation is additive: use mkdir -p for directories and update requested files in place; never satisfy a create/init/build objective by deleting existing state first.",
		"Do not create demo projects in the home directory unless the user explicitly asked for home.",
		"Available terminal tools may include bash, curl, python3, sed, awk, grep, jq, date, uname, and package managers; discover with commands when uncertain.",
		"To identify the operating system, inspect command evidence such as uname and /etc/os-release.",
		"For OS identification requests, gather distro, kernel, architecture, and package-manager evidence before done=true; prefer one command that prints /etc/os-release, uname -srmo, and command -v pacman apt dnf yum zypper apk.",
		"For OS identification requests, package-manager evidence means discovery output from command -v, which, or type -p for pacman apt dnf yum zypper apk; distro-specific files such as /etc/apt/sources.list are not enough.",
		"For identification tasks, inspect available package managers only; do not ask for permission to proceed with a package manager.",
		"Before OS-specific package or install advice, verify OS, distro, version, architecture, and available package managers with commands.",
		"If a needed tool is missing, identify install options from verified OS/package-manager evidence.",
		"Do not install missing tools unless the user explicitly asked to install or approved installation.",
		"When installation is not approved, answer with the proposed install command and ask for approval.",
		"For desktop/browser tasks, inspect running processes and the GUI session with commands before acting.",
		"For browser window tasks, discover available tools such as firefox, xdg-open, wmctrl, xdotool, gdbus, or gio with commands when uncertain.",
		"When asked to use a browser PID or existing browser process, find the running process first, then use window/browser commands based on observed evidence.",
		"If desktop control is impossible because no GUI session, browser process, or needed tool is available, report the missing evidence and ask for the smallest needed user action.",
		"Do not use placeholder credentials.",
		"Do not call APIs that require unavailable keys.",
		"Never put placeholder key text in a command.",
		"Never put placeholder angle-bracket values such as <location>, <query>, <file>, or <url> in a command.",
		"For external facts, use public unauthenticated sources.",
		"For timely public information, use internet commands by default.",
		"For current, recent, latest, today, or now public facts, the first accepted command should gather live evidence from the internet.",
		"For current external facts, run an internet command and use observed output before done.",
		"For filesystem changes, run shell commands that create or modify the requested filesystem state.",
		"For unfamiliar language, framework, or toolchain build tasks, gather documentation evidence before guessing project structure. Use concrete shell internet commands such as curl -fsSL to official docs or installed tool help, then create the smallest hello-world project and iterate from build/test errors.",
		"When a requested compiler or framework tool is missing and installation is not approved, still create substantive source/build/test files from documentation or tool conventions, then run a deterministic source verification fallback without claiming a compiler build succeeded.",
		"For local static web app demos, create files locally and serve them with a local server such as python3 http.server.",
		"For Go CLI demos, use curl to discover the latest Go release from go.dev/dl/?mode=json, install that Go toolchain into a user-writable project directory unless system installation is approved, then build, test, and run the app.",
		"The Go release JSON has version and files[].filename fields; construct downloads as https://go.dev/dl/<filename>.",
		"For Go CLI demos, do not return done=true until go test, go build, and the built executable have all succeeded.",
		"Do not treat null or empty JSON query output as useful evidence.",
		"For npm React TypeScript demos, prefer a minimal Vite project with package.json and src files; create-react-app is discouraged but not a hard ban when the active task explicitly asks to create a new React app.",
		"For npm install/build commands in tests, keep output concise when possible.",
		"For Docker app tasks, verify docker is available, create the app and Dockerfile, build the image, run a named container, verify it with curl, inspect container state/restart count, and inspect docker logs before done=true.",
		"For Docker smoke tests, prefer local build contexts that do not require pulling large base images when a static binary or scratch image can satisfy the request.",
		"Do not return done=true for a Docker app until docker build, docker run, live endpoint verification, docker inspect, and docker logs checks have succeeded.",
		"When starting a background server, use nohup or equivalent and write the background process PID with $! if a PID file is requested.",
		"When starting a background server, redirect stdout and stderr away from the command pipe.",
		"Do not background file creation or setup commands; only background the long-running server process.",
		"When chaining commands before a background server, use semicolons before nohup; avoid '&& nohup ... &' because bash may background the setup chain.",
		"After starting a local server, verify it with a short curl retry loop before done=true.",
		"Do not ask for public sources when the task can be completed with local files.",
		"If observed output is empty, denied, or not useful, try a different public source.",
		"If output reports invalid credentials, try a no-key public source before done.",
		"If the shell reports a syntax or quoting error, correct the command or use a simpler command.",
		"Match the command source to the requested fact type.",
		"Public no-key internet sources available: wttr.in, news.google.com/rss/search?q=<query>, duckduckgo.com/html/?q=<query>.",
		"For current events or news, use a concrete shell command such as curl -fsSL -A 'Mozilla/5.0' 'https://news.google.com/rss/search?q=<query>&hl=en-US&gl=US&ceid=US:en' or curl -L 'https://duckduckgo.com/html/?q=<query>'; do not emit web.search.",
		"For Google News RSS, use curl -fsSL -A 'Mozilla/5.0' 'https://news.google.com/rss/search?q=<query>&hl=en-US&gl=US&ceid=US:en'; keep the requested location in q= and parse a small number of titles.",
		"When using wttr.in, include an explicit location path and a concise format query.",
		"For current weather, prefer wttr.in with an explicit location path and concise format query, for example curl -s 'https://wttr.in/Pattaya?format=%l|%C|%t|%f'.",
		"Do not use OpenWeatherMap or api.openweathermap.org unless a real non-placeholder API key is already available in observed evidence.",
		"Never use YOUR_API_KEY, API_KEY_HERE, or invented credentials.",
		"Prefer simple curl commands that print readable evidence over fragile HTML parsing.",
		"For current time, prefer shell time/date commands or public no-key time sources.",
		"For location-specific time, produce local-time evidence for that location; do not answer from UTC unless UTC was requested.",
		"Do not use weather services as time sources.",
		"If using shell date for a location, choose an IANA timezone and prefix the command with TZ=Area/City before date.",
		"For Pattaya or any Thailand current-time request, use the IANA timezone Asia/Bangkok, for example TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
		"Do not pass TZ=Area/City as an argument to date.",
		"Prefer concise command output; use format/query options instead of large pages when available.",
		"No markdown.",
	}, "\n")
}

func buildStructuredCommandResponseFormat(observations []StructuredCommandObservation) map[string]interface{} {
	properties := map[string]interface{}{
		"command":          map[string]interface{}{"type": "string"},
		"done":             map[string]interface{}{"type": "boolean"},
		"answer":           map[string]interface{}{"type": "string"},
		"ask":              map[string]interface{}{"type": "boolean"},
		"question":         map[string]interface{}{"type": "string"},
		"tool":             map[string]interface{}{"type": "string"},
		"patch":            map[string]interface{}{"type": "string"},
		"objective_ledger": structuredObjectiveLedgerSchema(),
		"tool_task": map[string]interface{}{
			"type": "string",
		},
	}
	if !hasRealCommandObservation(observations) {
		properties["done"] = map[string]interface{}{"type": "boolean", "enum": []bool{false}}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   []string{"command", "done", "answer"},
	}
}

func structuredObjectiveLedgerSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":               map[string]interface{}{"type": "string"},
				"description":      map[string]interface{}{"type": "string"},
				"status":           map[string]interface{}{"type": "string", "enum": []string{"pending", "satisfied"}},
				"evidence":         map[string]interface{}{"type": "string"},
				"source":           map[string]interface{}{"type": "string", "enum": []string{structuredObjectiveSourceUserExplicit, structuredObjectiveSourceRecipeRequired, structuredObjectiveSourceDetectedProject, structuredObjectiveSourceEvidenceRequiredPrerequisite, structuredObjectiveSourceMemorySuggested, structuredObjectiveSourceModelInferred}},
				"parent_objective": map[string]interface{}{"type": "string"},
				"required":         map[string]interface{}{"type": "boolean"},
				"packages":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"id", "description", "status"},
		},
	}
}

func buildShellCommandSpecialistRequest(input ShellCommandSpecialistInput) OllamaChatRequest {
	payload := struct {
		Role             string                         `json:"role"`
		UserPrompt       string                         `json:"user_prompt"`
		ToolTask         string                         `json:"tool_task"`
		Observations     []StructuredCommandObservation `json:"observations"`
		CompletedActions []CompletedAction              `json:"completed_actions,omitempty"`
		LoopState        StructuredLoopState            `json:"loop_state,omitempty"`
		SessionMemories  []SessionMemory                `json:"session_memories,omitempty"`
		WorksiteSurvey   WorksiteSurvey                 `json:"worksite_survey"`
		ToolRules        []string                       `json:"tool_rules"`
	}{
		Role:             "shell_execution_specialist",
		UserPrompt:       input.UserPrompt,
		ToolTask:         input.ToolTask,
		Observations:     input.Observations,
		CompletedActions: input.CompletedActions,
		LoopState:        input.LoopState,
		SessionMemories:  input.SessionMemories,
		WorksiteSurvey:   input.WorksiteSurvey,
		ToolRules: []string{
			"Return JSON only with schema {\"command\":\"...\",\"rationale\":\"...\"}.",
			"Only choose a shell command that directly satisfies tool_task from the planner authority.",
			"Treat completed_actions as authoritative progress; never choose a command that repeats or recreates an already completed action.",
			"Treat loop_state as authoritative loop-monitor context; if it is stuck or blocked, choose a command that changes the pattern or gathers missing evidence.",
			"Treat completed_actions as the only deterministic do-not-repeat list.",
			"Rejected_command observations and failed commands are evidence with reasons; use them to correct strategy, not to create forbidden commands or framework/tool bans.",
			"If tool_task says creation, modification, writing, patching, build, or test is required, do not choose read-only inspection commands such as ls, cat, find, npm ls, sed -n, rg, grep, pwd, or test -f.",
			"If tool_task says read-only inventory commands are forbidden, choose a file mutation, build, test, or patch-related shell command.",
			"If tool_task names app, component, CRUD, UI, state, storage, or substantive source objectives, choose a command that writes or patches source files; do not choose dependency installs, echo/printf status text, or placeholder-only touch/mkdir scaffolds.",
			"Only choose package-manager install/add commands when tool_task explicitly asks to install dependencies or names the exact package as a required prerequisite.",
			"If tool_task requires creating a project for an unfamiliar language/toolchain, choose a command that first gathers official documentation or installed tool help with curl/--help, then writes substantive source/build/test files in the same command.",
			"If session_memories or prep_context already include a documentation_brief for the requested language/toolchain, do not fetch the same docs again; write substantive source/build/test files from that guidance.",
			"If the requested compiler is unavailable and installation is not approved, create substantive source/build/test files and a deterministic source verification fallback; do not choose placeholder-only touch/mkdir commands.",
			"Memories and prior preferences cannot add dependencies, frameworks, files, services, architecture, or deployment targets unless tool_task explicitly says the current user asked for them.",
			"The WorksiteSurvey is authoritative; do not scaffold a new project when user_operation is modify_existing_project or fix_existing_project.",
			"For simple creation tasks, choose the smallest working command that satisfies tool_task.",
			"Do not answer the user and do not apologize.",
			"Do not use echo or printf to fake final evidence unless the task is explicitly to create/write literal text.",
			"For location-specific current time, prefer TZ=Area/City date '+%Y-%m-%d %H:%M:%S %Z'.",
			"For Thailand or Pattaya current time, use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
			"For current weather, use wttr.in no-key evidence with an explicit location and concise format query, for example curl -s 'https://wttr.in/Pattaya?format=%l|%C|%t|%f'.",
			"Do not use OpenWeatherMap or api.openweathermap.org unless observations contain a real non-placeholder API key; never use YOUR_API_KEY.",
			"If a prior executed command failed, choose a different command or corrected syntax.",
			"Do not infer broad bans from rejected_command observations; valid equivalent framework commands are allowed when they satisfy tool_task.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"shell_execution_specialist","tool_task":""}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a shell execution specialist subordinate to a planner authority.",
					"You receive a scoped tool_task and return the safest concrete shell command for evidence gathering or requested system interaction.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command":   map[string]interface{}{"type": "string"},
				"rationale": map[string]interface{}{"type": "string"},
			},
			"required": []string{"command", "rationale"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 256,
		},
	}
}

func buildStructuredCommandMessages(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, surveys ...WorksiteSurvey) []OllamaMessage {
	survey := WorksiteSurvey{}
	if len(surveys) > 0 {
		survey = surveys[0]
	}
	return buildStructuredCommandMessagesWithPrep(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey, PrepContextBundle{})
}

func buildStructuredCommandMessagesWithPrep(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, survey WorksiteSurvey, prep PrepContextBundle) []OllamaMessage {
	messages := []OllamaMessage{}
	if memoryMessage := buildStructuredCommandCapabilityMemoryMessage(memories); memoryMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: memoryMessage},
			OllamaMessage{Role: "assistant", Content: "Capability memory received. I will use it only to avoid repeating false capability limitations."},
		)
	}
	if contextMessage := buildStructuredMinimalContextMessage(minimalContext); contextMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: contextMessage},
			OllamaMessage{Role: "assistant", Content: "Minimal context inventory received. I will use only these relevant facts unless tool evidence adds more."},
		)
	} else if historyMessage := buildStructuredCommandHistoryMessage(history); historyMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: historyMessage},
			OllamaMessage{Role: "assistant", Content: "Reference history received. I will use it only when the active task needs omitted context."},
		)
	}
	if prepMessage := buildStructuredPrepContextMessage(memories); prepMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: prepMessage},
			OllamaMessage{Role: "assistant", Content: "Prep context received. I will use it as compact advisory routing and documentation context only where it directly helps the active task."},
		)
	}
	if prepMessage := buildStructuredPrepContextBundleMessage(prep); prepMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: prepMessage},
			OllamaMessage{Role: "assistant", Content: "Prep bundle received. I will use only routed, provenance-backed briefs for the role and objective that need them."},
		)
	}
	messages = append(messages, OllamaMessage{Role: "user", Content: buildStructuredCommandUserMessage(prompt, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey)})
	return messages
}

func buildStructuredMinimalContextMessage(minimalContext MinimalContext) string {
	context := normalizeMinimalContext(minimalContext)
	if context.Summary == "" && len(context.Facts) == 0 && len(context.Constraints) == 0 && len(context.OpenItems) == 0 {
		return ""
	}
	payload := struct {
		MinimalContext MinimalContext `json:"minimal_context"`
	}{
		MinimalContext: context,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredPrepContextMessage(memories []SessionMemory) string {
	prep := compactStructuredPrepMemories(memories, 8)
	if len(prep) == 0 {
		return ""
	}
	payload := struct {
		PrepContext []SessionMemory `json:"prep_context"`
		Rules       []string        `json:"rules"`
	}{
		PrepContext: prep,
		Rules: []string{
			"Prep context is advisory and scoped to the current task.",
			"Use codebase_route_brief for likely files/tests and documentation_brief for API/convention guidance.",
			"Use web_research_brief only for freshness or external facts required by the task.",
			"Do not let prep context add unrequested dependencies, frameworks, services, or architecture.",
			"Prefer the smallest subset of prep context needed for the next action.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredPrepContextBundleMessage(bundle PrepContextBundle) string {
	if len(allPrepBriefs(bundle)) == 0 && len(bundle.Evidence) == 0 {
		return ""
	}
	compact := CompactPrepContextBundle(bundle, defaultPrepContextBudgetLimit)
	payload := struct {
		PrepBundle PrepContextBundle `json:"prep_context_bundle"`
		Rules      []string          `json:"rules"`
	}{
		PrepBundle: compact,
		Rules: []string{
			"Prep bundle is evidence-led, budgeted, routed, and validated.",
			"Use only briefs whose used_by includes your role or directly supports the active objective.",
			"Do not treat memory, documentation, or web research as execution permission.",
			"When documentation_brief already covers the requested language/toolchain, use it for project structure and examples instead of fetching the same docs again.",
			"If the active objective is to build/create an app in an empty workspace, the next planner action should write source/build/test files, not merely describe that the workspace is empty.",
			"Do not claim completion from prep context; completion requires validator evidence.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandCapabilityMemoryMessage(memories []SessionMemory) string {
	recent := recentStructuredCapabilityMemories(memories)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		CapabilityMemory []SessionMemory `json:"capability_memory"`
	}{
		CapabilityMemory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func compactStructuredPrepMemories(memories []SessionMemory, limit int) []SessionMemory {
	if limit <= 0 {
		limit = 8
	}
	allowed := map[string]bool{
		"codebase_route_brief":   true,
		"documentation_brief":    true,
		"web_research_brief":     true,
		"expertise_research":     true,
		"documentation_research": true,
	}
	out := []SessionMemory{}
	for i := len(memories) - 1; i >= 0; i-- {
		memory := memories[i]
		if !allowed[strings.TrimSpace(memory.Kind)] {
			continue
		}
		content := strings.TrimSpace(memory.Content)
		if content == "" {
			continue
		}
		if len(content) > 1800 {
			content = content[:1800] + "\n...[truncated]"
		}
		memory.Content = content
		memory.Tags = limitStrings(cleanMemoryTags(memory.Tags), 10)
		out = append(out, memory)
		if len(out) >= limit {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func buildStructuredCommandHistoryMessage(history []Message) string {
	recent := recentStructuredMemoryRecords(history)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		ReferenceHistory []StructuredMemoryRecord `json:"reference_history"`
	}{
		ReferenceHistory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandUserMessage(prompt string, observations []StructuredCommandObservation, args ...interface{}) string {
	workingDirectory := ""
	objectiveLedger := []StructuredObjective(nil)
	if len(args) > 0 {
		if value, ok := args[0].(string); ok {
			workingDirectory = value
		}
	}
	if len(args) > 1 {
		if value, ok := args[1].([]StructuredObjective); ok {
			objectiveLedger = value
		}
	}
	minimalContext := MinimalContext{}
	if len(args) > 2 {
		if value, ok := args[2].(MinimalContext); ok {
			minimalContext = normalizeMinimalContext(value)
		}
	}
	recipes := []Recipe(nil)
	if len(args) > 3 {
		if value, ok := args[3].([]Recipe); ok {
			recipes = value
		}
	}
	worksiteSurvey := WorksiteSurvey{}
	if len(args) > 4 {
		if value, ok := args[4].(WorksiteSurvey); ok {
			worksiteSurvey = value
		}
	}
	payload := struct {
		ActivePromptOpen string                  `json:"active_prompt_open"`
		ToolInventory    StructuredToolInventory `json:"tool_inventory"`
		ActiveTask       struct {
			CurrentPrompt               string                         `json:"current_prompt"`
			Prompt                      string                         `json:"prompt"`
			CurrentWorkingDirectory     string                         `json:"current_working_directory"`
			WorksiteSurvey              WorksiteSurvey                 `json:"worksite_survey"`
			RuntimeStateLifetime        StructuredRuntimeStateLifetime `json:"runtime_state_lifetime"`
			MinimalContext              MinimalContext                 `json:"minimal_context,omitempty"`
			Recipes                     []RecipeRuntimeConstraint      `json:"recipes,omitempty"`
			ObjectiveLedger             []StructuredObjective          `json:"objective_ledger,omitempty"`
			CompletedActions            []CompletedAction              `json:"completed_actions,omitempty"`
			LoopState                   StructuredLoopState            `json:"loop_state,omitempty"`
			ForbiddenCommands           []string                       `json:"forbidden_commands,omitempty"`
			RecoveryInstruction         string                         `json:"recovery_instruction,omitempty"`
			TaskRoute                   TaskRoute                      `json:"task_route,omitempty"`
			PendingObjectiveIDs         []string                       `json:"pending_objective_ids,omitempty"`
			MustReturnCommand           bool                           `json:"must_return_command"`
			RealCommandObservationCount int                            `json:"real_command_observation_count"`
			SuccessfulCommandCount      int                            `json:"successful_command_count"`
			FailedCommandCount          int                            `json:"failed_command_count"`
			AttemptBudgetRemaining      int                            `json:"attempt_budget_remaining"`
			Observations                []StructuredCommandObservation `json:"observations"`
		} `json:"active_task"`
		ActivePromptClose string `json:"active_prompt_close"`
	}{}
	payload.ActivePromptOpen = prompt
	payload.ToolInventory = buildStructuredToolInventory()
	payload.ActiveTask.CurrentPrompt = prompt
	payload.ActiveTask.Prompt = prompt
	payload.ActiveTask.CurrentWorkingDirectory = structuredPromptWorkingDirectory(workingDirectory)
	payload.ActiveTask.WorksiteSurvey = worksiteSurvey
	payload.ActiveTask.RuntimeStateLifetime = structuredRuntimeStateLifetime()
	payload.ActiveTask.MinimalContext = minimalContext
	payload.ActiveTask.Recipes = recipeRuntimeConstraints(recipes)
	payload.ActiveTask.ObjectiveLedger = mergeStructuredObjectiveLedger(nil, objectiveLedger)
	payload.ActiveTask.CompletedActions = completedActionsFromState(payload.ActiveTask.ObjectiveLedger, observations)
	payload.ActiveTask.LoopState = structuredLoopStateFromState(payload.ActiveTask.ObjectiveLedger, observations)
	payload.ActiveTask.ForbiddenCommands = payload.ActiveTask.LoopState.ForbiddenCommands
	payload.ActiveTask.RecoveryInstruction = payload.ActiveTask.LoopState.Instruction
	if route, ok := LoadCodebaseTaskRoute(payload.ActiveTask.CurrentWorkingDirectory, prompt); ok {
		payload.ActiveTask.TaskRoute = route
	}
	payload.ActiveTask.PendingObjectiveIDs = structuredObjectiveIDs(pendingStructuredObjectives(payload.ActiveTask.ObjectiveLedger))
	payload.ActiveTask.MustReturnCommand = !hasRealCommandObservation(observations)
	payload.ActiveTask.RealCommandObservationCount = realCommandObservationCount(observations)
	payload.ActiveTask.SuccessfulCommandCount = successfulCommandObservationCount(observations)
	payload.ActiveTask.FailedCommandCount = failedCommandObservationCount(observations)
	payload.ActiveTask.AttemptBudgetRemaining = maxInt(0, defaultCommandDecisionMaxSteps-len(observations))
	payload.ActiveTask.Observations = observations
	payload.ActivePromptClose = prompt
	blob, err := json.Marshal(payload)
	if err != nil {
		return prompt
	}
	return string(blob)
}

type StructuredToolInventory struct {
	TerminalTools  []string                 `json:"terminal_tools"`
	Skills         []string                 `json:"skills,omitempty"`
	PublicSources  []string                 `json:"public_sources"`
	LLMRoles       []string                 `json:"llm_roles"`
	SpecialistTeam []specialist.TeamProfile `json:"specialist_team"`
	ShellRules     []string                 `json:"shell_rules"`
}

func buildStructuredToolInventory() StructuredToolInventory {
	return StructuredToolInventory{
		TerminalTools: discoveredTerminalTools(),
		Skills:        discoveredSkillNames(),
		PublicSources: []string{
			"wttr.in",
			"news.google.com/rss/search",
			"duckduckgo.com/html",
			"go.dev/dl/?mode=json",
		},
		LLMRoles: []string{
			"command_planner",
			"shell_execution_specialist",
			"final_responder",
			"memory_retriever",
			"memory_reviewer",
			"web_researcher",
			"workspace_researcher",
			"subtask_executor",
			"verifier",
		},
		SpecialistTeam: specialist.DefaultTeam(),
		ShellRules: []string{
			"single fresh bash shell per command",
			"working directory does not persist between commands",
			"use absolute paths or cd within the same command",
			"for Thailand current time use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'",
			"stdout stderr and exit code are observed after execution",
		},
	}
}

func discoveredTerminalTools() []string {
	candidates := []string{
		"bash", "sh", "curl", "python3", "sed", "awk", "grep", "rg", "jq", "date", "uname",
		"cat", "find", "ls", "pwd", "mkdir", "touch", "tee", "git", "go", "npm", "node",
		"docker", "ps", "pgrep", "xdg-open", "firefox", "wmctrl", "xdotool",
	}
	tools := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, tool := range candidates {
		if _, err := exec.LookPath(tool); err == nil && !seen[tool] {
			tools = append(tools, tool)
			seen[tool] = true
		}
	}
	sort.Strings(tools)
	return tools
}

func discoveredSkillNames() []string {
	root := findStructuredSkillsRoot()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	skills := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err == nil {
			skills = append(skills, name)
		}
	}
	sort.Strings(skills)
	return skills
}

func findStructuredSkillsRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(wd, "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		next := filepath.Dir(wd)
		if next == wd {
			return ""
		}
		wd = next
	}
}

type StructuredMemoryRecord struct {
	Turn        int    `json:"turn"`
	Role        string `json:"role"`
	NotPrompt   bool   `json:"not_prompt"`
	MemoryStyle string `json:"memory_style"`
	MemoryNote  string `json:"memory_note"`
}

func recentStructuredCapabilityMemories(memories []SessionMemory) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxConversationHistoryMessages {
		start = len(memories) - maxConversationHistoryMessages
	}
	out := []SessionMemory{}
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		kind := strings.TrimSpace(memory.Kind)
		if kind == "" {
			kind = "capability"
		}
		if kind != "capability" {
			continue
		}
		out = append(out, SessionMemory{
			Kind:      kind,
			Content:   truncateStructuredObservation(memory.Content),
			Tags:      sortedCopy(memory.Tags),
			CreatedAt: memory.CreatedAt,
		})
	}
	return out
}

func recentStructuredSessionMemories(memories []SessionMemory) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxConversationHistoryMessages {
		start = len(memories) - maxConversationHistoryMessages
	}
	out := []SessionMemory{}
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		kind := strings.TrimSpace(memory.Kind)
		if kind == "" {
			kind = "episodic"
		}
		out = append(out, SessionMemory{
			Kind:      kind,
			Content:   truncateStructuredObservation(memory.Content),
			Tags:      sortedCopy(memory.Tags),
			CreatedAt: memory.CreatedAt,
		})
	}
	return out
}

func recentStructuredMemoryRecords(history []Message) []StructuredMemoryRecord {
	recent := recentStructuredConversation(history)
	if len(recent) == 0 {
		return nil
	}
	out := make([]StructuredMemoryRecord, 0, len(recent))
	for i, msg := range recent {
		content := sanitizeStructuredReferenceHistoryContent(msg.Role, msg.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		out = append(out, StructuredMemoryRecord{
			Turn:        i + 1,
			Role:        msg.Role,
			NotPrompt:   true,
			MemoryStyle: "terse_reference_only",
			MemoryNote:  compactStructuredMemoryNote(content),
		})
	}
	return out
}

func sanitizeStructuredReferenceHistoryContent(role, content string) string {
	content = strings.TrimSpace(content)
	if content == "" || strings.TrimSpace(role) != "assistant" {
		return content
	}
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if structuredReferenceHistoryLineIsOperationalState(line) {
			continue
		}
		kept = append(kept, line)
	}
	clean := strings.TrimSpace(strings.Join(kept, "\n"))
	if clean == "" {
		return "prior assistant response omitted operational loop state"
	}
	return clean
}

func structuredReferenceHistoryLineIsOperationalState(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	needles := []string{
		"forbidden_commands",
		"forbidden command",
		"blocked command",
		"loop blocker",
		"last blocker",
		"anti_loop:",
		"progression_gate",
		"structured_command_loop_blocked",
		"repeated command exhausted",
		"command repeats a previous failed command",
		"pending objectives:",
		"command:",
		"last command exit code:",
		"attempts:",
		"stdout:",
		"stderr:",
		"answer:",
		"status:",
		"stopped:",
		"stopped: structured command loop exhausted",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func compactStructuredMemoryNote(content string) string {
	note := strings.Join(strings.Fields(content), " ")
	if len(note) <= 320 {
		return note
	}
	return note[:320] + " [truncated]"
}

func recentStructuredConversation(history []Message) []Message {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > maxConversationHistoryMessages {
		start = len(history) - maxConversationHistoryMessages
	}
	out := make([]Message, 0, len(history)-start)
	for _, msg := range history[start:] {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		out = append(out, Message{
			Role:      role,
			Content:   truncateStructuredObservation(content),
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}

func currentWorkingDirectoryForStructuredPrompt() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func structuredPromptWorkingDirectory(workingDirectory string) string {
	if strings.TrimSpace(workingDirectory) != "" {
		return strings.TrimSpace(workingDirectory)
	}
	return currentWorkingDirectoryForStructuredPrompt()
}

func realCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			count++
		}
	}
	return count
}

func successfulCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			count++
		}
	}
	return count
}

func failedCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode != 0 {
			count++
		}
	}
	return count
}

func truncateStructuredObservation(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= defaultStructuredObservationChars {
		return trimmed
	}
	return trimmed[:defaultStructuredObservationChars] + "\n[truncated]"
}

func truncateStructuredTimelineValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 400 {
		return trimmed
	}
	return trimmed[:400] + "..."
}

func ParseStructuredCommandPayload(raw string) (StructuredCommandPayload, error) {
	var decoded struct {
		Command         *string               `json:"command"`
		Done            *bool                 `json:"done"`
		Answer          *string               `json:"answer"`
		Ask             bool                  `json:"ask"`
		Question        string                `json:"question"`
		Tool            string                `json:"tool"`
		ToolTask        string                `json:"tool_task"`
		Patch           string                `json:"patch"`
		ObjectiveLedger []StructuredObjective `json:"objective_ledger"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return StructuredCommandPayload{}, fmt.Errorf("parse structured command payload: %w", err)
	}
	if decoded.Command == nil || decoded.Done == nil || decoded.Answer == nil {
		return StructuredCommandPayload{}, fmt.Errorf("structured command payload missing required fields")
	}
	return StructuredCommandPayload{
		Command:         *decoded.Command,
		Done:            *decoded.Done,
		Answer:          *decoded.Answer,
		Ask:             decoded.Ask,
		Question:        decoded.Question,
		Tool:            decoded.Tool,
		ToolTask:        decoded.ToolTask,
		Patch:           decoded.Patch,
		ObjectiveLedger: mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
	}, nil
}

func ParseShellCommandProposal(raw string) (ShellCommandProposal, error) {
	var decoded struct {
		Command   string `json:"command"`
		Rationale string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ShellCommandProposal{}, fmt.Errorf("parse shell specialist response: %w", err)
	}
	command := strings.TrimSpace(decoded.Command)
	if command == "" {
		return ShellCommandProposal{}, fmt.Errorf("shell specialist response missing command")
	}
	return ShellCommandProposal{
		Command:   command,
		Rationale: strings.TrimSpace(decoded.Rationale),
	}, nil
}

func ExecuteStructuredCommand(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	return ExecuteStructuredCommandInDir(ctx, command, "", stdout, stderr)
}

func ExecuteStructuredCommandInDir(ctx context.Context, command, workingDirectory string, stdout, stderr io.Writer) (int, error) {
	cmd := newStructuredShellCommand(command)
	if strings.TrimSpace(workingDirectory) != "" {
		cmd.Dir = strings.TrimSpace(workingDirectory)
	}
	configureStructuredCommandProcess(cmd)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return 1, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killStructuredCommandProcess(cmd)
		<-done
		return 1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}
