package worker

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/websearch"
)

const stepControlPollInterval = 300 * time.Millisecond

type ModelRouting = modelconfig.Routing

type stepCompleteFunc func(context.Context, queue.CompleteStepCommand) error

type Options struct {
	WorkerCount            int
	FragmentConcurrency    int
	PollInterval           time.Duration
	InferenceContextTokens int
	Logger                 *log.Logger
	OnJobFinished          func(jobID int64)
}

type Service struct {
	repo                   *queue.Repository
	stationClient          llm.ExactStationClient
	webSearch              *websearch.Service
	workerCount            int
	fragmentConcurrency    int
	pollInterval           time.Duration
	inferenceContextTokens int
	completeStep           stepCompleteFunc
	logger                 *log.Logger
	onJobFinished          func(jobID int64)
}

func New(
	repo *queue.Repository,
	stationClient llm.ExactStationClient,
	webSearch *websearch.Service,
	opts Options,
) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("worker repository is required")
	}

	var completeStep stepCompleteFunc
	if repo != nil {
		completeStep = repo.CompleteStep
	}
	svc := &Service{
		repo:                   repo,
		stationClient:          stationClient,
		webSearch:              webSearch,
		workerCount:            opts.WorkerCount,
		fragmentConcurrency:    opts.FragmentConcurrency,
		pollInterval:           opts.PollInterval,
		inferenceContextTokens: opts.InferenceContextTokens,
		completeStep:           completeStep,
		logger:                 opts.Logger,
		onJobFinished:          opts.OnJobFinished,
	}
	if repo != nil && completeStep != nil {
		svc.completeStep = svc.wrapStepCompleter(completeStep)
	}
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
