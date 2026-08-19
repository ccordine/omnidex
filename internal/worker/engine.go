package worker

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/queue"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/websearch"
)

const stepControlPollInterval = 300 * time.Millisecond

type ModelRouting struct {
	Stations map[station.ID]string
}

type stepCompleteFunc func(context.Context, queue.CompleteStepCommand) error

type nativeV3StepRunner func(context.Context, *model.ClaimedStep, map[string]string, string) error

type WorkspaceSettings struct {
	Root     string
	HostRoot string
}

type Options struct {
	WorkerCount               int
	FragmentConcurrency       int
	PollInterval              time.Duration
	InferenceContextTokens    int
	InferenceProvider         string
	EmbeddingProvider         string
	EmbeddingModel            string
	Models                    ModelRouting
	ObjectiveAdvisoryMode     objectiveadvisory.Mode
	ObjectiveAdvisoryProvider string
	Workspace                 WorkspaceSettings
	Logger                    *log.Logger
	OnJobFinished             func(jobID int64)
	OnJobOutput               func(jobID int64, delta string)
}

type Service struct {
	repo                      *queue.Repository
	embeddings                llm.EmbeddingClient
	stationClient             llm.ExactStationClient
	webSearch                 *websearch.Service
	workerCount               int
	fragmentConcurrency       int
	pollInterval              time.Duration
	inferenceContextTokens    int
	inferenceProvider         string
	embeddingProvider         string
	embeddingModel            string
	models                    ModelRouting
	objectiveAdvisoryMode     objectiveadvisory.Mode
	objectiveAdvisoryProvider string
	workspaceRoot             string
	repositoryIndex           repositoryIndexRefresher
	repositoryRetrieval       repositoryEvidenceBuilder
	workspaceHostRoot         string
	completeStep              stepCompleteFunc
	nativeV3Runner            nativeV3StepRunner
	logger                    *log.Logger
	onJobFinished             func(jobID int64)
	onJobOutput               func(jobID int64, delta string)
}

func New(
	repo *queue.Repository,
	stationClient llm.ExactStationClient,
	embeddings llm.EmbeddingClient,
	webSearch *websearch.Service,
	opts Options,
) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("worker repository is required")
	}
	if nilWorkerTransport(stationClient) {
		return nil, fmt.Errorf("exact station client is required")
	}
	if nilWorkerTransport(embeddings) {
		return nil, fmt.Errorf("embedding client is required")
	}
	if err := validateWorkerOptions(opts); err != nil {
		return nil, fmt.Errorf("invalid worker options: %w", err)
	}
	opts = normalizeWorkerOptions(opts)

	repositoryIndex, err := repositoryindex.New(repo)
	if err != nil {
		return nil, fmt.Errorf("configure repository indexer: %w", err)
	}
	repositoryRetrieval, err := repositoryretrieval.New(repo)
	if err != nil {
		return nil, fmt.Errorf("configure repository retrieval: %w", err)
	}

	var completeStep stepCompleteFunc
	if repo != nil {
		completeStep = repo.CompleteStep
	}
	svc := &Service{
		repo:                      repo,
		embeddings:                embeddings,
		stationClient:             stationClient,
		webSearch:                 webSearch,
		workerCount:               opts.WorkerCount,
		fragmentConcurrency:       opts.FragmentConcurrency,
		pollInterval:              opts.PollInterval,
		inferenceContextTokens:    opts.InferenceContextTokens,
		inferenceProvider:         opts.InferenceProvider,
		embeddingProvider:         opts.EmbeddingProvider,
		embeddingModel:            opts.EmbeddingModel,
		models:                    opts.Models,
		objectiveAdvisoryMode:     opts.ObjectiveAdvisoryMode,
		objectiveAdvisoryProvider: opts.ObjectiveAdvisoryProvider,
		workspaceRoot:             opts.Workspace.Root,
		repositoryIndex:           repositoryIndex,
		repositoryRetrieval:       repositoryRetrieval,
		workspaceHostRoot:         opts.Workspace.HostRoot,
		completeStep:              completeStep,
		logger:                    opts.Logger,
		onJobFinished:             opts.OnJobFinished,
		onJobOutput:               opts.OnJobOutput,
	}
	if repo != nil && completeStep != nil {
		svc.completeStep = svc.wrapStepCompleter(completeStep)
	}
	svc.nativeV3Runner = svc.runNativeV3Step
	return svc, nil
}

func nilWorkerTransport(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
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
