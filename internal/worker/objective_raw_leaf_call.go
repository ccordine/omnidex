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
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, err
	}
	if err := validateDirectCodingSemanticPrompt(
		prompt, nil, runtime.PathProvenance,
	); err != nil {
		return zero, err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1, PromptBytes: len(prompt),
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
		validationErr = validateObjectiveRawCandidatePathBoundary(
			job.Kind, result.Candidate, runtime.PathProvenance,
		)
	}
	if validationErr == nil {
		value, validationErr = decode(result.Candidate)
	}
	if validationErr == nil {
		validationErr = validateObjectiveRawLeafPathBoundary(value, runtime.PathProvenance)
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
