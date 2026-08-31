package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func directCodingWorkerRuntime(session *directCodingSession) typedWorkerRuntime {
	if session == nil || session.runtime == nil || session.runtime.svc == nil || session.runtime.claim == nil {
		return typedWorkerRuntime{}
	}
	runtime := portableWorkerRuntime(session.runtime, "coding")
	runtime.PathProvenance = session.pathProvenance
	return runtime
}

func portableWorkerRuntime(runtime *nativeRuntimeV3, eventNamespace string) typedWorkerRuntime {
	if runtime == nil {
		return typedWorkerRuntime{}
	}
	return portableWorkerRuntimeWithContext(runtime, eventNamespace, runtime.ctx)
}

func portableWorkerRuntimeWithContext(
	runtime *nativeRuntimeV3,
	eventNamespace string,
	executionContext context.Context,
) typedWorkerRuntime {
	return portableWorkerRuntimeWithIdentityGuard(runtime, eventNamespace, executionContext, nil)
}

type portableIdentityGuard func(assemblyline.PortableJob, exactStationExecution) error

func portableWorkerRuntimeWithIdentityGuard(
	runtime *nativeRuntimeV3,
	eventNamespace string,
	executionContext context.Context,
	identityGuard portableIdentityGuard,
) typedWorkerRuntime {
	if runtime == nil || runtime.svc == nil || runtime.claim == nil {
		return typedWorkerRuntime{}
	}
	eventNamespace = safeEventToken(eventNamespace, "portable")
	pending := &sync.Map{}
	execute := func(
		job assemblyline.PortableJob,
		model string,
	) (assemblyline.PortableResult, error) {
		runtime.svc.emitStepEvent(
			runtime.claim.Authority,
			eventNamespace+"_portable_dispatched",
			fmt.Sprintf("kind=%s work=%s payload=%dB model=%s", job.Kind, job.ID[:12], len(job.Payload), safeEventToken(model, "unknown")),
		)
		result, execution, err := runtime.svc.executeExactPortableStation(
			executionContext, runtime.claim.Authority, job, model,
		)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if identityGuard != nil {
			if guardErr := identityGuard(job, execution); guardErr != nil {
				return assemblyline.PortableResult{}, guardErr
			}
		}
		if _, loaded := pending.LoadOrStore(job.ID, execution); loaded {
			return assemblyline.PortableResult{}, fmt.Errorf("portable work %s already has an unvalidated exact result", job.ID)
		}
		return result, nil
	}
	return typedWorkerRuntime{
		Context:        executionContext,
		MaxAttempts:    exactSemanticLeafCalls,
		MaxConcurrency: runtime.svc.fragmentConcurrency,
		PathProvenance: runtime.objectivePathProvenance,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			return execute(job, model)
		},
		Finalize: func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error {
			stored, exists := pending.LoadAndDelete(job.ID)
			if !exists {
				return fmt.Errorf("portable work %s has no pending exact station result", job.ID)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok || execution.WorkID != job.ID || result.JobID != job.ID ||
				execution.Candidate != result.Candidate {
				return fmt.Errorf("portable work %s result differs from its exact station receipt", job.ID)
			}
			if err := result.ValidateFor(job); err != nil {
				return fmt.Errorf("portable work %s result projection is invalid: %w", job.ID, err)
			}
			if validationErr != nil {
				return validationErr
			}
			if result.Projection == nil {
				return fmt.Errorf("portable work %s accepted result lacks an exact source projection", job.ID)
			}
			if result.Projection.SourceResponseSHA256 != execution.CandidateResponseSHA256 {
				return fmt.Errorf("portable work %s projection differs from its exact response", job.ID)
			}
			return nil
		},
		Emit: func(event typedWorkerEvent) {
			runtime.svc.emitStepEvent(
				runtime.claim.Authority,
				eventNamespace+"_worker_"+string(event.State),
				renderDirectCodingWorkerEvent(event),
			)
		},
	}
}

func portableModelScope(kind assemblyline.WorkKind) (string, error) {
	return assemblyline.PortableWorkerScopeForWorkKind(kind)
}

func (s *directCodingSession) workerModel(id station.ID) (string, error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.claim == nil {
		return "", fmt.Errorf("direct coding worker model routing is unavailable")
	}
	routing, err := s.runtime.modelRouting()
	if err != nil {
		return "", err
	}
	return stationModel(routing, id)
}

func renderDirectCodingWorkerEvent(event typedWorkerEvent) string {
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
