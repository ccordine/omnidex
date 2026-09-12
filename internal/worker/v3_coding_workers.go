package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
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
	continuations := &sync.Map{}
	var providerCalls atomic.Int64
	reservePending := func(job assemblyline.PortableJob) (func(), error) {
		if _, loaded := pending.LoadOrStore(job.Key(), struct{}{}); loaded {
			return nil, fmt.Errorf(
				"portable work %s already has an active or unvalidated exact result", job.Kind,
			)
		}
		return func() { pending.Delete(job.Key()) }, nil
	}
	execute := func(
		job assemblyline.PortableJob,
		model string,
	) (assemblyline.PortableResult, error) {
		if _, exists := continuations.Load(job.Key()); exists {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has a persisted rejected result and must continue that context",
				job.Kind,
			)
		}
		deterministic, resolved, err := assemblyline.ResolvePortableJobWithoutInference(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if resolved {
			releasePending, err := reservePending(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			execution := exactStationExecution{
				WorkInput: string(job.Payload), WorkKind: job.Kind, InferenceFree: true,
				Candidate: deterministic.Candidate,
			}
			if identityGuard != nil {
				if guardErr := identityGuard(job, execution); guardErr != nil {
					releasePending()
					return assemblyline.PortableResult{}, guardErr
				}
			}
			pending.Store(job.Key(), execution)
			return deterministic, nil
		}
		recovered, err := runtime.svc.recoverExactPortableStation(
			executionContext, runtime.claim.Authority, job, model,
		)
		if recovered != nil && recovered.Execution.ProviderCalls > 0 {
			providerCalls.Add(int64(recovered.Execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if recovered != nil {
			_, reserveErr := reservePending(job)
			if reserveErr != nil {
				return assemblyline.PortableResult{}, reserveErr
			}
			pending.Store(job.Key(), recovered.Execution)
			return recovered.Result, nil
		}
		releasePending, err := reservePending(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		keepPending := false
		defer func() {
			if !keepPending {
				releasePending()
			}
		}()
		runtime.svc.emitStepEvent(
			runtime.claim.Authority,
			eventNamespace+"_portable_dispatched",
			fmt.Sprintf("kind=%s payload=%dB model=%s", job.Kind, len(job.Payload), safeEventToken(model, "unknown")),
		)
		// Persisted rehydration returns above without spending inference.
		result, execution, err := runtime.svc.executeExactPortableStation(
			executionContext, runtime.claim.Authority, job, model,
		)
		if execution.ProviderCalls > 0 {
			providerCalls.Add(int64(execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if identityGuard != nil {
			if guardErr := identityGuard(job, execution); guardErr != nil {
				if persistErr := runtime.svc.persistExactStationSemanticOutcome(
					executionContext, runtime.claim.Authority, execution, result, guardErr,
				); persistErr != nil {
					return assemblyline.PortableResult{}, persistErr
				}
				return assemblyline.PortableResult{}, guardErr
			}
		}
		if job.Kind == assemblyline.WorkFragmentGeneration {
			if sourceState, stateErr := assemblyline.ExtractFragmentGenerationSourceBody(
				job, result.Candidate,
			); stateErr == nil {
				execution.SourceState = sourceState
			}
		}
		pending.Store(job.Key(), execution)
		keepPending = true
		return result, nil
	}
	correct := func(
		job assemblyline.PortableJob,
		model string,
		correction assemblyline.SourceBodyCorrection,
	) (assemblyline.PortableResult, error) {
		stored, exists := continuations.Load(job.Key())
		if !exists {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has no persisted rejected result to correct", job.Kind,
			)
		}
		previous, ok := stored.(exactStationExecution)
		if !ok {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has an invalid persisted correction context", job.Kind,
			)
		}
		if previous.Model != model {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s correction model %q differs from persisted model %q",
				job.Kind, model, previous.Model,
			)
		}
		if err := correction.Validate(); err != nil {
			return assemblyline.PortableResult{}, err
		}
		previousState := previous.SourceState
		if previousState == "" && previous.Iteration == 1 {
			var stateErr error
			previousState, stateErr = assemblyline.ExtractFragmentGenerationSourceBody(
				job, previous.Candidate,
			)
			if stateErr != nil {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"portable work %s rejected response has no correctable source state: %w",
					job.Kind, stateErr,
				)
			}
		}
		evidence, err := correction.Evidence()
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if evidence.BaseCandidate != previousState {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s correction does not bind to its persisted current source",
				job.Kind,
			)
		}
		modelInput, err := correction.ModelInput()
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"portable source-span correction", runtime.objectivePathProvenance, modelInput,
		); err != nil {
			return assemblyline.PortableResult{}, err
		}
		recovered, err := runtime.svc.recoverExactPortableStationChild(
			executionContext, runtime.claim.Authority, job, model,
			previous.CallEvidenceID,
		)
		if recovered != nil && recovered.Execution.ProviderCalls > 0 {
			providerCalls.Add(int64(recovered.Execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if recovered != nil {
			persistedCorrection := assemblyline.SourceBodyCorrectionEvidence{
				BaseCandidate: recovered.Evidence.SourceBaseCandidate,
				StartByte:     recovered.Evidence.SourceStartByte,
				EndByte:       recovered.Evidence.SourceEndByte,
				Question:      recovered.Evidence.SourceQuestion,
			}
			if recovered.Execution.Iteration != previous.Iteration+1 ||
				recovered.SemanticParentCallEvidenceID != previous.CallEvidenceID ||
				recovered.Evidence.ModelInput != modelInput ||
				persistedCorrection != evidence {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"portable work %s recreated correction differs from its persisted child",
					job.Kind,
				)
			}
			if err := persistedCorrection.Validate(recovered.Evidence.ModelInput); err != nil {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"portable work %s persisted child correction is invalid: %w",
					job.Kind, err,
				)
			}
			_, reserveErr := reservePending(job)
			if reserveErr != nil {
				return assemblyline.PortableResult{}, reserveErr
			}
			execution := recovered.Execution
			if sourceState, stateErr := correction.Apply(recovered.Result.Candidate); stateErr == nil {
				execution.SourceState = sourceState
			}
			pending.Store(job.Key(), execution)
			return recovered.Result, nil
		}
		releasePending, err := reservePending(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		keepPending := false
		defer func() {
			if !keepPending {
				releasePending()
			}
		}()
		runtime.svc.emitStepEvent(
			runtime.claim.Authority,
			eventNamespace+"_portable_correction_dispatched",
			fmt.Sprintf(
				"kind=%s iteration=%d model=%s mutable=%dB",
				job.Kind, previous.Iteration+1,
				safeEventToken(model, "unknown"), len(correction.Mutable()),
			),
		)
		result, execution, err := runtime.svc.executeExactPortableStationCorrection(
			executionContext, runtime.claim.Authority, job, model, previous, correction,
		)
		if execution.ProviderCalls > 0 {
			providerCalls.Add(int64(execution.ProviderCalls))
		}
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if identityGuard != nil {
			if guardErr := identityGuard(job, execution); guardErr != nil {
				if persistErr := runtime.svc.persistExactStationSemanticOutcome(
					executionContext, runtime.claim.Authority, execution, result, guardErr,
				); persistErr != nil {
					return assemblyline.PortableResult{}, persistErr
				}
				continuations.Delete(job.Key())
				return assemblyline.PortableResult{}, guardErr
			}
		}
		if sourceState, stateErr := correction.Apply(result.Candidate); stateErr == nil {
			execution.SourceState = sourceState
		}
		pending.Store(job.Key(), execution)
		keepPending = true
		return result, nil
	}
	return typedWorkerRuntime{
		Context:        executionContext,
		MaxAttempts:    exactSemanticLeafCalls,
		PathProvenance: runtime.objectivePathProvenance,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			return execute(job, model)
		},
		Correct: func(job assemblyline.PortableJob, model string, correction assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error) {
			return correct(job, model, correction)
		},
		AdvanceSource: func(
			job assemblyline.PortableJob,
			model string,
			expectedBase string,
			updatedBase string,
		) error {
			stored, exists := continuations.Load(job.Key())
			if !exists {
				return fmt.Errorf(
					"portable work %s has no persisted rejected source to advance", job.Kind,
				)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok || execution.WorkKind != assemblyline.WorkFragmentGeneration ||
				execution.Model != model || execution.SourceState != expectedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance differs from its persisted context",
					job.Kind,
				)
			}
			normalized, err := assemblyline.NormalizeSourceBodyResponse(updatedBase)
			if err != nil {
				return fmt.Errorf(
					"portable work %s deterministic source advance: %w", job.Kind, err,
				)
			}
			if normalized != updatedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance must already be normalized",
					job.Kind,
				)
			}
			if normalized == expectedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance has zero delta", job.Kind,
				)
			}
			execution.SourceState = normalized
			continuations.Store(job.Key(), execution)
			return nil
		},
		ProviderCalls: func() int {
			return int(providerCalls.Load())
		},
		Release: func(job assemblyline.PortableJob) error {
			if _, active := pending.Load(job.Key()); active {
				return fmt.Errorf(
					"portable work %s cannot release an unvalidated exact result", job.Kind,
				)
			}
			continuations.Delete(job.Key())
			return nil
		},
		Finalize: func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error {
			stored, exists := pending.LoadAndDelete(job.Key())
			if !exists {
				return fmt.Errorf("portable work %s has no pending exact station result", job.Kind)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok {
				return fmt.Errorf("portable work %s has an invalid exact station receipt", job.Kind)
			}
			if handled, deterministicErr := finalizeInferenceFreePortableResult(
				job, result, execution,
			); handled {
				return deterministicErr
			}
			providedValidationErr := validationErr
			if validationErr == nil && (execution.WorkInput != string(job.Payload) || execution.WorkKind != job.Kind ||
				execution.Candidate != result.Candidate) {
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
					return fmt.Errorf(
						"portable work %s deterministic replay differs from its persisted semantic outcome",
						job.Kind,
					)
				}
				if validationErr != nil &&
					execution.PersistedValidationError != exactStationEvidenceError(validationErr) {
					return fmt.Errorf(
						"portable work %s deterministic replay differs from its persisted rejection",
						job.Kind,
					)
				}
				if validationErr == nil {
					continuations.Delete(job.Key())
				} else if providedValidationErr != nil &&
					execution.WorkKind == assemblyline.WorkFragmentGeneration {
					continuations.Store(job.Key(), execution)
				}
				if providedValidationErr != nil {
					return nil
				}
				return validationErr
			}
			if persistErr := runtime.svc.persistExactStationSemanticOutcome(
				executionContext, runtime.claim.Authority, execution, result, validationErr,
			); persistErr != nil {
				return persistErr
			}
			if validationErr == nil {
				continuations.Delete(job.Key())
			} else if providedValidationErr != nil {
				continuations.Store(job.Key(), execution)
			}
			if providedValidationErr != nil {
				return nil
			}
			return validationErr
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
