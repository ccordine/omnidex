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
	if validationErr == nil {
		validationErr = validate(value)
	}
	validationErr = finalizeTypedWorkerResult(runtime, job, result, validationErr)
	if validationErr != nil {
		emitDirectCodingSemanticRejection(runtime, modelName, subject, 1, validationErr)
		return zero, failDirectCodingSemanticCall(
			runtime, modelName, subject, 1,
			fmt.Errorf("invalid %s semantic result: %w", subject, validationErr),
		)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return value, nil
}
