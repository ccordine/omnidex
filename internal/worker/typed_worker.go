package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const maxTypedWorkerAttempts = 3

type typedWorkerKind string

const (
	typedWorkerSemantic typedWorkerKind = "semantic"
	typedWorkerFragment typedWorkerKind = "fragment"
	typedWorkerAdvisory typedWorkerKind = "advisory"
)

func (kind typedWorkerKind) validate() error {
	switch kind {
	case typedWorkerSemantic, typedWorkerFragment, typedWorkerAdvisory:
		return nil
	default:
		return fmt.Errorf("typed worker kind %q is not registered", kind)
	}
}

type typedWorkerState string

const (
	typedWorkerStarted   typedWorkerState = "started"
	typedWorkerRejected  typedWorkerState = "rejected"
	typedWorkerCompleted typedWorkerState = "completed"
	typedWorkerFailed    typedWorkerState = "failed"
)

type typedWorkerEvent struct {
	State           typedWorkerState
	Kind            typedWorkerKind
	Subject         string
	Model           string
	Attempt         int
	MaxAttempts     int
	PromptBytes     int
	CapabilityBytes int
	CurrentBytes    int
	CorrectionBytes int
	Detail          string
}

type typedWorkerRuntime struct {
	Context         context.Context
	MaxAttempts     int
	MaxConcurrency  int
	CorrectionModel string
	AdvisoryModel   string
	Execute         func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error)
	Advise          func(job assemblyline.PortableJob, model string) (llm.AdvisoryResponse, error)
	Emit            func(event typedWorkerEvent)
}

func emitTypedWorker(runtime typedWorkerRuntime, event typedWorkerEvent) {
	if runtime.Emit != nil {
		runtime.Emit(event)
	}
}
