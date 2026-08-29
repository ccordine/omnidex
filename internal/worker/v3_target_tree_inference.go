package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// One initial tree and, only after one exact code-proven defect, one complete
// replacement tree. There is no generic semantic retry budget.
const maxDirectCodingTargetTreeCalls = 2

func runDirectCodingTargetTreeCall(
	runtime typedWorkerRuntime,
	initialModel string,
	replacementModel string,
	input assemblyline.TargetTreeInput,
	existingFiles []string,
	stack directCodingProjectStack,
) (assemblyline.TargetTree, error) {
	return runDirectCodingTargetTreeCallWithValidator(
		runtime, initialModel, replacementModel, input,
		func(target assemblyline.TargetTree) error {
			if err := validateDirectCodingFocusedTargetTree(stack, target); err != nil {
				return err
			}
			return validateDirectCodingTargetTreePathClosure(input, existingFiles, target)
		},
	)
}

type directCodingTargetTreeCandidateValidator func(assemblyline.TargetTree) error

func runDirectCodingTargetTreeCallWithValidator(
	runtime typedWorkerRuntime,
	initialModel string,
	replacementModel string,
	input assemblyline.TargetTreeInput,
	validateCandidate directCodingTargetTreeCandidateValidator,
) (assemblyline.TargetTree, error) {
	var zero assemblyline.TargetTree
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("target tree requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxDirectCodingTargetTreeCalls {
		return zero, fmt.Errorf("target tree calls must be between 1 and %d", maxDirectCodingTargetTreeCalls)
	}
	initialModel = strings.TrimSpace(initialModel)
	replacementModel = strings.TrimSpace(replacementModel)
	if initialModel == "" || replacementModel == "" {
		return zero, fmt.Errorf("target tree requires configured initial and replacement models")
	}
	if validateCandidate == nil {
		return zero, fmt.Errorf("target tree requires one code-owned candidate validator")
	}
	var lastSafeCandidate string
	var lastCorrectionFailure string
	var lastFailure error
	seen := make(map[string]struct{})
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return zero, fmt.Errorf("target tree authority ended: %w", err)
		}
		attemptInput := input
		modelName := initialModel
		if lastFailure != nil {
			attemptInput.Correction = &assemblyline.TargetTreeCorrection{
				CandidateTree: lastSafeCandidate,
				Failure:       lastCorrectionFailure,
			}
			modelName = replacementModel
		}
		job, err := assemblyline.NewTargetTreeJob(attemptInput)
		if err != nil {
			return zero, err
		}
		result, err := runtime.Execute(job, modelName)
		if err != nil {
			return zero, fmt.Errorf("target tree inference: %w", err)
		}
		if err := result.ValidateFor(job); err != nil {
			return zero, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		candidate := result.Candidate
		if _, duplicate := seen[candidate]; duplicate {
			if lastFailure == nil {
				return zero, fmt.Errorf("target tree repeated a candidate without a retained validation defect")
			}
			if err := persistTargetTreeRejection(runtime, job, result, lastFailure); err != nil {
				return zero, err
			}
			return zero, fmt.Errorf("target tree remains invalid after an exact zero delta: %w", lastFailure)
		}
		seen[candidate] = struct{}{}
		target, validationErr := assemblyline.DecodeTargetTreeCandidate(input, candidate)
		if validationErr == nil {
			validationErr = validateCandidate(target)
		}
		if validationErr == nil {
			if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
				return zero, err
			}
			return target, nil
		}
		if err := persistTargetTreeRejection(runtime, job, result, validationErr); err != nil {
			return zero, err
		}
		lastSafeCandidate = ""
		lastCorrectionFailure = "The response violates the exact raw target-tree grammar."
		if parsed, syntaxErr := assemblyline.ParseTargetTree(candidate); syntaxErr == nil {
			canonical, renderErr := assemblyline.RenderTargetTree(parsed.Paths)
			if renderErr != nil {
				return zero, renderErr
			}
			lastSafeCandidate = canonical
			lastCorrectionFailure, err = directCodingTargetTreeCorrectionFailure(validationErr)
			if err != nil {
				return zero, err
			}
		}
		lastFailure = validationErr
	}
	return zero, fmt.Errorf("target tree candidate failed after %d bounded calls: %w", runtime.MaxAttempts, lastFailure)
}

func directCodingTargetTreeCorrectionFailure(validationErr error) (string, error) {
	if !errors.Is(validationErr, errDirectCodingTargetTreeExistingFileConflict) {
		return assemblyline.TargetTreeCorrectionFailure(validationErr)
	}
	failure := "One response node crosses a basename hierarchy already held by an existing workspace file."
	if err := assemblyline.ValidatePathFreeModelContext(
		"target tree correction failure", failure,
	); err != nil {
		return "", err
	}
	return failure, nil
}

func persistTargetTreeRejection(
	runtime typedWorkerRuntime,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if runtime.Finalize == nil {
		return nil
	}
	if err := runtime.Finalize(job, result, validationErr); err != nil {
		return fmt.Errorf("persist target tree rejection: %w", err)
	}
	return nil
}
