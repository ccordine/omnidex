package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	runtimev3 "github.com/gryph/omnidex/internal/runtime/v3"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
	"github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/workspace"
)

var webSearchKeywordPattern = regexp.MustCompile(`\b(search|find|latest|today|current|news|price|weather|time|release|update)\b`)
var complexityKeywordPattern = regexp.MustCompile(`\b(architecture|design|refactor|migrate|tradeoff|root cause|investigate|debug|security|performance|optimize|strategy|plan)\b`)
var bracketedMatchPattern = regexp.MustCompile(`\[\d+\]`)
var codeKeywordPattern = regexp.MustCompile(`\b(code|project|repo|repository|file|files|directory|directories|package|dependency|dependencies|compile|build|test|refactor|fix|bug)\b`)
var memoryLookbackPattern = regexp.MustCompile(`(?i)\b(older|oldest|earlier|previous|before|back then|history|historical|timeline|chronological|long[-\s]?term|past|from last|used to|think back|look back)\b`)
var explicitHistoricalRecallPattern = regexp.MustCompile(`(?i)\b(do you remember|remember when|remember what|what did (i|we|you)\s+(say|ask|mention|discuss)|earlier in (this|our)\s+chat|from (earlier|before|last time|previous (chat|conversation|session))|previous (chat|conversation|session)|recall)\b`)
var relativeTimePattern = regexp.MustCompile(`(?i)\b(today|tomorrow|yesterday|tonight|now|right now|currently|current|as of|latest|recent|most recent|this week|this month|this year|this quarter|this morning|this evening)\b`)
var explicitWebRequestPattern = regexp.MustCompile(`(?i)\b(web\s*search|search\s+the\s+web|search\s+online|look\s+up|lookup|browse\s+the\s+web|browse\s+online|check\s+online|internet\s+search|google\s+it)\b`)
var staleMemoryPattern = regexp.MustCompile(`(?i)\b(memory|context|cached)\b.*\b(stale|outdated|out\s+of\s+date|old|wrong|incorrect|inaccurate)\b|\b(stale|outdated|out\s+of\s+date|wrong|incorrect|inaccurate)\b.*\b(memory|context|cached)\b|\b(ignore|don't use|do not use|skip)\s+(memory|cached context)\b`)
var explicitFreshContextPattern = regexp.MustCompile(`(?i)\b(fresh\s+(thread|context)|clean\s+slate|from\s+scratch|start\s+over|new\s+session|ignore\s+(prior|previous|earlier|historical|old)\s+(context|history|conversation)|do\s+not\s+use\s+(prior|previous|earlier|historical|old)\s+(context|history|conversation))\b`)
var localClockQuestionPattern = regexp.MustCompile(`(?i)\b(what(?:'s| is)?\s+the?\s*time|what time is it|current time|what(?:'s| is)?\s+today(?:'s)?\s+date|what day is it|today(?:'s)?\s+date)\b`)
var dateAnchorPattern = regexp.MustCompile(`(?i)\b(\d{4}-\d{2}-\d{2}|20\d{2}|jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\b`)
var searchPromptArtifactPattern = regexp.MustCompile(`(?i)\b(current[_ ]?time[_ ]?context|autonomy[_ ]?mode|user[_ ]?instruction|user[_ ]?feedback|retrieved[_ ]?memory)\b`)
var duplicateAsOfPattern = regexp.MustCompile(`(?i)\bas of\s+as of\b`)
var needInputPattern = regexp.MustCompile(`(?im)^\s*need_input:\s*(.+)$`)
var riskyActionPattern = regexp.MustCompile(`(?i)(rm\s+-rf|git\s+reset\s+--hard|drop\s+table|truncate\s+table|delete\s+from|mkfs|dd\s+if=|chmod\s+777|shutdown|destroy|wipe|production)`)
var testLinePattern = regexp.MustCompile(`(?im)^(.*test.*)$`)
var skipTestsPattern = regexp.MustCompile(`(?i)\b(skip|without|omit|don't run|do not run|no)\s+(all\s+)?tests?\b`)
var verifyStatusPattern = regexp.MustCompile(`(?i)^(pass|retry|blocked)$`)
var sourceSectionPattern = regexp.MustCompile(`(?im)^\s*(?:#+\s*)?sources?\s*:?`)
var tokenWordPattern = regexp.MustCompile(`[a-z0-9]{3,}`)
var backtickedTokenPattern = regexp.MustCompile("`([^`\\n]+)`")
var filePathTokenPattern = regexp.MustCompile(`(?i)\b[a-z0-9._/\-]+\.[a-z0-9]{1,16}\b`)
var namedTypedFilePattern = regexp.MustCompile(`(?i)\b(?:create|make|touch|write)\s+(?:me\s+)?(?:a\s+|an\s+)?(?:new\s+)?([a-z0-9][a-z0-9._/\-]*)\s+(html|css|js|javascript|json|md|markdown|txt|text)\s+file\b`)
var codeOnlyPreferencePattern = regexp.MustCompile(`(?i)\b(code[-\s]?only|only (?:return|output|respond with)\s+code|just code|raw file content|raw code|no backticks|no markdown|without markdown|no explanations?|no commentary|no prose|no templating)\b`)
var executionClaimPattern = regexp.MustCompile(`(?i)\b(i|we)\s+(ran|executed|installed|deployed|committed|merged|modified|edited|deleted|removed|updated|applied)\b`)
var webExecutionClaimPattern = regexp.MustCompile(`(?i)\b(i|we)\s+(searched|looked up|browsed|checked)\s+(the\s+)?(web|internet|online)\b`)
var webEvidenceClaimPattern = regexp.MustCompile(`(?i)\b(according to|based on)\s+(the\s+)?(web|internet|online|search results?)\b`)

const stepControlPollInterval = 300 * time.Millisecond
const stepEventWriteTimeout = 2 * time.Second
const verifyDefaultIterations = 2
const verifyMaxIterations = 4
const verifyDefaultTestTimeoutSeconds = 240
const verifyMaxCommandOutputChars = 2800
const defaultVerificationPasses = 2
const maxVerificationPasses = 2
const defaultHallucinationRetryLimit = 2
const maxHallucinationRetryLimit = 6
const defaultOllamaRestartTimeout = 20 * time.Second
const maxOllamaRestartOutputChars = 600
const maxAutoVerifyReplans = 1
const autoVerifyReplanMarker = "auto_verify_replan"
const recentConversationTurnLimit = 8
const recentConversationContextBudget = 2200
const recentConversationTurnBudget = 420
const defaultPlanningPasses = 3
const maxPlanningPasses = 5
const maxMemoryRetrievalLimit = 64
const maxRelatedMemoryTags = 12

type verificationOutcome struct {
	Status               string   `json:"status"`
	Confidence           float64  `json:"confidence"`
	Summary              string   `json:"summary"`
	Gaps                 []string `json:"gaps"`
	CannotCompleteReason string   `json:"cannot_complete_reason"`
}

type verificationActionAudit struct {
	Report          string
	MissingRequired []string
}

type testDirective struct {
	Skip  bool
	Notes []string
	Focus map[string]struct{}
}

type testCommand struct {
	Family string
	Name   string
	Args   []string
}

type testResult struct {
	Command  string
	Family   string
	Passed   bool
	Skipped  bool
	TimedOut bool
	Duration time.Duration
	ExitCode int
	Output   string
	Reason   string
}

type testReport struct {
	Root         string
	Notes        []string
	Attempted    int
	Passed       int
	Failed       int
	Skipped      int
	Commands     []testResult
	NotRunReason string
}

type tournamentLeafSummary struct {
	Index      int
	Relevant   bool
	Confidence int
	Summary    string
	Chunk      string
	Verified   bool
	Supported  string
}

type tournamentReport struct {
	Source         string
	RawChars       int
	LeafChunks     int
	SelectedLeaves int
	VerifiedLeaves int
	Rounds         int
	OutputChars    int
}

type planCandidateScore struct {
	Index  int
	Score  int
	Reason string
}

type ModelRouting struct {
	Default    string
	Fast       string
	Reasoning  string
	Tagging    string
	Plan       string
	Analyze    string
	Response   string
	Search     string
	Memory     string
	Specialist map[string]string
}

type CognitionSettings struct {
	StopOnSufficientContext bool
	SufficientContextChars  int
	MemoryInferenceEnabled  bool
	MemoryInferenceMaxItems int
}

type TournamentSettings struct {
	Enabled       bool
	ChunkChars    int
	SummaryChars  int
	MaxRounds     int
	VerifySupport bool
}

type WorkspaceSettings struct {
	Enabled       bool
	Root          string
	MaxFiles      int
	ContextBudget int
}

type Options struct {
	WorkerCount             int
	PollInterval            time.Duration
	RetrievalLimit          int
	ContextBudget           int
	InferenceContextTokens  int
	Models                  ModelRouting
	Cognition               CognitionSettings
	Tournament              TournamentSettings
	Workspace               WorkspaceSettings
	HallucinationRetryLimit int
	OllamaRestartCommand    string
	OllamaRestartTimeout    time.Duration
	OllamaBaseURL           string
	V3Enabled               bool
	SkillsRoot              string
	Logger                  *log.Logger
	OnJobFinished           func(jobID int64)
	OnJobOutput             func(jobID int64, delta string)
}

type Service struct {
	repo                    *queue.Repository
	llm                     llm.Client
	webSearch               *websearch.Service
	workerCount             int
	pollInterval            time.Duration
	retrievalLimit          int
	contextBudget           int
	inferenceContextTokens  int
	models                  ModelRouting
	cognition               CognitionSettings
	tournament              TournamentSettings
	workspace               *workspace.Service
	hallucinationRetryLimit int
	ollamaRestartCommand    string
	ollamaRestartTimeout    time.Duration
	ollamaBaseURL           string
	v3Enabled               bool
	v3SkillsRoot            string
	v3Registry              *specialists.Registry
	v3Tools                 *tools.Registry
	v3Engine                *runtimev3.Engine
	codingEngine            codingWorkflowRunner
	completeStep            stepCompleteFunc
	nativeV3Runner          nativeV3StepRunner
	agentRuntimeRunner      agentRuntimeStepRunner
	logger                  *log.Logger
	onJobFinished           func(jobID int64)
	onJobOutput             func(jobID int64, delta string)
}

func New(
	repo *queue.Repository,
	llmClient llm.Client,
	webSearch *websearch.Service,
	opts Options,
) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("worker repository is required")
	}
	if llmClient == nil {
		return nil, fmt.Errorf("worker LLM client is required")
	}
	if err := validateWorkerOptions(opts); err != nil {
		return nil, fmt.Errorf("invalid worker options: %w", err)
	}
	opts = normalizeWorkerOptions(opts)

	workspaceSvc := workspace.New(
		opts.Workspace.Enabled,
		opts.Workspace.Root,
		opts.Workspace.MaxFiles,
		opts.Workspace.ContextBudget,
	)

	var skillRegistry *specialists.Registry
	if opts.V3Enabled {
		registry, err := specialists.LoadRegistry(opts.SkillsRoot)
		if err != nil {
			return nil, fmt.Errorf("load V3 skill registry from %q: %w", opts.SkillsRoot, err)
		}
		if len(registry.Specs) == 0 {
			return nil, fmt.Errorf("load V3 skill registry from %q: no specialist specs found", opts.SkillsRoot)
		}
		skillRegistry = registry
	}

	var v3Engine *runtimev3.Engine
	if repo != nil {
		v3Engine = &runtimev3.Engine{Writer: repo}
	}
	var completeStep stepCompleteFunc
	if repo != nil {
		completeStep = repo.CompleteStep
	}
	svc := &Service{
		repo:                    repo,
		llm:                     llmClient,
		webSearch:               webSearch,
		workerCount:             opts.WorkerCount,
		pollInterval:            opts.PollInterval,
		retrievalLimit:          opts.RetrievalLimit,
		contextBudget:           opts.ContextBudget,
		inferenceContextTokens:  opts.InferenceContextTokens,
		models:                  opts.Models,
		cognition:               opts.Cognition,
		tournament:              opts.Tournament,
		workspace:               workspaceSvc,
		hallucinationRetryLimit: opts.HallucinationRetryLimit,
		ollamaRestartCommand:    opts.OllamaRestartCommand,
		ollamaRestartTimeout:    opts.OllamaRestartTimeout,
		ollamaBaseURL:           opts.OllamaBaseURL,
		v3Enabled:               opts.V3Enabled,
		v3SkillsRoot:            opts.SkillsRoot,
		v3Registry:              skillRegistry,
		v3Engine:                v3Engine,
		codingEngine:            nil,
		completeStep:            completeStep,
		logger:                  opts.Logger,
		onJobFinished:           opts.OnJobFinished,
		onJobOutput:             opts.OnJobOutput,
	}
	if repo != nil && completeStep != nil {
		svc.completeStep = svc.wrapStepCompleter(completeStep)
	}
	svc.nativeV3Runner = svc.runNativeV3Step
	svc.agentRuntimeRunner = svc.runAgentRuntimeStep
	if svc.v3Enabled {
		svc.v3Tools = newV3ToolRegistry(svc)
	}
	return svc, nil
}

func (s *Service) wrapStepCompleter(complete stepCompleteFunc) stepCompleteFunc {
	if complete == nil {
		return nil
	}
	return func(ctx context.Context, stepID int64, output, contextKey, contextValue string) error {
		err := complete(ctx, stepID, output, contextKey, contextValue)
		if err == nil {
			s.notifyJobFinishedForStep(ctx, stepID)
		}
		return err
	}
}

func (s *Service) notifyJobFinishedForStep(ctx context.Context, stepID int64) {
	if s.onJobFinished == nil || s.repo == nil || stepID <= 0 {
		return
	}
	jobID, err := s.repo.JobIDForStep(ctx, stepID)
	if err != nil || jobID <= 0 {
		return
	}
	s.notifyJobFinishedForJob(ctx, jobID)
}

func (s *Service) notifyJobFinishedForJob(ctx context.Context, jobID int64) {
	if s.onJobFinished == nil || s.repo == nil || jobID <= 0 {
		return
	}
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
		go s.onJobFinished(jobID)
	}
}

func (s *Service) skillSpec(id string) (specialists.Spec, bool) {
	if s == nil || s.v3Registry == nil {
		return specialists.Spec{}, false
	}
	spec, ok := s.v3Registry.Specs[strings.TrimSpace(id)]
	return spec, ok
}

func (s *Service) skillInstructions(id string) string {
	spec, ok := s.skillSpec(id)
	if !ok {
		return ""
	}
	return strings.TrimSpace(spec.Instructions)
}

func (s *Service) skillPreferredModel(id string, fallback string) string {
	spec, ok := s.skillSpec(id)
	if !ok || len(spec.PreferredModel) == 0 {
		return fallback
	}
	for _, preference := range spec.PreferredModel {
		if modelName := s.resolveSkillModelPreference(preference); modelName != "" {
			return modelName
		}
	}
	return fallback
}

func (s *Service) resolveSkillModelPreference(preference string) string {
	switch strings.ToLower(strings.TrimSpace(preference)) {
	case "default":
		return strings.TrimSpace(s.models.Default)
	case "fast":
		return strings.TrimSpace(s.models.Fast)
	case "reasoning", "analyze", "analyzer":
		return strings.TrimSpace(s.models.Analyze)
	case "planner", "plan":
		return strings.TrimSpace(s.models.Plan)
	case "response", "responder":
		return strings.TrimSpace(s.models.Response)
	case "search":
		return strings.TrimSpace(s.models.Search)
	case "memory":
		return strings.TrimSpace(s.models.Memory)
	default:
		if modelName := strings.TrimSpace(s.models.Specialist[strings.TrimSpace(preference)]); modelName != "" {
			return modelName
		}
		return strings.TrimSpace(preference)
	}
}

func (s *Service) v3Active() bool {
	return s != nil && s.v3Enabled && s.repo != nil
}

func (s *Service) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < s.workerCount; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker-%d", i+1)
		go func(id string) {
			defer wg.Done()
			s.run(ctx, id)
		}(workerID)
	}

	<-ctx.Done()
	wg.Wait()
}

func (s *Service) run(ctx context.Context, workerID string) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		claim, err := s.repo.ClaimNextStep(ctx, workerID)
		if err != nil {
			s.logger.Printf("worker=%s claim error: %v", workerID, err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}

		if claim == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}

		phase := pipelinePhaseForAction(claim.Step.Action)
		stepRole := specialist.ForPipelineAction(claim.Step.Action)
		s.emitStepContext(claim.Step.ID, "phase", phase)
		s.emitStepContext(claim.Step.ID, "specialist_role", strings.Join(specialist.DetailLines(stepRole), "\n"))
		s.emitStepEvent(claim.Step.ID, "step_start", fmt.Sprintf("phase=%s action=%s worker=%s specialist=%s", phase, claim.Step.Action, workerID, strings.TrimSpace(stepRole.ID)))
		if err := s.processStep(ctx, claim); err != nil {
			if s.skipFailureForControlledCancel(ctx, workerID, claim, err) {
				continue
			}
			s.emitStepEvent(claim.Step.ID, "step_error", err.Error())
			s.logger.Printf("worker=%s job=%d step=%d action=%s failed: %v", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action, err)
			failErr := s.repo.FailStep(ctx, claim.Step.ID, err.Error())
			if failErr != nil {
				s.logger.Printf("worker=%s job=%d step=%d fail update error: %v", workerID, claim.Job.ID, claim.Step.ID, failErr)
			} else {
				s.notifyJobFinishedForJob(ctx, claim.Job.ID)
			}
			continue
		}
		s.emitStepEvent(claim.Step.ID, "step_complete", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
	}
}

func (s *Service) processStep(ctx context.Context, claim *model.ClaimedStep) error {
	restoreModels, err := withJobModelRouting(s, claim.Job)
	if err != nil {
		return err
	}
	defer restoreModels()

	action := strings.ToLower(strings.TrimSpace(claim.Step.Action))
	contexts := contextsToMap(claim.Contexts)
	if isCodingJob(claim.Job) {
		if action != "coding_workflow" {
			return fmt.Errorf("coding pipeline cannot run non-coding action %q", action)
		}
		return s.runCodingWorkflowStep(ctx, claim, contexts)
	}

	stepCtx, stop := s.watchStepControl(ctx, claim.Job.ID, claim.Step.ID)
	defer stop()

	freshContextOnly := shouldBypassHistoricalContext(claim.Job.Instruction, contexts["user_feedback"])
	if shouldAttachRecentConversation(claim.Job, action) {
		if freshContextOnly {
			s.emitStepEvent(claim.Step.ID, "recent_conversation_skipped", "reason=fresh_context_requested")
		} else if recent := s.recentConversationContext(stepCtx, claim.Job); strings.TrimSpace(recent) != "" {
			contexts["recent_conversation"] = recent
			s.emitStepContext(claim.Step.ID, "recent_conversation", trimForBudget(recent, 1800))
		}
	}

	if strings.HasPrefix(action, "v3_") {
		if s.nativeV3Runner != nil {
			return s.nativeV3Runner(stepCtx, claim, contexts, action)
		}
		return s.runNativeV3Step(stepCtx, claim, contexts, action)
	}
	if action == "external_agent_execute" {
		return s.runExternalAgentStep(stepCtx, claim, contexts)
	}
	if action == "data_source_query" {
		return s.runDataSourceQueryStep(stepCtx, claim)
	}
	if action == "data_source_explore" {
		return s.runDataSourceExploreStep(stepCtx, claim)
	}
	if action == "project_debugger" {
		return s.runProjectDebuggerStep(stepCtx, claim)
	}
	if action == "scrum_card_llm" {
		return s.runScrumCardLLMStep(stepCtx, claim)
	}
	// Runtime v2: keep the queue contract/action names stable while executing
	// through a simpler, stage-driven orchestrator.
	if s.agentRuntimeRunner != nil {
		return s.agentRuntimeRunner(stepCtx, claim, contexts, action)
	}
	return s.runAgentRuntimeStep(stepCtx, claim, contexts, action)
}

func (s *Service) watchStepControl(ctx context.Context, jobID, stepID int64) (context.Context, func()) {
	stepCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(stepControlPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stepCtx.Done():
				return
			case <-ticker.C:
			}

			jobStatus, stepStatus, err := s.repo.GetStepRuntimeState(stepCtx, jobID, stepID)
			if err != nil {
				if errors.Is(err, context.Canceled) || stepCtx.Err() != nil {
					return
				}
				s.logger.Printf("job=%d step=%d control poll error: %v", jobID, stepID, err)
				continue
			}

			if jobStatus == model.JobStatusCanceled {
				cancel()
				return
			}
			if stepStatus == model.StepStatusPending || stepStatus == model.StepStatusCanceled {
				cancel()
				return
			}
		}
	}()

	stop := func() {
		cancel()
		<-done
	}

	return stepCtx, stop
}

func (s *Service) skipFailureForControlledCancel(ctx context.Context, workerID string, claim *model.ClaimedStep, err error) bool {
	if !errors.Is(err, context.Canceled) {
		return false
	}
	if ctx.Err() != nil {
		return true
	}

	jobStatus, stepStatus, stateErr := s.repo.GetStepRuntimeState(ctx, claim.Job.ID, claim.Step.ID)
	if stateErr != nil {
		s.logger.Printf("worker=%s job=%d step=%d cancel-state lookup error: %v", workerID, claim.Job.ID, claim.Step.ID, stateErr)
		return false
	}

	if jobStatus == model.JobStatusCanceled || stepStatus == model.StepStatusCanceled {
		s.logger.Printf("worker=%s job=%d step=%d action=%s canceled", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action)
		s.emitStepEvent(claim.Step.ID, "step_canceled", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		return true
	}
	if stepStatus == model.StepStatusPending {
		s.logger.Printf("worker=%s job=%d step=%d action=%s interrupted and re-queued", workerID, claim.Job.ID, claim.Step.ID, claim.Step.Action)
		s.emitStepEvent(claim.Step.ID, "step_interrupted", fmt.Sprintf("action=%s worker=%s", claim.Step.Action, workerID))
		return true
	}

	return false
}

func (s *Service) emitStepEvent(stepID int64, eventType, message string) {
	payload := strings.TrimSpace(strings.Join([]string{
		"time=" + time.Now().UTC().Format(time.RFC3339),
		"event=" + strings.TrimSpace(eventType),
		strings.TrimSpace(message),
	}, " "))
	s.emitStepContext(stepID, "event", payload)
	if s.repo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.repo.RecordTelemetryStepEvent(ctx, stepID, eventType, message); err != nil {
			s.logger.Printf("step=%d telemetry event=%s write error: %v", stepID, eventType, err)
		}
	}
}
