package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
	"github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/workspace"
)

const stepControlPollInterval = 300 * time.Millisecond

type ModelRouting struct {
	Default    string
	Fast       string
	Glue       string
	Reasoning  string
	Tagging    string
	Plan       string
	Analyze    string
	Response   string
	Search     string
	Memory     string
	Specialist map[string]string
}

type stepCompleteFunc func(context.Context, queue.CompleteStepCommand) error

type nativeV3StepRunner func(context.Context, *model.ClaimedStep, map[string]string, string) error

type WorkspaceSettings struct {
	Enabled       bool
	Root          string
	HostRoot      string
	MaxFiles      int
	ContextBudget int
}

type Options struct {
	WorkerCount            int
	FragmentConcurrency    int
	PollInterval           time.Duration
	RetrievalLimit         int
	ContextBudget          int
	InferenceContextTokens int
	EmbeddingProvider      string
	EmbeddingModel         string
	Models                 ModelRouting
	Workspace              WorkspaceSettings
	SkillsRoot             string
	Logger                 *log.Logger
	OnJobFinished          func(jobID int64)
	OnJobOutput            func(jobID int64, delta string)
}

type Service struct {
	repo                   *queue.Repository
	llm                    llm.Client
	webSearch              *websearch.Service
	workerCount            int
	fragmentConcurrency    int
	pollInterval           time.Duration
	retrievalLimit         int
	contextBudget          int
	inferenceContextTokens int
	embeddingProvider      string
	embeddingModel         string
	models                 ModelRouting
	workspace              *workspace.Service
	repositoryIndex        repositoryIndexRefresher
	repositoryRetrieval    repositoryEvidenceBuilder
	workspaceHostRoot      string
	bootstrapRegistry      *specialists.Registry
	skillMu                sync.RWMutex
	v3Registry             *specialists.Registry
	v3Tools                *tools.Registry
	completeStep           stepCompleteFunc
	nativeV3Runner         nativeV3StepRunner
	logger                 *log.Logger
	onJobFinished          func(jobID int64)
	onJobOutput            func(jobID int64, delta string)
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

	workspaceSvc, err := workspace.New(
		opts.Workspace.Enabled,
		opts.Workspace.Root,
		opts.Workspace.MaxFiles,
		opts.Workspace.ContextBudget,
	)
	if err != nil {
		return nil, fmt.Errorf("configure workspace scanner: %w", err)
	}
	repositoryIndex, err := repositoryindex.New(repo)
	if err != nil {
		return nil, fmt.Errorf("configure repository indexer: %w", err)
	}
	repositoryRetrieval, err := repositoryretrieval.New(repo)
	if err != nil {
		return nil, fmt.Errorf("configure repository retrieval: %w", err)
	}

	skillRegistry, err := specialists.LoadRegistry(opts.SkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("load specialist registry from %q: %w", opts.SkillsRoot, err)
	}
	if len(skillRegistry.Specs) == 0 {
		return nil, fmt.Errorf("load specialist registry from %q: no specialist specs found", opts.SkillsRoot)
	}
	var completeStep stepCompleteFunc
	if repo != nil {
		completeStep = repo.CompleteStep
	}
	svc := &Service{
		repo:                   repo,
		llm:                    llmClient,
		webSearch:              webSearch,
		workerCount:            opts.WorkerCount,
		fragmentConcurrency:    opts.FragmentConcurrency,
		pollInterval:           opts.PollInterval,
		retrievalLimit:         opts.RetrievalLimit,
		contextBudget:          opts.ContextBudget,
		inferenceContextTokens: opts.InferenceContextTokens,
		embeddingProvider:      opts.EmbeddingProvider,
		embeddingModel:         opts.EmbeddingModel,
		models:                 opts.Models,
		workspace:              workspaceSvc,
		repositoryIndex:        repositoryIndex,
		repositoryRetrieval:    repositoryRetrieval,
		workspaceHostRoot:      opts.Workspace.HostRoot,
		bootstrapRegistry:      skillRegistry,
		completeStep:           completeStep,
		logger:                 opts.Logger,
		onJobFinished:          opts.OnJobFinished,
		onJobOutput:            opts.OnJobOutput,
	}
	if repo != nil && completeStep != nil {
		svc.completeStep = svc.wrapStepCompleter(completeStep)
	}
	svc.nativeV3Runner = svc.runNativeV3Step
	svc.v3Tools = newV3ToolRegistry(svc)
	return svc, nil
}

func (s *Service) wrapStepCompleter(complete stepCompleteFunc) stepCompleteFunc {
	if complete == nil {
		return nil
	}
	return func(ctx context.Context, command queue.CompleteStepCommand) error {
		err := complete(ctx, command)
		if err == nil {
			s.notifyJobFinishedForStep(ctx, command.StepID)
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
	details, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
		go s.onJobFinished(jobID)
	}
}

func (s *Service) skillSpec(id string) (specialists.Spec, bool) {
	if s == nil {
		return specialists.Spec{}, false
	}
	s.skillMu.RLock()
	defer s.skillMu.RUnlock()
	if s.v3Registry == nil {
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

func (s *Service) skillPreferredModel(id, fallback string, routing ModelRouting) string {
	spec, ok := s.skillSpec(id)
	if !ok || len(spec.PreferredModel) == 0 {
		return fallback
	}
	for _, preference := range spec.PreferredModel {
		if modelName := resolveSkillModelPreference(preference, routing); modelName != "" {
			return modelName
		}
	}
	return fallback
}

func resolveSkillModelPreference(preference string, routing ModelRouting) string {
	switch strings.ToLower(strings.TrimSpace(preference)) {
	case "default":
		return strings.TrimSpace(routing.Default)
	case "fast":
		return strings.TrimSpace(routing.Fast)
	case "reasoning", "analyze", "analyzer":
		return strings.TrimSpace(routing.Analyze)
	case "planner", "plan":
		return strings.TrimSpace(routing.Plan)
	case "response", "responder":
		return strings.TrimSpace(routing.Response)
	case "search":
		return strings.TrimSpace(routing.Search)
	case "memory":
		return strings.TrimSpace(routing.Memory)
	default:
		if modelName := strings.TrimSpace(routing.Specialist[strings.TrimSpace(preference)]); modelName != "" {
			return modelName
		}
		return strings.TrimSpace(preference)
	}
}

const staleV3StepLease = 75 * time.Second

func (s *Service) Start(ctx context.Context) error {
	if err := s.refreshSkillRegistry(ctx); err != nil {
		return fmt.Errorf("initialize authoritative worker skill registry: %w", err)
	}
	if err := s.repo.CheckStaleV3StepLeases(ctx, time.Now().Add(-staleV3StepLease)); err != nil {
		return fmt.Errorf("validate V3 worker leases: %w", err)
	}
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
	return nil
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
			failCommand, identityErr := failClaimedStepCommand(claim, err.Error())
			if identityErr != nil {
				s.logger.Printf("worker=%s job=%d step=%d failure identity error: %v", workerID, claim.Job.ID, claim.Step.ID, identityErr)
				continue
			}
			failErr := s.repo.FailStep(ctx, failCommand)
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
	if _, err := modelRoutingFromJobMetadata(claim.Job.Metadata, s.models); err != nil {
		return err
	}

	action := strings.ToLower(strings.TrimSpace(claim.Step.Action))
	contexts := contextsToMap(claim.Contexts)
	stepCtx, stop := s.watchStepControl(ctx, claim.Job.ID, claim.Step.ID)
	defer stop()

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
	return fmt.Errorf("unsupported worker action %q", action)
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
			if stepStatus == model.StepStatusCanceled {
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
	return false
}
