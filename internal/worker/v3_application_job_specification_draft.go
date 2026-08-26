package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runProgressiveApplicationJobSpecificationDraft(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	original assemblyline.PortableJob,
	authority assemblyline.ApplicationJobSpecificationInput,
) (assemblyline.ApplicationJobSpecification, error) {
	var zero assemblyline.ApplicationJobSpecification
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("application job specification draft requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return zero, fmt.Errorf(
			"application job specification draft attempts must be between 1 and %d",
			maxTypedWorkerAttempts,
		)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, fmt.Errorf("application job specification draft requires one configured model")
	}
	if err := original.Validate(); err != nil {
		return zero, err
	}
	if err := authority.ValidatePathFree(runtime.PathProvenance); err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 0, err)
	}

	seenStates := make(map[string]struct{})
	seenJobs := make(map[string]struct{})
	retained := ""
	correctionTarget := ""
	validationFailure := ""
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		if err := runtime.Context.Err(); err != nil {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt-1,
				fmt.Errorf("authority ended: %w", err),
			)
		}
		job := original
		if retained != "" {
			var err error
			job, err = assemblyline.NewResponseCorrectionJobForField(
				original, validationFailure, correctionTarget,
			)
			if err != nil {
				return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
			}
		}
		if _, repeated := seenJobs[job.ID]; repeated {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt-1,
				fmt.Errorf("application job specification repeated an identical correction gap"),
			)
		}
		seenJobs[job.ID] = struct{}{}
		prompt, _, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
			Model: modelName, Attempt: attempt, PromptBytes: len(prompt),
		})

		result, err := runtime.Execute(job, modelName)
		if err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		if err = result.ValidateFor(job); err != nil {
			return zero, failApplicationJobSpecificationDraftResult(
				runtime, modelName, subject, attempt, job, result, err,
			)
		}
		candidate := strings.TrimSpace(result.Candidate)
		if job.Kind == assemblyline.WorkResponseCorrection {
			candidate, err = assemblyline.ApplyResponseCorrectionForField(
				original, retained, candidate, correctionTarget,
			)
			if err != nil {
				return zero, failApplicationJobSpecificationDraftResult(
					runtime, modelName, subject, attempt, job, result, err,
				)
			}
		}
		specification, err := assemblyline.DecodeApplicationJobSpecification(authority, candidate)
		if err != nil {
			return zero, failApplicationJobSpecificationDraftResult(
				runtime, modelName, subject, attempt, job, result, err,
			)
		}
		if err = specification.ValidatePathFree(runtime.PathProvenance); err != nil {
			return zero, failApplicationJobSpecificationDraftResult(
				runtime, modelName, subject, attempt, job, result, err,
			)
		}
		canonical, err := json.Marshal(specification)
		if err != nil {
			return zero, failApplicationJobSpecificationDraftResult(
				runtime, modelName, subject, attempt, job, result, err,
			)
		}
		candidate = string(canonical)
		if _, repeated := seenStates[candidate]; repeated {
			err = fmt.Errorf("application job specification repeated a prior retained state")
			return zero, failApplicationJobSpecificationDraftResult(
				runtime, modelName, subject, attempt, job, result, err,
			)
		}
		seenStates[candidate] = struct{}{}

		defect := assemblyline.FirstApplicationJobSpecificationDefect(specification)
		if defect == nil {
			if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
				return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
			}
			emitTypedWorker(runtime, typedWorkerEvent{
				State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
				Model: modelName, Attempt: attempt,
			})
			return specification, nil
		}
		if err := persistApplicationJobSpecificationDraftRejection(runtime, job, result, defect); err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		emitDirectCodingSemanticRejection(runtime, modelName, subject, attempt, defect)
		target, correctable := defect.CorrectionTarget()
		if !correctable {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt,
				fmt.Errorf("application job specification has no single-leaf correction authority: %w", defect),
			)
		}
		retained = candidate
		correctionTarget = target
		validationFailure = defect.Error()
	}
	return zero, failDirectCodingSemanticCall(
		runtime, modelName, subject, runtime.MaxAttempts,
		fmt.Errorf(
			"application job specification failed after %d bounded attempts: %s",
			runtime.MaxAttempts, validationFailure,
		),
	)
}

func persistApplicationJobSpecificationDraftRejection(
	runtime typedWorkerRuntime,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if runtime.Finalize == nil {
		return nil
	}
	if err := runtime.Finalize(job, result, validationErr); err != nil {
		return fmt.Errorf("%v; persist station rejection: %w", validationErr, err)
	}
	return nil
}

func failApplicationJobSpecificationDraftResult(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if err := persistApplicationJobSpecificationDraftRejection(runtime, job, result, validationErr); err != nil {
		validationErr = err
	}
	emitDirectCodingSemanticRejection(runtime, modelName, subject, attempt, validationErr)
	return failDirectCodingSemanticCall(runtime, modelName, subject, attempt, validationErr)
}
