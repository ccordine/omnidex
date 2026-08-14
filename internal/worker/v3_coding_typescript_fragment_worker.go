package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingTypeScriptRequiredChange = "Fix only the observed local validation failure in the current declaration. Preserve all unrelated executable behavior."

var errDirectCodingTypeScriptUnchangedCorrection = errors.New(
	"unchanged correction rejected; modify the current declaration to resolve the original failure",
)

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
		var repairRegion *assemblyline.TypeScriptFragmentRepairRegion
		if lastErr != nil {
			attemptModel = strings.TrimSpace(runtime.CorrectionModel)
			if attemptModel == "" {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, modelName, job.block.ID, attempt-1,
					fmt.Errorf("TypeScript fragment correction requires one configured correction model"),
				)
			}
			retry := job
			if failure, localized := assemblyline.TypeScriptSyntaxFailureFromError(lastErr); localized {
				region, regionErr := assemblyline.NewTypeScriptFragmentRepairRegion(
					lastCandidate, failure, 2*(attempt-1),
				)
				if regionErr != nil {
					return "", failDirectCodingTypeScriptFragmentWorker(
						runtime, attemptModel, job.block.ID, attempt-1,
						fmt.Errorf("localize TypeScript syntax repair: %w", regionErr),
					)
				}
				retry.current = ""
				retry.repairRegion = &region
				repairRegion = &region
			} else {
				retry.current = lastCandidate
				retry.repairRegion = nil
			}
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
			if repairRegion != nil {
				currentBytes = len(repairRegion.Source)
			} else {
				currentBytes = len(lastCandidate)
			}
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
			err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
		}
		candidate := directCodingCorrectionCandidate(result.Candidate)
		if repairRegion != nil {
			var replacement string
			replacement, err = assemblyline.DecodeTypeScriptFragmentRepairDecision(
				*repairRegion, result.Candidate,
			)
			if err == nil {
				candidate, err = assemblyline.ApplyTypeScriptFragmentRepairRegion(
					lastCandidate, *repairRegion, replacement,
				)
			}
		} else {
			candidate, err = assemblyline.DecodeTypeScriptFunctionModelResponse(
				assemblyline.TypeScriptFunctionContract{
					Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
				},
				result.Candidate,
			)
		}
		if candidate != "" {
			lastCandidate = candidate
			if _, duplicate := seenCandidates[lastCandidate]; duplicate {
				err = fmt.Errorf("repeated identical candidate rejected; the correction made no progress")
			} else {
				seenCandidates[lastCandidate] = struct{}{}
			}
		}
		if err == nil {
			candidate := strings.TrimSpace(lastCandidate)
			var fragment assemblyline.TypeScriptFragment
			fragment, err = assemblyline.ParseTypeScriptFunction(assemblyline.TypeScriptFunctionContract{
				Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
			}, candidate)
			if err == nil {
				source := strings.TrimSpace(fragment.Source)
				if strings.TrimSpace(job.current) != "" && source == strings.TrimSpace(job.current) {
					err = errDirectCodingTypeScriptUnchangedCorrection
				} else {
					if err = finalizeTypedWorkerResult(runtime, attemptJob, result, nil); err != nil {
						return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
					}
					emitTypedWorker(runtime, typedWorkerEvent{
						State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.block.ID,
						Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
					})
					return source, nil
				}
			}
		}
		rejectionErr := err
		if rejectionErr == nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt,
				fmt.Errorf("TypeScript fragment rejection lost its exact failure"))
		}
		if runtime.Finalize != nil {
			if persistErr := runtime.Finalize(attemptJob, result, rejectionErr); persistErr != nil {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, attemptModel, job.block.ID, attempt,
					fmt.Errorf("persist TypeScript fragment rejection: %w", persistErr),
				)
			}
		}
		lastErr = rejectionErr
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.block.ID,
			Model: attemptModel, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			Detail: trimForBudget(rejectionErr.Error(), 1200),
		})
		if repairRegion != nil {
			if _, remainsSyntaxLocal := assemblyline.TypeScriptSyntaxFailureFromError(rejectionErr); !remainsSyntaxLocal {
				break
			}
		}
		if strings.Contains(rejectionErr.Error(), "repeated identical candidate") {
			break
		}
		if errors.Is(rejectionErr, errDirectCodingTypeScriptUnchangedCorrection) {
			break
		}
	}
	return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, attemptsUsed, fmt.Errorf(
		"failed after %d bounded attempts: %w", attemptsUsed, lastErr,
	))
}

func directCodingCorrectionCandidate(raw string) string {
	return strings.TrimSpace(raw)
}

func newDirectCodingTypeScriptPortableJob(job directCodingTypeScriptFragmentJob) (assemblyline.PortableJob, error) {
	capabilities := make([]string, 0, 1)
	if available := strings.TrimSpace(job.available); available != "" && job.repairRegion == nil {
		capabilities = append(capabilities, available)
	}
	permittedSymbols := append([]string(nil), job.block.Globals...)
	if job.repairRegion != nil {
		permittedSymbols = nil
	}
	if strings.TrimSpace(job.current) == "" && job.repairRegion == nil {
		return assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
			Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
			Behavior: strings.TrimSpace(job.block.Contract), Capabilities: capabilities,
			PermittedSymbols: permittedSymbols,
		})
	}
	diagnostic := directCodingTypeScriptModelFailure(job.failure)
	requiredChange := strings.TrimSpace(job.requiredChange)
	if requiredChange == "" {
		requiredChange = directCodingTypeScriptRequiredChange
	}
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		Capabilities: capabilities, PermittedSymbols: permittedSymbols,
		CurrentDeclaration: strings.TrimSpace(job.current),
		RepairRegion:       job.repairRegion,
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
