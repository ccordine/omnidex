package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

type directCodingGoModificationJob struct {
	Subject string
	Input   assemblyline.FragmentModificationInput
}

func runDirectCodingGoFragmentModificationWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingGoModificationJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("Go fragment modification worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return "", fmt.Errorf("Go fragment modification attempts must be between 1 and %d", maxTypedWorkerAttempts)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || strings.TrimSpace(job.Subject) == "" {
		return "", fmt.Errorf("Go fragment modification requires one model and subject")
	}
	baseJob, err := assemblyline.NewFragmentModificationJob(job.Input)
	if err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0, err)
	}
	if err := job.Input.ValidatePathFree(runtime.PathProvenance); err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0, err)
	}
	contract := gofragment.Contract{
		Signature: job.Input.Signature, Current: job.Input.CurrentDeclaration,
		PermittedSymbols: append([]string(nil), job.Input.PermittedSymbols...),
	}
	currentCanonical, err := gofragment.ParseFunction(contract, job.Input.CurrentDeclaration)
	if err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0,
			fmt.Errorf("validate current Go declaration: %w", err))
	}
	if err := runtime.Context.Err(); err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0, err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(baseJob)
	if err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
		PromptBytes: len(prompt), CurrentBytes: len([]byte(job.Input.CurrentDeclaration)),
		CapabilityBytes: goModificationCapabilityBytes(job.Input),
	})
	result, err := runtime.Execute(baseJob, modelName)
	if err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 1, err)
	}
	if err := result.ValidateFor(baseJob); err != nil {
		err = finalizeTypedWorkerResult(runtime, baseJob, result, err)
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 1, err)
	}
	candidate, candidateErr := gofragment.ProjectFunctionModelResponse(strings.TrimSpace(result.Candidate))
	if candidateErr == nil {
		candidate, candidateErr = gofragment.ParseFunction(contract, candidate)
	}
	if candidateErr == nil {
		candidateErr = assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"Go fragment candidate", runtime.PathProvenance, candidate,
		)
	}
	if candidateErr == nil && candidate == currentCanonical {
		candidateErr = fmt.Errorf(
			"unchanged modification rejected; the declaration must satisfy the registered requirement",
		)
	}
	if candidateErr != nil {
		candidateErr = finalizeTypedWorkerResult(runtime, baseJob, result, candidateErr)
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
			Detail: trimForBudget(candidateErr.Error(), 1200),
		})
		return "", failDirectCodingGoModification(
			runtime, modelName, job.Subject, 1,
			fmt.Errorf("initial candidate rejected: %w", candidateErr),
		)
	}
	if err = finalizeTypedWorkerResult(runtime, baseJob, result, nil); err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 1, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingGoModelAttempts,
	})
	return candidate, nil
}

func goModificationCapabilityBytes(input assemblyline.FragmentModificationInput) int {
	return len(strings.Join(input.Capabilities, "\n")) + len(strings.Join(input.PermittedSymbols, "\n"))
}

func failDirectCodingGoModification(
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
	return fmt.Errorf("Go fragment modification worker failed: %w", err)
}
