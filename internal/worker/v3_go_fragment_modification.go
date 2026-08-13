package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

const directCodingGoRequiredChange = "Fix only the observed local validation failure in the current declaration. Preserve all unrelated executable behavior."

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
	contract := gofragment.Contract{
		Signature: job.Input.Signature, Current: job.Input.CurrentDeclaration,
		PermittedSymbols: append([]string(nil), job.Input.PermittedSymbols...),
	}
	currentCanonical, err := gofragment.ParseFunction(contract, job.Input.CurrentDeclaration)
	if err != nil {
		return "", failDirectCodingGoModification(runtime, modelName, job.Subject, 0,
			fmt.Errorf("validate current Go declaration: %w", err))
	}
	seen := make(map[string]struct{})
	var lastCandidate string
	var lastErr error
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return "", failDirectCodingGoModification(runtime, modelName, job.Subject, attempt-1, err)
		}
		attemptJob := baseJob
		attemptModel := modelName
		if lastErr != nil {
			attemptModel = strings.TrimSpace(runtime.CorrectionModel)
			if attemptModel == "" {
				return "", failDirectCodingGoModification(runtime, modelName, job.Subject, attempt-1,
					fmt.Errorf("Go fragment correction requires one configured correction model"))
			}
			attemptJob, err = newDirectCodingGoCorrectionJob(job.Input, lastCandidate, lastErr)
			if err != nil {
				return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt-1, err)
			}
		}
		prompt, _, err := assemblyline.RenderPortableJob(attemptJob)
		if err != nil {
			return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt-1, err)
		}
		currentBytes := len([]byte(job.Input.CurrentDeclaration))
		correctionBytes := 0
		if lastErr != nil {
			currentBytes = len([]byte(lastCandidate))
			correctionBytes = len([]byte(strings.TrimSpace(lastErr.Error())))
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			PromptBytes: len(prompt), CurrentBytes: currentBytes, CorrectionBytes: correctionBytes,
			CapabilityBytes: goModificationCapabilityBytes(job.Input),
		})
		result, err := runtime.Execute(attemptJob, attemptModel)
		if err != nil {
			return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt, err)
		}
		if err := result.ValidateFor(attemptJob); err != nil {
			err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
			return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt, err)
		}
		lastCandidate = strings.TrimSpace(result.Candidate)
		if _, duplicate := seen[lastCandidate]; duplicate {
			lastErr = fmt.Errorf("repeated identical candidate rejected; the correction made no progress")
		} else {
			seen[lastCandidate] = struct{}{}
			var parsed string
			parsed, lastErr = gofragment.ParseFunction(contract, lastCandidate)
			if lastErr == nil {
				if parsed == currentCanonical {
					lastErr = fmt.Errorf("unchanged modification rejected; the declaration must satisfy the registered requirement")
				} else {
					if err = finalizeTypedWorkerResult(runtime, attemptJob, result, nil); err != nil {
						return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt, err)
					}
					emitTypedWorker(runtime, typedWorkerEvent{
						State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
						Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
					})
					return parsed, nil
				}
			}
		}
		lastErr = finalizeTypedWorkerResult(runtime, attemptJob, result, lastErr)
		if lastErr == nil {
			return "", failDirectCodingGoModification(runtime, attemptModel, job.Subject, attempt,
				fmt.Errorf("Go fragment rejection lost its exact failure"))
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			Detail: trimForBudget(lastErr.Error(), 1200),
		})
		if strings.Contains(lastErr.Error(), "repeated identical candidate") {
			break
		}
	}
	return "", failDirectCodingGoModification(runtime, modelName, job.Subject, runtime.MaxAttempts,
		fmt.Errorf("failed after %d bounded attempts: %w", runtime.MaxAttempts, lastErr))
}

func newDirectCodingGoCorrectionJob(
	input assemblyline.FragmentModificationInput,
	current string,
	failure error,
) (assemblyline.PortableJob, error) {
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "go", Signature: input.Signature,
		Capabilities:       append([]string(nil), input.Capabilities...),
		PermittedSymbols:   append([]string(nil), input.PermittedSymbols...),
		CurrentDeclaration: strings.TrimSpace(current),
		RequiredChange:     directCodingGoRequiredChange,
		Diagnostic:         trimForBudget(strings.TrimSpace(failure.Error()), 1200),
	})
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
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("Go fragment modification worker failed: %w", err)
}
