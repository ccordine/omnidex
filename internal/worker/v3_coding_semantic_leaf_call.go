package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingSemanticLeafDecoder[T any] func(string) (T, error)

type directCodingSemanticLeafRejection struct {
	subject string
	err     error
}

func (rejection *directCodingSemanticLeafRejection) Error() string {
	return fmt.Sprintf("invalid %s semantic result: %v", rejection.subject, rejection.err)
}

func (rejection *directCodingSemanticLeafRejection) Unwrap() error {
	return rejection.err
}

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
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil || decode == nil {
		return zero, fmt.Errorf("coding semantic leaf requires an exact portable runtime and decoder")
	}
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
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		PromptBytes: len(basePrompt),
	})
	if err := runtime.Context.Err(); err != nil {
		return zero, failDirectCodingSemanticCall(
			runtime, modelName, subject, 0, fmt.Errorf("authority ended: %w", err),
		)
	}
	result, err := runtime.Execute(job, modelName)
	if err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 1, err)
	}
	validationErr := result.ValidateFor(job)
	var value T
	if validationErr == nil {
		validationErr = validateDirectCodingSemanticCandidatePathBoundary(
			job.Kind, result.Candidate, runtime.PathProvenance,
		)
	}
	if validationErr == nil {
		value, validationErr = decode(result.Candidate)
	}
	if validationErr == nil {
		if boundary, ok := any(value).(interface {
			ValidatePathFree(assemblyline.ArtifactIdentityProvenance) error
		}); ok {
			validationErr = boundary.ValidatePathFree(runtime.PathProvenance)
		}
	}
	validationErr = finalizeTypedWorkerResult(runtime, job, result, validationErr)
	if validationErr != nil {
		emitDirectCodingSemanticRejection(runtime, modelName, subject, 1, validationErr)
		return zero, failDirectCodingSemanticCall(
			runtime, modelName, subject, 1,
			&directCodingSemanticLeafRejection{subject: subject, err: validationErr},
		)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return value, nil
}
