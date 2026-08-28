package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type objectiveRawLeafDecoder[T any] func(string) (T, error)

func runObjectivePortableRawLeafCall[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	modelName, subject string,
	job assemblyline.PortableJob,
	decode objectiveRawLeafDecoder[T],
	validate func(T) error,
) (T, int, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil {
		return zero, 0, fmt.Errorf("objective raw leaf requires exact running step authority")
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, 0, fmt.Errorf("objective raw leaf %s model is not configured", subject)
	}
	workerRuntime := portableWorkerRuntimeWithContext(runtime, "objective", ctx)
	calls := 0
	execute := workerRuntime.Execute
	workerRuntime.Execute = func(
		job assemblyline.PortableJob,
		model string,
	) (assemblyline.PortableResult, error) {
		calls++
		return execute(job, model)
	}
	value, err := runObjectiveRawLeafWorkerCall(
		workerRuntime, modelName, subject, job, decode, validate,
	)
	return value, calls, err
}

func runObjectiveRawLeafWorkerCall[T any](
	runtime typedWorkerRuntime,
	modelName, subject string,
	job assemblyline.PortableJob,
	decode objectiveRawLeafDecoder[T],
	validate func(T) error,
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil || decode == nil || validate == nil {
		return zero, fmt.Errorf("objective raw leaf requires an exact portable runtime and decoder")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return zero, fmt.Errorf(
			"objective raw leaf attempts must be between 1 and %d", maxTypedWorkerAttempts,
		)
	}
	basePrompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, err
	}
	if err := validateDirectCodingSemanticPrompt(
		basePrompt, nil, runtime.PathProvenance,
	); err != nil {
		return zero, err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: runtime.MaxAttempts,
		PromptBytes: len(basePrompt),
	})

	var lastErr error
	var retainedCandidate string
	attemptsUsed := 0
	seenCandidates := make(map[string]struct{})
	seenJobs := make(map[string]struct{})
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		attemptsUsed = attempt
		if err := runtime.Context.Err(); err != nil {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt-1,
				fmt.Errorf("authority ended: %w", err),
			)
		}
		attemptJob := job
		if lastErr != nil {
			if retainedCandidate == "" || retainedCandidate != strings.TrimSpace(retainedCandidate) {
				break
			}
			attemptJob, err = assemblyline.NewRetainedResponseCorrectionJob(
				job, trimForBudget(lastErr.Error(), 1200), retainedCandidate,
			)
			if err != nil {
				return zero, failDirectCodingSemanticCall(
					runtime, modelName, subject, attempt-1, err,
				)
			}
		}
		prompt, err := assemblyline.RenderPortableJob(attemptJob)
		if err != nil {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt-1, err,
			)
		}
		if err := validateDirectCodingSemanticPrompt(
			prompt, nil, runtime.PathProvenance,
		); err != nil {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt-1, err,
			)
		}
		if _, duplicate := seenJobs[attemptJob.ID]; duplicate {
			break
		}
		seenJobs[attemptJob.ID] = struct{}{}

		result, err := runtime.Execute(attemptJob, modelName)
		if err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		validationErr := result.ValidateFor(attemptJob)
		if validationErr != nil {
			validationErr = finalizeTypedWorkerResult(
				runtime, attemptJob, result, validationErr,
			)
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt, validationErr,
			)
		}
		candidate := result.Candidate
		candidatePreservable := true
		if attemptJob.Kind == assemblyline.WorkResponseCorrection {
			var replacement string
			replacement, validationErr = assemblyline.DecodeResponseCorrectionReplacement(
				attemptJob, result.Candidate,
			)
			if validationErr == nil && replacement != result.Candidate {
				candidatePreservable = false
				validationErr = fmt.Errorf(
					"response correction altered exact raw replacement bytes",
				)
			}
			candidate = replacement
		}
		if validationErr == nil {
			if _, duplicate := seenCandidates[candidate]; duplicate {
				validationErr = fmt.Errorf(
					"repeated identical raw candidate; correction made no progress",
				)
			} else {
				seenCandidates[candidate] = struct{}{}
			}
		}
		var value T
		if validationErr == nil {
			value, validationErr = decode(candidate)
		}
		if validationErr == nil {
			validationErr = validateObjectiveRawLeafPathBoundary(
				value, runtime.PathProvenance,
			)
		}
		if validationErr == nil {
			validationErr = validate(value)
		}
		validationErr = finalizeTypedWorkerResult(
			runtime, attemptJob, result, validationErr,
		)
		if validationErr == nil {
			emitTypedWorker(runtime, typedWorkerEvent{
				State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
				Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			})
			return value, nil
		}
		lastErr = validationErr
		if candidatePreservable {
			retainedCandidate = candidate
		} else {
			retainedCandidate = ""
		}
		emitDirectCodingSemanticRejection(
			runtime, modelName, subject, attempt, validationErr,
		)
		if strings.Contains(validationErr.Error(), "repeated identical raw candidate") {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("raw semantic leaf correction made no deterministic progress")
	}
	return zero, failDirectCodingSemanticCall(
		runtime, modelName, subject, attemptsUsed,
		&semanticCandidateExhaustedError{
			Subject: subject, Attempts: attemptsUsed, Err: lastErr,
		},
	)
}

func validateObjectiveRawLeafPathBoundary[T any](
	value T,
	provenance assemblyline.ArtifactIdentityProvenance,
) error {
	if boundary, ok := any(value).(interface {
		ValidatePathFree(assemblyline.ArtifactIdentityProvenance) error
	}); ok {
		return boundary.ValidatePathFree(provenance)
	}
	return nil
}
