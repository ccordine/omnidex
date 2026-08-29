package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

const directCodingGoModelAttempts = 1

type directCodingGoGenerationJob struct {
	Subject string
	Input   assemblyline.FragmentGenerationInput
}

func runDirectCodingGoFragmentGenerationWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingGoGenerationJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("Go fragment generation worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts != exactSemanticLeafCalls {
		return "", fmt.Errorf("Go fragment generation requires exactly %d model call", exactSemanticLeafCalls)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || strings.TrimSpace(job.Subject) == "" {
		return "", fmt.Errorf("Go fragment generation requires one model and opaque subject")
	}
	if job.Input.Language != "go" {
		return "", fmt.Errorf("Go fragment generation received language %q", job.Input.Language)
	}
	baseJob, err := assemblyline.NewFragmentGenerationJob(job.Input)
	if err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 0, err)
	}
	if err := job.Input.ValidatePathFree(runtime.PathProvenance); err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 0, err)
	}
	if err := runtime.Context.Err(); err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 0, err)
	}
	prompt, err := assemblyline.RenderPortableJob(baseJob)
	if err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 0, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
		PromptBytes: len(prompt), CapabilityBytes: goGenerationCapabilityBytes(job.Input),
	})
	baseJob, result, err := executeInitialFragmentGenerationWithReplacement(
		runtime, baseJob, job.Input, modelName,
	)
	if err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 1, err)
	}
	if err := result.ValidateFor(baseJob); err != nil {
		err = finalizeTypedWorkerResult(runtime, baseJob, result, err)
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 1, err)
	}
	projection, candidateErr := projectDirectCodingGoFragment(result.Candidate)
	candidate := projection.Source
	if candidateErr == nil {
		result.Projection = &projection
		permitted := append([]string(nil), job.Input.Capabilities...)
		permitted = append(permitted, job.Input.PermittedSymbols...)
		_, candidateErr = gofragment.ParseNewFunction(job.Input.Signature, permitted, candidate)
	}
	if candidateErr == nil {
		candidateErr = assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"Go fragment candidate", runtime.PathProvenance, candidate,
		)
	}
	if candidateErr != nil {
		candidateErr = finalizeTypedWorkerResult(runtime, baseJob, result, candidateErr)
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
			Detail: trimForBudget(candidateErr.Error(), 1200),
		})
		return "", failDirectCodingGoGeneration(
			runtime, modelName, job.Subject, 1,
			fmt.Errorf("initial candidate rejected: %w", candidateErr),
		)
	}
	if err = finalizeTypedWorkerResult(runtime, baseJob, result, nil); err != nil {
		return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, 1, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
	})
	return candidate, nil
}

func goGenerationCapabilityBytes(input assemblyline.FragmentGenerationInput) int {
	return len(strings.Join(input.Capabilities, "\n")) + len(strings.Join(input.PermittedSymbols, "\n"))
}

func failDirectCodingGoGeneration(
	runtime typedWorkerRuntime,
	modelName, subject string,
	attempt int,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: directCodingGoModelAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("Go fragment generation worker failed: %w", err)
}
