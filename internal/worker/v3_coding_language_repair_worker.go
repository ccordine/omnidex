package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	maxDirectCodingLanguageRepairTransitions = 2
	maxDirectCodingLanguageExecutorAttempts  = 2
)

var (
	errDirectCodingLanguageCorrectionUnchanged = errors.New("repair executor returned byte-identical source")
	errDirectCodingLanguageCorrectionInvalid   = errors.New("repair executor returned invalid source")
	errDirectCodingLanguageGuidanceRepeated    = errors.New("repair guidance repeated a rejected instruction")
)

type directCodingLanguageFragmentRejection struct {
	Candidate string
	Failure   error
}

func (rejection *directCodingLanguageFragmentRejection) Error() string {
	return rejection.Failure.Error()
}

func (rejection *directCodingLanguageFragmentRejection) Unwrap() error {
	return rejection.Failure
}

func runDirectCodingLanguageRepairGuidance(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	input assemblyline.FragmentRepairGuidanceInput,
) (string, error) {
	job, err := assemblyline.NewFragmentRepairGuidanceJob(input)
	if err != nil {
		return "", fmt.Errorf("construct fragment repair-guidance job: %w", err)
	}
	guidance, err := runDirectCodingSemanticCall[assemblyline.FragmentRepairGuidance](
		runtime, modelName, subject+":repair_guidance", job, nil,
		func(candidate assemblyline.FragmentRepairGuidance) error {
			if err := candidate.Validate(); err != nil {
				return err
			}
			return candidate.ValidatePathFree(runtime.PathProvenance)
		},
	)
	if err != nil {
		return "", err
	}
	return guidance.Instruction, nil
}

func runDirectCodingLanguageCorrection(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	current string,
	instruction string,
	sourceProjection string,
	validate func(string) (string, error),
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil || validate == nil {
		return "", fmt.Errorf("language correction requires execution, projection, and parser boundaries")
	}
	job, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		CurrentDeclaration: strings.TrimSpace(current),
		RepairGuidance:     strings.TrimSpace(instruction),
	}, sourceProjection)
	if err != nil {
		return "", err
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return "", err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1, PromptBytes: len(prompt),
		CurrentBytes: len(current), CorrectionBytes: len(instruction),
	})
	result, err := runtime.Execute(job, strings.TrimSpace(modelName))
	if err != nil {
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	if err := result.ValidateFor(job); err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	projection, err := projectDirectCodingSourceDeclaration(sourceProjection, result.Candidate)
	if err != nil {
		err = fmt.Errorf("%w: %v", errDirectCodingLanguageCorrectionInvalid, err)
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	result.Projection = &projection
	candidate := projection.Source
	if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"language correction candidate", runtime.PathProvenance, candidate,
	); err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	if candidate == strings.TrimSpace(current) {
		err = finalizeTypedWorkerResult(
			runtime, job, result, errDirectCodingLanguageCorrectionUnchanged,
		)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	validated, err := validate(candidate)
	if err != nil {
		err = fmt.Errorf("%w: %v", errDirectCodingLanguageCorrectionInvalid, err)
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	if currentValidated, currentErr := validate(current); currentErr == nil &&
		validated == currentValidated {
		err = finalizeTypedWorkerResult(
			runtime, job, result, errDirectCodingLanguageCorrectionUnchanged,
		)
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
		return "", failDirectCodingLanguageCorrection(runtime, modelName, subject, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return candidate, nil
}

func failDirectCodingLanguageCorrection(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("language fragment correction failed: %w", err)
}
