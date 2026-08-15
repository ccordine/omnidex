package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	directCodingTypeScriptRequiredChange         = "Fix only the observed local validation failure in the current declaration. Preserve all unrelated executable behavior."
	directCodingTypeScriptCompilerRequiredChange = "Eliminate the exact compiler failure using only bindings available at the failing expression. If the invalid value crosses a nested scope, restructure only the supplied repair region. Preserve unrelated behavior."
	directCodingTypeScriptDeclarationReviewBytes = 5 * 1024
	maxTypeScriptOutputReviewBytes               = 128 * 1024
)

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
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("TypeScript fragment worker requires one configured model")
	}
	baseJob, err := newDirectCodingTypeScriptPortableJob(job)
	if err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	var lastErr error
	lastCandidate := strings.TrimSpace(job.current)
	progress := newDirectCodingTypeScriptCorrectionProgress()
	var syntaxProgress directCodingTypeScriptSyntaxProgress
	var nextSyntaxRepair directCodingTypeScriptSyntaxRepair
	for attempt := 1; ; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, attempt-1, err)
		}
		previousCandidate := lastCandidate
		correctionAttempt := lastErr != nil || strings.TrimSpace(job.current) != ""
		attemptJob := baseJob
		attemptModel := modelName
		repairRegion := job.repairRegion
		if lastErr != nil {
			if strings.TrimSpace(job.repairGuidance) != "" {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, attemptModel, job.block.ID, attempt-1,
					fmt.Errorf(
						"guided TypeScript repair executor returned invalid source: %w",
						lastErr,
					),
				)
			}
			repairRegion = nil
			attemptModel = strings.TrimSpace(runtime.CorrectionModel)
			if attemptModel == "" {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, modelName, job.block.ID, attempt-1,
					fmt.Errorf("TypeScript fragment correction requires one configured correction model"),
				)
			}
			retry := job
			if failure, localized := assemblyline.TypeScriptSyntaxFailureFromError(lastErr); localized {
				if nextSyntaxRepair.wholeDeclaration {
					retry.current = lastCandidate
					retry.repairRegion = nil
				} else if nextSyntaxRepair.radius < 1 {
					return "", failDirectCodingTypeScriptFragmentWorker(
						runtime, attemptModel, job.block.ID, attempt-1,
						fmt.Errorf("TypeScript syntax correction lost its code-owned repair radius"),
					)
				} else {
					region, regionErr := assemblyline.NewTypeScriptFragmentRepairRegion(
						lastCandidate, failure, nextSyntaxRepair.radius,
					)
					if regionErr != nil {
						if !errors.Is(regionErr, assemblyline.ErrTypeScriptRepairRegionUnrepresentable) {
							return "", failDirectCodingTypeScriptFragmentWorker(
								runtime, attemptModel, job.block.ID, attempt-1,
								fmt.Errorf("localize TypeScript syntax repair: %w", regionErr),
							)
						}
						retry.current = lastCandidate
						retry.repairRegion = nil
					} else {
						retry.current = ""
						retry.repairRegion = &region
						repairRegion = &region
					}
				}
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
		if repairRegion != nil {
			currentBytes = len(repairRegion.Source)
		}
		correctionBytes := len(strings.TrimSpace(job.failure))
		if strings.TrimSpace(job.repairGuidance) != "" {
			correctionBytes = len(strings.TrimSpace(job.repairGuidance))
		}
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
			Model: attemptModel, Attempt: attempt,
			PromptBytes: len(prompt), CapabilityBytes: len(strings.TrimSpace(job.available)),
			CurrentBytes: currentBytes, CorrectionBytes: correctionBytes,
			Warning: directCodingTypeScriptDeclarationSizeWarning(currentBytes),
		})

		result, err := runtime.Execute(attemptJob, attemptModel)
		if err != nil {
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
		}
		if err = result.ValidateFor(attemptJob); err != nil {
			err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
			return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
		}
		rawCandidate := directCodingCorrectionCandidate(result.Candidate)
		candidate := rawCandidate
		if repairRegion != nil {
			var replacement string
			replacement, err = assemblyline.ProjectTypeScriptFragmentRepairResponse(
				*repairRegion, result.Candidate,
			)
			if err == nil {
				candidate, err = assemblyline.ApplyTypeScriptFragmentRepairRegion(
					lastCandidate, *repairRegion, replacement,
				)
			}
		} else {
			var projection assemblyline.TypeScriptFunctionProjection
			projection, err = assemblyline.ProjectTypeScriptFunctionModelResponse(
				assemblyline.TypeScriptFunctionContract{
					Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
				},
				result.Candidate,
			)
			candidate = projection.Source
			if err == nil {
				var portableProjection assemblyline.PortableResultProjection
				portableProjection, err = projection.PortableResultProjection()
				if err == nil {
					result.Projection = &portableProjection
				}
			}
		}
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			lastCandidate = candidate
		}
		currentAuthority := strings.TrimSpace(job.current)
		if lastErr != nil {
			currentAuthority = strings.TrimSpace(previousCandidate)
		}
		if correctionAttempt && candidate != "" && candidate == currentAuthority {
			err = errDirectCodingTypeScriptUnchangedCorrection
		}
		if err == nil {
			candidate := strings.TrimSpace(lastCandidate)
			_, err = assemblyline.ParseTypeScriptFunction(assemblyline.TypeScriptFunctionContract{
				Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
			}, candidate)
			if err == nil {
				source := candidate
				if correctionAttempt && source == currentAuthority {
					err = errDirectCodingTypeScriptUnchangedCorrection
				} else {
					if err = finalizeTypedWorkerResult(runtime, attemptJob, result, nil); err != nil {
						return "", failDirectCodingTypeScriptFragmentWorker(runtime, attemptModel, job.block.ID, attempt, err)
					}
					emitTypedWorker(runtime, typedWorkerEvent{
						State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.block.ID,
						Model: attemptModel, Attempt: attempt,
						Warning: joinTypedWorkerWarnings(
							directCodingTypeScriptDeclarationSizeWarning(len(source)),
							directCodingTypeScriptProjectionWarning(result.Projection),
						),
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
		terminal := errors.Is(rejectionErr, errDirectCodingTypeScriptUnchangedCorrection)
		if !terminal {
			progressCandidate := strings.TrimSpace(lastCandidate)
			if progressCandidate == "" {
				progressCandidate = rawCandidate
			}
			verificationStage := "fragment_validation"
			if syntaxFailure, syntaxRejected := assemblyline.TypeScriptSyntaxFailureFromError(rejectionErr); syntaxRejected {
				nextSyntaxRepair = syntaxProgress.next(syntaxFailure)
				verificationStage = nextSyntaxRepair.verificationStage()
			} else {
				nextSyntaxRepair = directCodingTypeScriptSyntaxRepair{}
			}
			if progressErr := progress.observe(
				job.block.ID, progressCandidate, verificationStage, rejectionErr.Error(),
			); progressErr != nil {
				rejectionErr = progressErr
				terminal = true
			}
		}
		if runtime.Finalize != nil {
			if persistErr := runtime.Finalize(attemptJob, result, rejectionErr); persistErr != nil {
				return "", failDirectCodingTypeScriptFragmentWorker(
					runtime, attemptModel, job.block.ID, attempt,
					fmt.Errorf("persist TypeScript fragment rejection: %w", persistErr),
				)
			}
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.block.ID,
			Model: attemptModel, Attempt: attempt,
			Warning: joinTypedWorkerWarnings(
				directCodingTypeScriptDeclarationSizeWarning(len(lastCandidate)),
				directCodingTypeScriptProjectionWarning(result.Projection),
			),
			Detail: trimForBudget(rejectionErr.Error(), 1200),
		})
		if terminal {
			return "", failDirectCodingTypeScriptFragmentWorker(
				runtime, attemptModel, job.block.ID, attempt, rejectionErr,
			)
		}
		lastErr = rejectionErr
	}
}

func directCodingCorrectionCandidate(raw string) string {
	return strings.TrimSpace(raw)
}

func newDirectCodingTypeScriptPortableJob(job directCodingTypeScriptFragmentJob) (assemblyline.PortableJob, error) {
	guidance := strings.TrimSpace(job.repairGuidance)
	capabilities := make([]string, 0, 1)
	compilerRegion := job.repairRegion != nil &&
		job.repairRegion.Kind == assemblyline.TypeScriptRepairRegionCompilerOwner
	if available := strings.TrimSpace(job.available); guidance == "" && available != "" &&
		(job.repairRegion == nil || compilerRegion) {
		capabilities = append(capabilities, available)
	}
	permittedSymbols := append([]string(nil), job.block.Globals...)
	if guidance != "" || (job.repairRegion != nil && !compilerRegion) {
		permittedSymbols = nil
	}
	if strings.TrimSpace(job.current) == "" && job.repairRegion == nil {
		return assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
			Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
			Behavior: strings.TrimSpace(job.block.Contract), Capabilities: capabilities,
			PermittedSymbols: permittedSymbols,
		})
	}
	if guidance != "" {
		portableCurrent := strings.TrimSpace(job.current)
		if job.repairRegion != nil {
			portableCurrent = ""
		}
		return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
			Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
			CurrentDeclaration: portableCurrent, RepairRegion: job.repairRegion,
			RepairGuidance: guidance,
		})
	}
	diagnostic := strings.TrimSpace(job.failure)
	if diagnostic == "" {
		return assemblyline.PortableJob{}, fmt.Errorf("TypeScript fragment correction requires one exact model failure")
	}
	if directCodingTypeScriptCompilerContainsPathIdentity(diagnostic) {
		return assemblyline.PortableJob{}, fmt.Errorf("TypeScript fragment correction failure must be path-free")
	}
	requiredChange := strings.TrimSpace(job.requiredChange)
	if requiredChange == "" {
		requiredChange = directCodingTypeScriptRequiredChange
		if compilerRegion {
			requiredChange = directCodingTypeScriptCompilerRequiredChange
		}
	}
	portableCurrent := strings.TrimSpace(job.current)
	if job.repairRegion != nil {
		portableCurrent = ""
	}
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		Capabilities: capabilities, PermittedSymbols: permittedSymbols,
		CurrentDeclaration: portableCurrent,
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
		Model: modelName, Attempt: attempt,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("TypeScript fragment worker failed: %w", err)
}
