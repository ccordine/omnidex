package worker

import (
	"context"
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
	if _, err := llm.RequireExactPreparedContract(llmClient); err != nil {
		return nil, fmt.Errorf("worker cognition provider: %w", err)
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
