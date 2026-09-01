package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
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
	continuations := &sync.Map{}
	var providerCalls atomic.Int64
	reservePending := func(job assemblyline.PortableJob) (func(), error) {
		if _, loaded := pending.LoadOrStore(job.ID, struct{}{}); loaded {
			return nil, fmt.Errorf(
				"portable work %s already has an active or unvalidated exact result", job.ID,
			)
		}
		return func() { pending.Delete(job.ID) }, nil
	}
	execute := func(
		job assemblyline.PortableJob,
		model string,
	) (assemblyline.PortableResult, error) {
		if _, exists := continuations.Load(job.ID); exists {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has a persisted rejected result and must continue that context",
				job.ID,
			)
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
			pending.Store(job.ID, recovered.Execution)
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
			fmt.Sprintf("kind=%s work=%s payload=%dB model=%s", job.Kind, job.ID[:12], len(job.Payload), safeEventToken(model, "unknown")),
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
		pending.Store(job.ID, execution)
		keepPending = true
		return result, nil
	}
	correct := func(
		job assemblyline.PortableJob,
		model string,
		correction assemblyline.SourceBodyCorrection,
	) (assemblyline.PortableResult, error) {
		stored, exists := continuations.Load(job.ID)
		if !exists {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has no persisted rejected result to correct", job.ID,
			)
		}
		previous, ok := stored.(exactStationExecution)
		if !ok {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s has an invalid persisted correction context", job.ID,
			)
		}
		if previous.Model != model {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"portable work %s correction model %q differs from persisted model %q",
				job.ID, model, previous.Model,
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
					job.ID, stateErr,
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
				job.ID,
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
				BaseCandidate:  recovered.Evidence.SourceBaseCandidate,
				BaseSHA256:     recovered.Evidence.SourceBaseSHA256,
				StartByte:      recovered.Evidence.SourceStartByte,
				EndByte:        recovered.Evidence.SourceEndByte,
				Question:       recovered.Evidence.SourceQuestion,
				QuestionSHA256: recovered.Evidence.SourceQuestionSHA256,
			}
			if recovered.Execution.Iteration != previous.Iteration+1 ||
				recovered.SemanticParentCallEvidenceID != previous.CallEvidenceID ||
				recovered.Evidence.ModelInput != modelInput ||
				persistedCorrection != evidence {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"portable work %s recreated correction differs from its persisted child",
					job.ID,
				)
			}
			if err := persistedCorrection.Validate(recovered.Evidence.ModelInput); err != nil {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"portable work %s persisted child correction is invalid: %w",
					job.ID, err,
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
			pending.Store(job.ID, execution)
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
				"kind=%s work=%s iteration=%d model=%s mutable=%dB",
				job.Kind, job.ID[:12], previous.Iteration+1,
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
				continuations.Delete(job.ID)
				return assemblyline.PortableResult{}, guardErr
			}
		}
		if sourceState, stateErr := correction.Apply(result.Candidate); stateErr == nil {
			execution.SourceState = sourceState
		}
		pending.Store(job.ID, execution)
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
			stored, exists := continuations.Load(job.ID)
			if !exists {
				return fmt.Errorf(
					"portable work %s has no persisted rejected source to advance", job.ID,
				)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok || execution.WorkKind != assemblyline.WorkFragmentGeneration ||
				execution.Model != model || execution.SourceState != expectedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance differs from its persisted context",
					job.ID,
				)
			}
			normalized, err := assemblyline.NormalizeSourceBodyResponse(updatedBase)
			if err != nil {
				return fmt.Errorf(
					"portable work %s deterministic source advance: %w", job.ID, err,
				)
			}
			if normalized != updatedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance must already be normalized",
					job.ID,
				)
			}
			if normalized == expectedBase {
				return fmt.Errorf(
					"portable work %s deterministic source advance has zero delta", job.ID,
				)
			}
			execution.SourceState = normalized
			continuations.Store(job.ID, execution)
			return nil
		},
		ProviderCalls: func() int {
			return int(providerCalls.Load())
		},
		Release: func(job assemblyline.PortableJob) error {
			if _, active := pending.Load(job.ID); active {
				return fmt.Errorf(
					"portable work %s cannot release an unvalidated exact result", job.ID,
				)
			}
			continuations.Delete(job.ID)
			return nil
		},
		Finalize: func(job assemblyline.PortableJob, result assemblyline.PortableResult, validationErr error) error {
			stored, exists := pending.LoadAndDelete(job.ID)
			if !exists {
				return fmt.Errorf("portable work %s has no pending exact station result", job.ID)
			}
			execution, ok := stored.(exactStationExecution)
			if !ok {
				return fmt.Errorf("portable work %s has an invalid exact station receipt", job.ID)
			}
			providedValidationErr := validationErr
			if validationErr == nil && (execution.WorkID != job.ID || result.JobID != job.ID ||
				execution.Candidate != result.Candidate) {
				validationErr = fmt.Errorf("portable work %s result differs from its exact station receipt", job.ID)
			}
			if validationErr == nil {
				if err := result.ValidateFor(job); err != nil {
					validationErr = fmt.Errorf("portable work %s result projection is invalid: %w", job.ID, err)
				}
			}
			if validationErr == nil && result.Projection == nil {
				validationErr = fmt.Errorf("portable work %s accepted result lacks its exact response receipt", job.ID)
			}
			if validationErr == nil &&
				result.Projection.SourceResponseSHA256 != execution.CandidateResponseSHA256 {
				validationErr = fmt.Errorf("portable work %s projection differs from its exact response", job.ID)
			}
			if execution.Replayed {
				expectedAccepted := execution.PersistedOutcome == queue.LLMCallAccepted
				if expectedAccepted != (validationErr == nil) {
					return fmt.Errorf(
						"portable work %s deterministic replay differs from its persisted semantic outcome",
						job.ID,
					)
				}
				if validationErr != nil &&
					execution.PersistedValidationError != exactStationEvidenceError(validationErr) {
					return fmt.Errorf(
						"portable work %s deterministic replay differs from its persisted rejection",
						job.ID,
					)
				}
				if validationErr == nil {
					continuations.Delete(job.ID)
				} else if providedValidationErr != nil &&
					execution.WorkKind == assemblyline.WorkFragmentGeneration {
					continuations.Store(job.ID, execution)
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
				continuations.Delete(job.ID)
			} else if providedValidationErr != nil {
				continuations.Store(job.ID, execution)
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
