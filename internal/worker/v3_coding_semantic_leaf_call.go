package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingSemanticLeafDecoder[T any] func(string) (T, error)

// runDirectCodingSemanticLeafCall resolves one raw semantic value. The
// station-specific decoder is the only authority that may turn model bytes
// into typed state; this boundary never decodes a generic object or record.
func runDirectCodingSemanticLeafCall[T any](
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	job assemblyline.PortableJob,
	identities []assemblyline.ArtifactIdentity,
	decode directCodingSemanticLeafDecoder[T],
	validate func(T) error,
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil || decode == nil || validate == nil {
		return zero, fmt.Errorf("coding semantic leaf requires an exact portable runtime, decoder, and validator")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return zero, fmt.Errorf(
			"coding semantic leaf attempts must be between 1 and %d",
			maxTypedWorkerAttempts,
		)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, fmt.Errorf("coding semantic leaf requires one configured model")
	}
	basePrompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, err
	}
	if err := validateDirectCodingSemanticPrompt(
		basePrompt, identities, runtime.PathProvenance,
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
			if retainedCandidate == "" {
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
			prompt, identities, runtime.PathProvenance,
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
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt, err,
			)
		}
		validationErr := result.ValidateFor(attemptJob)
		if validationErr != nil {
			validationErr = finalizeTypedWorkerResult(
				runtime, attemptJob, result, validationErr,
			)
			emitDirectCodingSemanticRejection(
				runtime, modelName, subject, attempt, validationErr,
			)
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt, validationErr,
			)
		}
		candidate := result.Candidate
		candidatePreservable := candidate != "" && candidate == strings.TrimSpace(candidate)
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
		var value T
		if validationErr == nil {
			value, validationErr = decode(candidate)
		}
		if validationErr == nil {
			if boundary, ok := any(value).(interface {
				ValidatePathFree(assemblyline.ArtifactIdentityProvenance) error
			}); ok {
				validationErr = boundary.ValidatePathFree(runtime.PathProvenance)
			}
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
		emitDirectCodingSemanticRejection(
			runtime, modelName, subject, attempt, validationErr,
		)
		if !candidatePreservable {
			break
		}
		if _, duplicate := seenCandidates[candidate]; duplicate {
			break
		}
		seenCandidates[candidate] = struct{}{}
		retainedCandidate = candidate
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("semantic leaf correction made no deterministic progress")
	}
	return zero, failDirectCodingSemanticCall(
		runtime, modelName, subject, attemptsUsed,
		&semanticCandidateExhaustedError{
			Subject: subject, Attempts: attemptsUsed, Err: lastErr,
		},
	)
}
