package worker

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/websearch"
)

const stepControlPollInterval = 300 * time.Millisecond

type ModelRouting = modelconfig.Routing

type stepCompleteFunc func(context.Context, queue.CompleteStepCommand) error

type Options struct {
	PollInterval           string
	InferenceContextTokens string
	Logger                 *log.Logger
	RuntimeEventSink       RuntimeEventSink
}

type Service struct {
	repo                   *queue.Repository
	stationClient          llm.ExactStationClient
	webSearch              *websearch.Service
	pollInterval           string
	inferenceContextTokens string
	completeStep           stepCompleteFunc
	logger                 *log.Logger
	runtimeEventSink       RuntimeEventSink
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

	svc := &Service{
		repo:                   repo,
		stationClient:          stationClient,
		webSearch:              webSearch,
		pollInterval:           opts.PollInterval,
		inferenceContextTokens: opts.InferenceContextTokens,
		completeStep:           repo.CompleteStep,
		logger:                 opts.Logger,
		runtimeEventSink:       opts.RuntimeEventSink,
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
