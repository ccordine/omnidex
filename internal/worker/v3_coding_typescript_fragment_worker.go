package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingTypeScriptRequiredChange = "Fix only the observed local validation failure in the current declaration. Preserve all unrelated executable behavior."

func runDirectCodingTypeScriptFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingTypeScriptFragmentJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("TypeScript fragment worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return "", fmt.Errorf("TypeScript fragment worker attempts must be between 1 and %d", maxTypedWorkerAttempts)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("TypeScript fragment worker requires one configured model")
	}
	baseJob, err := newDirectCodingTypeScriptPortableJob(job)
	if err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	var lastErr error
	var lastCandidate string
	attemptsUsed := 0
	seenCandidates := make(map[string]struct{})
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		attemptsUsed = attempt
		if err := runtime.Context.Err(); err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, attempt-1, err)
		}
		attemptJob := baseJob
		attemptModel := modelName
		if lastErr != nil {
			attemptModel = strings.TrimSpace(runtime.CorrectionModel)
			if attemptModel == "" {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, modelName, job.block.ID, attempt-1,
					fmt.Errorf("TypeScript fragment correction requires one configured correction model"),
				)
			}
			retry := job
			retry.current = lastCandidate
			retry.failure = directCodingTypeScriptFragmentFailure(job.failure, lastErr)
			retry.requiredChange = directCodingTypeScriptRequiredChangeFor(lastErr)
			attemptJob, err = newDirectCodingTypeScriptPortableJob(retry)
			if err != nil {
				return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt-1, err)
			}
		}
		prompt, _, err := assemblyline.RenderPortableJob(attemptJob)
		if err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt-1, err)
		}
		currentBytes := len(strings.TrimSpace(job.current))
		correctionBytes := len(strings.TrimSpace(job.failure))
		if lastErr != nil {
			currentBytes = len(lastCandidate)
			correctionBytes = len(strings.TrimSpace(lastErr.Error()))
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.block.ID,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			PromptBytes: len(prompt), CapabilityBytes: len(strings.TrimSpace(job.available)),
			CurrentBytes: currentBytes, CorrectionBytes: correctionBytes,
		})

		result, err := runtime.Execute(attemptJob, attemptModel)
		if err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
		}
		if err = result.ValidateFor(attemptJob); err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
		}
		lastCandidate = directCodingCorrectionCandidate(result.Candidate)
		if err == nil {
			if _, duplicate := seenCandidates[lastCandidate]; duplicate {
				err = fmt.Errorf("repeated identical candidate rejected; the correction made no progress")
			} else {
				seenCandidates[lastCandidate] = struct{}{}
			}
		}
		if err == nil {
			var raw string
			raw, err = normalizeDirectCodingTypeScriptResponse(result.Candidate)
			if err == nil {
				var fragment assemblyline.TypeScriptFragment
				fragment, err = assemblyline.ParseTypeScriptFunction(assemblyline.TypeScriptFunctionContract{
					Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
				}, raw)
				if err == nil {
					source := strings.TrimSpace(fragment.Source)
					if strings.TrimSpace(job.current) != "" && source == strings.TrimSpace(job.current) {
						err = fmt.Errorf("unchanged correction rejected; modify the current declaration to resolve the original failure")
					} else {
						emitTypedWorker(runtime, typedWorkerEvent{
							State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.block.ID,
							Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
						})
						return source, nil
					}
				}
			}
		}
		lastErr = err
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.block.ID,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			Detail: trimForBudget(err.Error(), 1200),
		})
		if strings.Contains(err.Error(), "repeated identical candidate") {
			break
		}
	}
	return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, attemptsUsed, fmt.Errorf(
		"failed after %d bounded attempts: %w", attemptsUsed, lastErr,
	))
}

func directCodingCorrectionCandidate(raw string) string {
	candidate := strings.TrimSpace(raw)
	if normalized, err := normalizeDirectCodingTypeScriptResponse(candidate); err == nil {
		return normalized
	}
	return candidate
}

func newDirectCodingTypeScriptPortableJob(job directCodingTypeScriptFragmentJob) (assemblyline.PortableJob, error) {
	capabilities := make([]string, 0, 1)
	if available := strings.TrimSpace(job.available); available != "" {
		capabilities = append(capabilities, available)
	}
	if strings.TrimSpace(job.current) == "" {
		return assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
			Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
			Behavior: strings.TrimSpace(job.block.Contract), Capabilities: capabilities,
			PermittedSymbols: append([]string(nil), job.block.Globals...),
		})
	}
	diagnostic := directCodingTypeScriptModelFailure(job.failure)
	requiredChange := strings.TrimSpace(job.requiredChange)
	if requiredChange == "" {
		requiredChange = directCodingTypeScriptRequiredChange
	}
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		Capabilities: capabilities, PermittedSymbols: append([]string(nil), job.block.Globals...),
		CurrentDeclaration: strings.TrimSpace(job.current),
		RequiredChange:     requiredChange,
		Diagnostic:         diagnostic,
	})
}

func directCodingTypeScriptRequiredChangeFor(err error) string {
	if instruction, ok := assemblyline.TypeScriptFragmentCorrectionInstruction(err); ok {
		return instruction
	}
	return directCodingTypeScriptRequiredChange
}

func failDirectCodingTypeScriptFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	blockID string,
	attempt int,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: blockID,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("TypeScript fragment worker failed: %w", err)
}
