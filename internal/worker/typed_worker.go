package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const exactSemanticLeafCalls = assemblyline.ExactSemanticLeafCalls

type typedWorkerKind string

const (
	typedWorkerSemantic typedWorkerKind = "semantic"
	typedWorkerFragment typedWorkerKind = "fragment"
)

func (kind typedWorkerKind) validate() error {
	switch kind {
	case typedWorkerSemantic, typedWorkerFragment:
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
	Warning         string
	Detail          string
}

type typedWorkerRuntime struct {
	Context        context.Context
	MaxAttempts    int
	PathProvenance assemblyline.ArtifactIdentityProvenance
	Execute        func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error)
	Correct        func(job assemblyline.PortableJob, model string, correction assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error)
	AdvanceSource  func(job assemblyline.PortableJob, model string, expectedBase string, updatedBase string) error
	ProviderCalls  func() int
	Release        func(job assemblyline.PortableJob) error
	Finalize       func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error
	Emit           func(event typedWorkerEvent)
}

func finalizeTypedWorkerResult(
	runtime typedWorkerRuntime,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if runtime.Finalize == nil {
		return validationErr
	}
	finalizeErr := runtime.Finalize(job, result, validationErr)
	if validationErr != nil {
		if finalizeErr != nil {
			return fmt.Errorf("%v; persist station rejection: %w", validationErr, finalizeErr)
		}
		return validationErr
	}
	return finalizeErr
}

func emitTypedWorker(runtime typedWorkerRuntime, event typedWorkerEvent) {
	if runtime.Emit != nil {
		runtime.Emit(event)
	}
}
