package worker

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/websearch"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const stepControlPollInterval = 300 * time.Millisecond

type ModelRouting = modelconfig.Routing

type stepCompleteFunc func(context.Context, queue.CompleteStepCommand) error
type stepFailFunc func(context.Context, queue.FailStepCommand) error

type Options struct {
	PollInterval            string
	InferenceContextTokens  string
	HostDirectoryAccessRoot string
	Logger                  *log.Logger
	RuntimeEventSink        RuntimeEventSink
}

type Service struct {
	repo                   *queue.Repository
	stationClient          llm.ExactStationClient
	webSearch              *websearch.Service
	pollInterval           string
	inferenceContextTokens string
	hostDirectoryAccess    workspacefacts.HostDirectoryAccess
	completeStep           stepCompleteFunc
	failStep               stepFailFunc
	logger                 *log.Logger
	runtimeEventSink       RuntimeEventSink
	runtimeEventMu         sync.RWMutex
	runtimeEventChannels   map[int64]runtimeEventChannelBinding
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
	hostDirectoryAccess, err := workspacefacts.NewHostDirectoryAccess(
		opts.HostDirectoryAccessRoot,
	)
	if err != nil {
		return nil, fmt.Errorf("construct worker host directory access authority: %w", err)
	}

	svc := &Service{
		repo:                   repo,
		stationClient:          stationClient,
		webSearch:              webSearch,
		pollInterval:           opts.PollInterval,
		inferenceContextTokens: opts.InferenceContextTokens,
		hostDirectoryAccess:    hostDirectoryAccess,
		completeStep:           repo.CompleteStep,
		failStep:               repo.FailStep,
		logger:                 opts.Logger,
		runtimeEventSink:       opts.RuntimeEventSink,
		runtimeEventChannels:   make(map[int64]runtimeEventChannelBinding),
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
