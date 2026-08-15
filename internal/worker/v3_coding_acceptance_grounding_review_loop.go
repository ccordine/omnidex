package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runProgressiveDirectCodingAcceptanceGroundingReview(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	original assemblyline.PortableJob,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
) (assemblyline.ApplicationAcceptanceGroundingReview, error) {
	var zero assemblyline.ApplicationAcceptanceGroundingReview
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("acceptance grounding review requires a portable execution runtime")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, fmt.Errorf("acceptance grounding review requires one configured model")
	}
	if err := original.Validate(); err != nil {
		return zero, err
	}

	seenStates := make(map[string]struct{})
	retained := ""
	correctionField := ""
	var correctionKind assemblyline.ApplicationAcceptanceGroundingLeafValidationKind
	for attempt := 1; ; attempt++ {
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
				original, directCodingGroundingReviewCorrectionFailure(
					retained, correctionField, correctionKind,
				),
				correctionField,
			)
			if err != nil {
				return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
			}
		}
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
			return zero, failGroundingReviewResult(runtime, modelName, subject, attempt, job, result, err)
		}
		candidate := strings.TrimSpace(result.Candidate)
		if job.Kind == assemblyline.WorkResponseCorrection {
			candidate, err = assemblyline.ApplyResponseCorrectionForField(
				original, retained, candidate, correctionField,
			)
			if err != nil {
				return zero, failGroundingReviewResult(runtime, modelName, subject, attempt, job, result, err)
			}
		}
		candidate, err = canonicalGroundingReviewCandidate(candidate)
		if err != nil {
			return zero, failGroundingReviewResult(runtime, modelName, subject, attempt, job, result, err)
		}
		if err = rememberGroundingReviewState(seenStates, candidate); err != nil {
			return zero, failGroundingReviewResult(runtime, modelName, subject, attempt, job, result, err)
		}

		review, validationErr := assemblyline.DecodeApplicationAcceptanceGroundingReview(input, candidate)
		if validationErr == nil {
			if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
				return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
			}
			emitTypedWorker(runtime, typedWorkerEvent{
				State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
				Model: modelName, Attempt: attempt,
			})
			return review, nil
		}
		if err := persistGroundingReviewRejection(runtime, job, result, validationErr); err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		emitDirectCodingSemanticRejection(runtime, modelName, subject, attempt, validationErr)
		var leafErr *assemblyline.ApplicationAcceptanceGroundingLeafValidationError
		if !errors.As(validationErr, &leafErr) {
			return zero, failDirectCodingSemanticCall(
				runtime, modelName, subject, attempt,
				fmt.Errorf("acceptance grounding review has no correctable leaf authority: %w", validationErr),
			)
		}
		retained = candidate
		correctionField = leafErr.Field()
		correctionKind = leafErr.Kind()
	}
}

func rememberGroundingReviewState(seen map[string]struct{}, candidate string) error {
	if _, repeated := seen[candidate]; repeated {
		return fmt.Errorf("acceptance grounding review repeated prior retained state")
	}
	seen[candidate] = struct{}{}
	return nil
}

func canonicalGroundingReviewCandidate(raw string) (string, error) {
	object, err := decodeDirectCodingSemanticJSON[map[string]any](raw)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("encode retained acceptance grounding review: %w", err)
	}
	return string(canonical), nil
}

func directCodingGroundingReviewCorrectionFailure(
	retained string,
	field string,
	kind assemblyline.ApplicationAcceptanceGroundingLeafValidationKind,
) string {
	sum := sha256.Sum256([]byte(retained))
	return fmt.Sprintf(
		"RETAINED_STATE_SHA256=%s; INVALID_LEAF=%s; INVALID_KIND=%s",
		hex.EncodeToString(sum[:]), field, kind,
	)
}

func persistGroundingReviewRejection(
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

func failGroundingReviewResult(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	if err := persistGroundingReviewRejection(runtime, job, result, validationErr); err != nil {
		validationErr = err
	}
	emitDirectCodingSemanticRejection(runtime, modelName, subject, attempt, validationErr)
	return failDirectCodingSemanticCall(runtime, modelName, subject, attempt, validationErr)
}
