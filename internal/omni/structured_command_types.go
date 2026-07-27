package omni

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const defaultCommandDecisionTimeout = 6 * time.Hour
const defaultCommandDecisionMaxSteps = 40
const defaultStructuredObservationChars = 2400
const defaultStructuredLLMRequestAttempts = 3
const defaultStructuredPlannerRepairAttempts = 2
const defaultShellSpecialistRepairAttempts = 2
const defaultShellSpecialistRepairTemperature = 0.25
const defaultEvaluatorPlannerRepairAttempts = 2
const maxStructuredLLMBackoff = 30 * time.Second
const defaultStructuredEvaluatorTimeout = defaultOllamaRequestTimeout
const maxRepeatedPrematureDoneRejections = 3
const defaultStructuredPlannerPromptBudgetChars = 100000
const defaultStructuredShellPromptBudgetChars = 14000
const defaultStructuredEvaluatorPromptBudgetChars = 16000
const defaultStructuredCompletionPromptBudgetChars = 18000

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
	ProofPlan       StructuredProofPlan   `json:"proof_plan,omitempty"`
}

type CommandDecisionResult struct {
	Command         string
	ExitCode        int
	Answer          string
	PartialProgress bool
	TaskMode        TaskMode
	TargetRoot      string
	Observations    []StructuredCommandObservation
	ObjectiveLedger []StructuredObjective
	WorkItems       []ObjectiveWorkItem
	ChildJobs       []ChildJob
	MinimalContext  MinimalContext
	StartedAt       time.Time
	FinishedAt      time.Time
	Elapsed         time.Duration
}

type StructuredCommandObservation struct {
	Step                 int      `json:"step"`
	CommandID            string   `json:"command_id,omitempty"`
	ChildJobID           string   `json:"child_job_id,omitempty"`
	ObjectiveID          string   `json:"objective_id,omitempty"`
	Command              string   `json:"command"`
	RejectedCommand      string   `json:"rejected_command,omitempty"`
	RejectedResponse     string   `json:"rejected_response,omitempty"`
	EvidenceKind         string   `json:"evidence_kind,omitempty"`
	VerifierID           string   `json:"verifier_id,omitempty"`
	GeneratedBy          string   `json:"generated_by,omitempty"`
	CheckedFiles         []string `json:"checked_files,omitempty"`
	CheckedPredicates    []string `json:"checked_predicates,omitempty"`
	EvaluationConfidence int      `json:"evaluation_confidence,omitempty"`
	EvaluationFeedback   string   `json:"evaluation_feedback,omitempty"`
	CapabilityMemory     string   `json:"capability_memory,omitempty"`
	ExitCode             int      `json:"exit_code"`
	Stdout               string   `json:"stdout"`
	Stderr               string   `json:"stderr"`
	Cached               bool     `json:"cached,omitempty"`
	CWD                  string   `json:"cwd,omitempty"`
	Attempt              int      `json:"attempt,omitempty"`
	StartedAt            string   `json:"started_at,omitempty"`
	FinishedAt           string   `json:"finished_at,omitempty"`
	Question             string   `json:"question,omitempty"`
	UserResponse         string   `json:"user_response,omitempty"`
}

type StructuredObjective struct {
	ID               string   `json:"id"`
	Description      string   `json:"description"`
	Status           string   `json:"status"`
	Kind             string   `json:"kind,omitempty"`
	Evidence         string   `json:"evidence,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
	Source           string   `json:"source,omitempty"`
	ParentObjective  string   `json:"parent_objective,omitempty"`
	Required         bool     `json:"required,omitempty"`
	Packages         []string `json:"packages,omitempty"`
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
	ValidationScope  string
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
	Step              int
	UserPrompt        string
	ToolTask          string
	ArchitectContract ImplementationArchitectContract
	RepairFeedback    string
	RepairAttempt     int
	RejectedCommand   string
	Observations      []StructuredCommandObservation
	CompletedActions  []CompletedAction
	LoopState         StructuredLoopState
	ProjectFileMap    ProjectFileMap
	SessionMemories   []SessionMemory
	WorksiteSurvey    WorksiteSurvey
}

type ShellCommandProposal struct {
	Command   string
	Rationale string
}

type ShellCommandSpecialist interface {
	ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error)
}

type CodeContentSpecialistInput struct {
	Step               int
	UserPrompt         string
	ArchitectContract  ImplementationArchitectContract
	WorkItem           ArchitectWorkItem
	ExistingContent    string
	TestFirst          bool
	RepairFeedback     string
	RepairAttempt      int
	RejectedContent    string
	IntegrationContext string
	ProjectFileMap     ProjectFileMap
	Observations       []StructuredCommandObservation
	SessionMemories    []SessionMemory
	WorksiteSurvey     WorksiteSurvey
}

type CodeContentProposal struct {
	Content   string `json:"content"`
	Rationale string `json:"rationale"`
}

type CodeContentFileContract struct {
	Path        string   `json:"path"`
	Operation   string   `json:"operation"`
	Role        string   `json:"role"`
	Language    string   `json:"language"`
	MustInclude []string `json:"must_include,omitempty"`
	MustAvoid   []string `json:"must_avoid,omitempty"`
}

type CodeContentSpecialist interface {
	GenerateCodeContent(ctx context.Context, input CodeContentSpecialistInput) (CodeContentProposal, error)
}

type CursorArchitectAgentInput struct {
	Step              int
	UserPrompt        string
	ToolTask          string
	ArchitectContract ImplementationArchitectContract
	Packet            CursorImplementationPacket
	Observations      []StructuredCommandObservation
	SessionMemories   []SessionMemory
	WorksiteSurvey    WorksiteSurvey
	Workspace         string
}

type CursorImplementationPacket struct {
	Task               string                    `json:"task"`
	Mode               string                    `json:"mode"`
	Workspace          string                    `json:"workspace"`
	TargetRoot         string                    `json:"target_root"`
	Worksite           CursorPacketWorksite      `json:"worksite"`
	EditSurface        []string                  `json:"edit_surface"`
	ReadOnlyContext    []string                  `json:"read_only_context,omitempty"`
	Objectives         []string                  `json:"objectives"`
	ProofContract      CursorPacketProofContract `json:"proof_contract"`
	Forbidden          []string                  `json:"forbidden"`
	ReturnRequirements []string                  `json:"return"`
	PreparedContext    []string                  `json:"prepared_context,omitempty"`
}

type CursorPacketWorksite struct {
	ProjectState   string   `json:"project_state,omitempty"`
	PackageManager string   `json:"package_manager,omitempty"`
	Frameworks     []string `json:"frameworks,omitempty"`
}

type CursorPacketProofContract struct {
	Commands           []string `json:"commands,omitempty"`
	ArtifactChecks     []string `json:"artifact_checks,omitempty"`
	EvidencePredicates []string `json:"evidence_predicates,omitempty"`
}

type CursorArchitectAgentResult struct {
	Summary string `json:"summary"`
	AgentID string `json:"agent_id"`
	RunID   string `json:"run_id"`
	Output  string `json:"output"`
}

type CursorArchitectAgent interface {
	RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error)
}

type ExternalAgentSessionProvider interface {
	NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error)
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
	interpretation, parseErr := ParsePromptInterpretation(resp.Content)
	if parseErr == nil {
		return interpretation, nil
	}
	return PromptInterpretation{}, fmt.Errorf("parse prompt interpreter response: %w", parseErr)
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

type OllamaCodeContentSpecialist struct {
	Client CommandDecisionClient
}

func NewOllamaCodeContentSpecialist(client CommandDecisionClient) OllamaCodeContentSpecialist {
	return OllamaCodeContentSpecialist{Client: client}
}

func (s OllamaCodeContentSpecialist) GenerateCodeContent(ctx context.Context, input CodeContentSpecialistInput) (CodeContentProposal, error) {
	if s.Client == nil {
		return CodeContentProposal{}, fmt.Errorf("code content specialist client is required")
	}
	resp, err := s.Client.ChatRaw(ctx, buildCodeContentSpecialistRequest(input))
	if err != nil {
		return CodeContentProposal{}, err
	}
	return ParseCodeContentProposal(resp.Content)
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

type UserAssistanceInput struct {
	Step       int
	Kind       string
	Command    string
	Reason     string
	Packages   []string
	UserPrompt string
}

type UserAssistanceQuestion struct {
	Question string
}

type UserAssistanceSpecialist interface {
	BuildUserAssistanceQuestion(ctx context.Context, input UserAssistanceInput) (UserAssistanceQuestion, error)
}

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
	SessionMemories          []SessionMemory
	PrepContext              PrepContextBundle
	CurrentWorkingDirectory  string
	Recipes                  []Recipe
	PromptInterpreter        PromptInterpreter
	ContextSummarizer        ContextSummarizer
	CompletionChecker        CompletionChecker
	Evaluator                StructuredLLMResponseEvaluator
	EvaluatorThreshold       int
	ShellSpecialist          ShellCommandSpecialist
	CodeContentSpecialist    CodeContentSpecialist
	CursorArchitectAgent     CursorArchitectAgent
	CodexArchitectAgent      CursorArchitectAgent
	UserAssistanceSpecialist UserAssistanceSpecialist
	EnableCommandCache       bool
	CommandCacheRoot         string
	TaskMode                 TaskMode
	ThinkingService          ThinkingService
	ThoughtTurnID            string
}
