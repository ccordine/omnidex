package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

const directCodingGoGenerationRequiredChange = "Fix only the observed local declaration validation failure while preserving the exact signature and local behavior."

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
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return "", fmt.Errorf("Go fragment generation attempts must be between 1 and %d", maxTypedWorkerAttempts)
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
	seen := make(map[string]struct{})
	var lastCandidate string
	var lastErr error
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, attempt-1, err)
		}
		attemptJob, attemptModel := baseJob, modelName
		if lastErr != nil {
			attemptModel = strings.TrimSpace(runtime.CorrectionModel)
			if attemptModel == "" {
				return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, attempt-1,
					fmt.Errorf("Go fragment correction requires one configured correction model"))
			}
			attemptJob, err = assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
				Language: "go", Signature: job.Input.Signature,
				Capabilities:       append([]string(nil), job.Input.Capabilities...),
				PermittedSymbols:   append([]string(nil), job.Input.PermittedSymbols...),
				CurrentDeclaration: lastCandidate,
				RequiredChange:     directCodingGoGenerationRequiredChange,
				Diagnostic:         trimForBudget(lastErr.Error(), 1200),
			})
			if err != nil {
				return "", failDirectCodingGoGeneration(runtime, attemptModel, job.Subject, attempt-1, err)
			}
		}
		prompt, _, err := assemblyline.RenderPortableJob(attemptJob)
		if err != nil {
			return "", failDirectCodingGoGeneration(runtime, attemptModel, job.Subject, attempt-1, err)
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			PromptBytes: len(prompt), CapabilityBytes: goGenerationCapabilityBytes(job.Input),
		})
		result, err := runtime.Execute(attemptJob, attemptModel)
		if err != nil {
			return "", failDirectCodingGoGeneration(runtime, attemptModel, job.Subject, attempt, err)
		}
		if err := result.ValidateFor(attemptJob); err != nil {
			err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
			return "", failDirectCodingGoGeneration(runtime, attemptModel, job.Subject, attempt, err)
		}
		rawCandidate := strings.TrimSpace(result.Candidate)
		lastCandidate = rawCandidate
		projected, projectionErr := gofragment.ProjectFunctionModelResponse(rawCandidate)
		if projectionErr == nil {
			lastCandidate = projected
		}
		if _, duplicate := seen[lastCandidate]; duplicate {
			lastErr = fmt.Errorf("repeated identical candidate rejected; the correction made no progress")
		} else {
			seen[lastCandidate] = struct{}{}
			rejectedCandidate := lastCandidate
			lastErr = projectionErr
			if lastErr == nil {
				lastCandidate, lastErr = gofragment.ParseNewFunction(
					job.Input.Signature, job.Input.PermittedSymbols, lastCandidate,
				)
			}
			if lastErr == nil {
				if err = finalizeTypedWorkerResult(runtime, attemptJob, result, nil); err != nil {
					return "", failDirectCodingGoGeneration(runtime, attemptModel, job.Subject, attempt, err)
				}
				emitTypedWorker(runtime, typedWorkerEvent{
					State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
					Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
				})
				return lastCandidate, nil
			}
			lastCandidate = rejectedCandidate
		}
		lastErr = finalizeTypedWorkerResult(runtime, attemptJob, result, lastErr)
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			Detail: trimForBudget(lastErr.Error(), 1200),
		})
		if strings.Contains(lastErr.Error(), "repeated identical candidate") {
			break
		}
	}
	return "", failDirectCodingGoGeneration(runtime, modelName, job.Subject, runtime.MaxAttempts,
		fmt.Errorf("failed after %d bounded attempts: %w", runtime.MaxAttempts, lastErr))
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
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("Go fragment generation worker failed: %w", err)
}
