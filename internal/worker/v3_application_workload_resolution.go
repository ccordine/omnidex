package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingApplicationWorkload(
	runtime typedWorkerRuntime,
	plannerModel string,
	reviewModel string,
	input assemblyline.ApplicationWorkloadDraftInput,
) (assemblyline.FrozenApplicationWorkload, error) {
	var zero assemblyline.FrozenApplicationWorkload
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("application workload resolver requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 {
		return zero, fmt.Errorf("application workload runtime requires a positive attempt identity")
	}
	plannerModel = strings.TrimSpace(plannerModel)
	reviewModel = strings.TrimSpace(reviewModel)
	if plannerModel == "" || reviewModel == "" {
		return zero, fmt.Errorf("application workload resolver requires planner and review models")
	}
	specifications := make([]assemblyline.ApplicationJobSpecification, 0, len(input.Requirements))
	for index, requirement := range input.Requirements {
		authority := assemblyline.ApplicationJobSpecificationInput{
			Surface: input.Surface, ProductQuote: input.ProductQuote,
			AcceptedRequirements: append([]assemblyline.Requirement(nil), input.Requirements...),
			FocusedRequirement:   requirement,
		}
		specification, err := resolveDirectCodingApplicationJobSpecification(
			runtime, plannerModel, reviewModel,
			fmt.Sprintf("application_job_specification_%03d", index+1), authority,
		)
		if err != nil {
			return zero, err
		}
		specifications = append(specifications, specification)
	}
	draft, err := assemblyline.MaterializeApplicationWorkloadDraft(input, specifications)
	if err != nil {
		return zero, err
	}
	return assemblyline.FreezeApplicationWorkload(input, draft)
}

func resolveDirectCodingApplicationJobSpecification(
	runtime typedWorkerRuntime,
	plannerModel string,
	reviewModel string,
	subject string,
	authority assemblyline.ApplicationJobSpecificationInput,
) (assemblyline.ApplicationJobSpecification, error) {
	var zero assemblyline.ApplicationJobSpecification
	job, err := assemblyline.NewApplicationJobSpecificationJob(authority)
	if err != nil {
		return zero, err
	}
	retained, err := runProgressiveApplicationJobSpecificationDraft(
		runtime, plannerModel, subject, job, authority,
	)
	if err != nil {
		return zero, err
	}
	return reviewDirectCodingApplicationJobSpecification(
		runtime, reviewModel, subject, authority, retained,
	)
}

func reviewDirectCodingApplicationJobSpecification(
	runtime typedWorkerRuntime,
	reviewModel string,
	subject string,
	authority assemblyline.ApplicationJobSpecificationInput,
	retained assemblyline.ApplicationJobSpecification,
) (assemblyline.ApplicationJobSpecification, error) {
	return reviewDirectCodingApplicationJobSpecificationAfterFailure(
		runtime, reviewModel, subject, authority, retained, nil,
	)
}

func reviewDirectCodingApplicationJobSpecificationAfterFailure(
	runtime typedWorkerRuntime,
	reviewModel string,
	subject string,
	authority assemblyline.ApplicationJobSpecificationInput,
	retained assemblyline.ApplicationJobSpecification,
	initialValidationFailure *assemblyline.ApplicationJobSpecificationReviewEvidenceError,
) (assemblyline.ApplicationJobSpecification, error) {
	var zero assemblyline.ApplicationJobSpecification
	reviewModel = strings.TrimSpace(reviewModel)
	if reviewModel == "" {
		return zero, fmt.Errorf("application job specification review model is required")
	}
	progress, err := newApplicationJobSpecificationProgress(retained)
	if err != nil {
		return zero, err
	}
	reviewTargetIndex := 0
	if initialValidationFailure != nil {
		targets := applicationJobSpecificationReviewTargets(retained)
		evidenceID, exists := assemblyline.ApplicationJobSpecificationReviewEvidenceID(
			retained, initialValidationFailure.Field, initialValidationFailure.FindingEvidence,
		)
		if !exists {
			return zero, fmt.Errorf(
				"application job specification initial review failure has no current evidence",
			)
		}
		matched := false
		for index, target := range targets {
			if target.field == initialValidationFailure.Field && target.evidenceID == evidenceID {
				reviewTargetIndex = index
				matched = true
				break
			}
		}
		if !matched {
			return zero, fmt.Errorf("application job specification initial review target is unavailable")
		}
	}

	reviewCall := 0
	repairAttempt := 0
	invalidReviewEvidence := make(map[string]struct{})
	var validationFailure *assemblyline.ApplicationJobSpecificationReviewEvidenceError
	if initialValidationFailure != nil {
		copy := *initialValidationFailure
		validationFailure = &copy
		invalidReviewEvidence[copy.Identity()] = struct{}{}
	}
	for {
		targets := applicationJobSpecificationReviewTargets(retained)
		if reviewTargetIndex >= len(targets) {
			return retained, nil
		}
		currentTarget := targets[reviewTargetIndex]
		reviewCall++
		var reviewInput assemblyline.ApplicationJobSpecificationReviewInput
		var inputErr error
		if validationFailure == nil {
			reviewInput, inputErr = assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, retained, currentTarget.field, currentTarget.evidenceID, reviewCall,
			)
		} else {
			reviewInput, inputErr = assemblyline.NewApplicationJobSpecificationReviewRetryInput(
				authority, retained, currentTarget.field, currentTarget.evidenceID,
				reviewCall, *validationFailure,
			)
		}
		if inputErr != nil {
			return zero, inputErr
		}
		reviewJob, jobErr := assemblyline.NewApplicationJobSpecificationReviewJob(reviewInput)
		if jobErr != nil {
			return zero, jobErr
		}
		review, callErr := runApplicationJobSpecificationCall(
			runtime, reviewModel, fmt.Sprintf("%s_review_%d", subject, reviewCall), reviewJob,
			func(raw string) (assemblyline.ApplicationJobSpecificationReview, error) {
				return assemblyline.DecodeApplicationJobSpecificationReview(reviewInput, raw)
			},
		)
		if callErr != nil {
			var evidenceErr *assemblyline.ApplicationJobSpecificationReviewEvidenceError
			if !errors.As(callErr, &evidenceErr) {
				return zero, callErr
			}
			identity := evidenceErr.Identity()
			if _, repeated := invalidReviewEvidence[identity]; repeated {
				return zero, fmt.Errorf(
					"application job specification %s: repeated invalid review evidence rejected for unchanged retained state",
					subject,
				)
			}
			invalidReviewEvidence[identity] = struct{}{}
			copy := *evidenceErr
			validationFailure = &copy
			continue
		}
		validationFailure = nil
		if review.Decision == assemblyline.ApplicationJobSpecificationReviewAccept {
			reviewTargetIndex++
			continue
		}
		repairAttempt++
		repairInput, inputErr := assemblyline.NewApplicationJobSpecificationRepairInput(
			authority, retained, review, repairAttempt,
		)
		if inputErr != nil {
			return zero, inputErr
		}
		repairJob, jobErr := assemblyline.NewApplicationJobSpecificationRepairJob(repairInput)
		if jobErr != nil {
			return zero, jobErr
		}
		retainedBeforeRepair := retained
		retained, callErr = runApplicationJobSpecificationRepair(
			runtime, reviewModel,
			fmt.Sprintf("%s_repair_%d", subject, repairAttempt),
			repairJob, repairInput, retainedBeforeRepair,
		)
		if callErr != nil {
			var noOp *assemblyline.ApplicationJobSpecificationRepairNoOpError
			if !errors.As(callErr, &noOp) {
				return zero, callErr
			}
			failure := noOp.ReviewFailure()
			identity := failure.Identity()
			if _, repeated := invalidReviewEvidence[identity]; repeated {
				return zero, fmt.Errorf(
					"application job specification %s: repeated no-op reviewer verdict rejected for unchanged retained state",
					subject,
				)
			}
			invalidReviewEvidence[identity] = struct{}{}
			validationFailure = &failure
			retained = retainedBeforeRepair
			continue
		}
		if progressErr := progress.Observe(retained); progressErr != nil {
			return zero, fmt.Errorf("application job specification %s: %w", subject, progressErr)
		}
		reviewTargetIndex = 0
	}
}

func runApplicationJobSpecificationRepair(
	runtime typedWorkerRuntime,
	repairModel string,
	subject string,
	repairJob assemblyline.PortableJob,
	repairInput assemblyline.ApplicationJobSpecificationRepairInput,
	retained assemblyline.ApplicationJobSpecification,
) (assemblyline.ApplicationJobSpecification, error) {
	var zero assemblyline.ApplicationJobSpecification
	return runApplicationJobSpecificationCall(
		runtime, repairModel, subject, repairJob,
		func(raw string) (assemblyline.ApplicationJobSpecification, error) {
			patch, decodeErr := assemblyline.DecodeApplicationJobSpecificationRepair(
				repairInput, raw,
			)
			if decodeErr != nil {
				return zero, decodeErr
			}
			return assemblyline.ApplyApplicationJobSpecificationRepair(
				repairInput, retained, patch,
			)
		},
	)
}

func runApplicationJobSpecificationCall[T any](
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, error) {
	var zero T
	if err := runtime.Context.Err(); err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 0, err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 0, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1, PromptBytes: len(prompt),
	})
	result, err := runtime.Execute(job, modelName)
	if err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 1, err)
	}
	if err = result.ValidateFor(job); err == nil {
		if decode == nil {
			err = fmt.Errorf("application job specification call requires one decoder")
		} else {
			var value T
			value, err = decode(result.Candidate)
			if err == nil {
				if finalizeErr := finalizeTypedWorkerResult(runtime, job, result, nil); finalizeErr != nil {
					return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 1, finalizeErr)
				}
				emitTypedWorker(runtime, typedWorkerEvent{
					State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
					Model: modelName, Attempt: 1, MaxAttempts: 1,
				})
				return value, nil
			}
		}
	}
	err = finalizeTypedWorkerResult(runtime, job, result, err)
	emitDirectCodingSemanticRejection(runtime, modelName, subject, 1, err)
	return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 1, err)
}
