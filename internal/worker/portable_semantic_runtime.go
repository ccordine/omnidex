package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func portableModelScope(kind assemblyline.WorkKind) (string, error) {
	return assemblyline.PortableWorkerScopeForWorkKind(kind)
}

func portableWorkerRuntime(runtime *nativeRuntimeV3, eventNamespace string) typedWorkerRuntime {
	if runtime == nil {
		return typedWorkerRuntime{}
	}
	return portableSemanticRuntime(runtime, eventNamespace, runtime.ctx)
}

// portableSemanticRuntime is the evidence-backed boundary used by the narrow
// non-agent semantic stations. It cannot execute tools, mutate files, plan
// work, or continue a rejected source-generation context.
func portableSemanticRuntime(
	runtime *nativeRuntimeV3,
	eventNamespace string,
	executionContext context.Context,
) typedWorkerRuntime {
	if runtime == nil || runtime.svc == nil || runtime.claim == nil || executionContext == nil {
		return typedWorkerRuntime{}
	}
	eventNamespace = safeEventToken(eventNamespace, "portable")
	pending := &sync.Map{}
	var providerCalls atomic.Int64
	reserve := func(job assemblyline.PortableJob) error {
		if _, loaded := pending.LoadOrStore(job.Key(), struct{}{}); loaded {
			return fmt.Errorf("portable work %s already has an unvalidated exact result", job.Kind)
		}
		return nil
	}
	execute := func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
		deterministic, resolved, err := assemblyline.ResolvePortableJobWithoutInference(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if err := reserve(job); err != nil {
			return assemblyline.PortableResult{}, err
		}
		keep := false
		defer func() {
			if !keep {
				pending.Delete(job.Key())
			}
		}()
		if resolved {
			pending.Store(job.Key(), exactStationExecution{
				WorkInput: string(job.Payload), WorkKind: job.Kind,
				InferenceFree: true, Candidate: deterministic.Candidate,
			})
			keep = true
			return deterministic, nil
		}
		recovered, err := runtime.svc.recoverExactPortableStation(
			executionContext, runtime.claim.Authority, job, modelName,
		)
		if recovered != nil && recovered.Execution.ProviderCalls > 0 {
			providerCalls.Add(int64(recovered.Execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if recovered != nil {
			pending.Store(job.Key(), recovered.Execution)
			keep = true
			return recovered.Result, nil
		}
		runtime.svc.emitStepEvent(runtime.claim.Authority, eventNamespace+"_portable_dispatched",
			fmt.Sprintf("kind=%s payload=%dB model=%s", job.Kind, len(job.Payload), safeEventToken(modelName, "unknown")))
		result, execution, err := runtime.svc.executeExactPortableStation(
			executionContext, runtime.claim.Authority, job, modelName,
		)
		if execution.ProviderCalls > 0 {
			providerCalls.Add(int64(execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		pending.Store(job.Key(), execution)
		keep = true
		return result, nil
	}
	return typedWorkerRuntime{
		Context: executionContext, MaxAttempts: exactSemanticLeafCalls,
		PathProvenance: runtime.objectivePathProvenance,
		Execute:        execute,
		ProviderCalls:  func() int { return int(providerCalls.Load()) },
		ReadCallEvidenceID: func(job assemblyline.PortableJob) int64 {
			stored, exists := pending.Load(job.Key())
			if !exists {
				return 0
			}
			execution, ok := stored.(exactStationExecution)
			if !ok {
				return 0
			}
			return execution.CallEvidenceID
		},
		Release: func(job assemblyline.PortableJob) error {
			if _, active := pending.Load(job.Key()); active {
				return fmt.Errorf("portable work %s cannot release an unvalidated exact result", job.Kind)
			}
			return nil
		},
		Finalize: runtime.afterAcceptedPortableResult(executionContext, func(
			job assemblyline.PortableJob,
			result assemblyline.PortableResult,
			validationErr error,
		) error {
			stored, exists := pending.LoadAndDelete(job.Key())
			if !exists {
				return fmt.Errorf("portable work %s has no pending exact station result", job.Kind)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok {
				return fmt.Errorf("portable work %s has an invalid exact station receipt", job.Kind)
			}
			if handled, deterministicErr := finalizeInferenceFreePortableResult(job, result, execution); handled {
				return deterministicErr
			}
			if validationErr == nil && (execution.WorkInput != string(job.Payload) ||
				execution.WorkKind != job.Kind || execution.Candidate != result.Candidate) {
				validationErr = fmt.Errorf("portable work %s result differs from its exact station receipt", job.Kind)
			}
			if validationErr == nil {
				if err := result.ValidateFor(job); err != nil {
					validationErr = fmt.Errorf("portable work %s result projection is invalid: %w", job.Kind, err)
				}
			}
			if execution.Replayed {
				expectedAccepted := execution.PersistedOutcome == queue.LLMCallAccepted
				if expectedAccepted != (validationErr == nil) {
					return fmt.Errorf("portable work %s replay differs from its persisted outcome", job.Kind)
				}
				if validationErr != nil && execution.PersistedValidationError != exactStationEvidenceError(validationErr) {
					return fmt.Errorf("portable work %s replay differs from its persisted rejection", job.Kind)
				}
				return nil
			}
			if err := runtime.svc.persistExactStationSemanticOutcome(
				executionContext, runtime.claim.Authority, execution, result, validationErr,
			); err != nil {
				return err
			}
			return nil
		}),
		Emit: func(event typedWorkerEvent) {
			runtime.svc.emitStepEvent(runtime.claim.Authority,
				eventNamespace+"_worker_"+string(event.State), renderTypedWorkerEvent(event))
		},
		ObservePlanning: func(activity model.PlanningActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{
				JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
				Attempt: runtime.claim.Authority.Attempt, Kind: "planning_activity", PlanningActivity: &activity,
			})
		},
	}
}
