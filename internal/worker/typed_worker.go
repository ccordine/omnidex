package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func renderTypedWorkerEvent(event typedWorkerEvent) string {
	parts := []string{
		"kind=" + safeLine(string(event.Kind), "unknown"),
		"subject=" + safeEventToken(event.Subject, "unknown"),
		"model=" + safeEventToken(event.Model, "unknown"),
	}
	if event.MaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt=%d/%d", event.Attempt, event.MaxAttempts))
	} else if event.Attempt > 0 {
		parts = append(parts, fmt.Sprintf("attempt=%d", event.Attempt))
	}
	if event.PromptBytes > 0 {
		parts = append(parts, fmt.Sprintf(
			"context=prompt:%dB,capabilities:%dB,current:%dB,correction:%dB",
			event.PromptBytes, event.CapabilityBytes, event.CurrentBytes, event.CorrectionBytes,
		))
	}
	if detail := strings.TrimSpace(event.Detail); detail != "" {
		parts = append(parts, "error="+safeLine(detail, "unknown"))
	}
	if warning := strings.TrimSpace(event.Warning); warning != "" {
		parts = append(parts, "warning="+safeLine(warning, "unknown"))
	}
	return strings.Join(parts, " ")
}

func safeEventToken(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return strings.NewReplacer(" ", "_", "\t", "_", "\n", "_").Replace(value)
}

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
	typedWorkerStopped   typedWorkerState = "stopped"
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
	Context            context.Context
	MaxAttempts        int
	PathProvenance     assemblyline.ArtifactIdentityProvenance
	Execute            func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error)
	Correct            func(job assemblyline.PortableJob, model string, correction assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error)
	AdvanceSource      func(job assemblyline.PortableJob, model string, expectedBase string, updatedBase string) error
	ProviderCalls      func() int
	ReadCallEvidenceID func(job assemblyline.PortableJob) int64
	Release            func(job assemblyline.PortableJob) error
	Finalize           func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error
	Emit               func(event typedWorkerEvent)
	ObservePlanning    func(activity model.PlanningActivity)
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
