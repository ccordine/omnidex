package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type portableSemanticLeafDecoder[T any] func(string) (T, error)

type portableSemanticLeafRejection struct {
	subject string
	err     error
}

func (rejection *portableSemanticLeafRejection) Error() string {
	return fmt.Sprintf("invalid %s semantic result: %v", rejection.subject, rejection.err)
}

func (rejection *portableSemanticLeafRejection) Unwrap() error {
	return rejection.err
}

// runPortableSemanticLeafCall resolves one raw semantic value. The
// station-specific decoder is the only authority that may turn model bytes
// into typed state; this boundary never decodes a generic object or record.
func runPortableSemanticLeafCall[T any](
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	job assemblyline.PortableJob,
	identities []assemblyline.ArtifactIdentity,
	decode portableSemanticLeafDecoder[T],
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil || decode == nil {
		return zero, fmt.Errorf("portable semantic leaf requires an exact portable runtime and decoder")
	}
	deterministic, inferenceFree, err := assemblyline.ResolvePortableJobWithoutInference(job)
	if err != nil {
		return zero, err
	}
	basePrompt := ""
	if !inferenceFree {
		if modelName == "" {
			return zero, fmt.Errorf("portable semantic leaf requires one configured model")
		}
		basePrompt, err = assemblyline.RenderPortableJob(job)
		if err != nil {
			return zero, err
		}
		if err := validatePortableSemanticPrompt(
			basePrompt, identities, runtime.PathProvenance,
		); err != nil {
			return zero, err
		}
	}
	observation, err := beginSemanticObservation(runtime, job, modelName, subject, basePrompt, inferenceFree, deterministic.Candidate)
	if err != nil {
		return zero, err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		PromptBytes: len(basePrompt),
	})
	if err := runtime.Context.Err(); err != nil {
		return zero, failPortableSemanticCall(
			runtime, modelName, subject, 0, fmt.Errorf("authority ended: %w", err),
		)
	}
	result, err := runtime.Execute(job, modelName)
	if err != nil {
		return zero, failPortableSemanticCall(runtime, modelName, subject, 1, err)
	}
	callID := int64(0)
	if runtime.ReadCallEvidenceID != nil {
		callID = runtime.ReadCallEvidenceID(job)
	}
	validationErr := result.ValidateFor(job)
	var value T
	if validationErr == nil {
		validationErr = validatePortableSemanticCandidatePathBoundary(
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
		emitPortableSemanticRejection(runtime, modelName, subject, 1, validationErr)
		return zero, failPortableSemanticCall(
			runtime, modelName, subject, 1,
			&portableSemanticLeafRejection{subject: subject, err: validationErr},
		)
	}
	completeSemanticObservation(runtime, observation, result.Candidate, callID)
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return value, nil
}
