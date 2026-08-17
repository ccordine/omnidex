package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	intentModel string,
	reviewModel string,
	authority assemblyline.ApplicationIntentInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	job, err := assemblyline.NewApplicationIntentJob(authority)
	if err != nil {
		return zero, err
	}
	retained, err := runDirectCodingSemanticCall[assemblyline.ApplicationIntentCandidate](
		runtime, intentModel, "application_intent", job, identities,
		func(value assemblyline.ApplicationIntentCandidate) error { return value.Validate() },
	)
	if err != nil {
		return zero, err
	}
	seen := make(map[string]struct{})
	if err := observeApplicationIntentCandidate(seen, retained); err != nil {
		return zero, err
	}
	reviewIndex := 0
	for round := 1; ; round++ {
		targets := assemblyline.ApplicationIntentReviewTargets(retained)
		if reviewIndex >= len(targets) {
			return assemblyline.ResolveApplicationIntent(authority, retained)
		}
		reviewInput := assemblyline.ApplicationIntentReviewInput{
			Authority: authority, Candidate: retained, Target: targets[reviewIndex],
		}
		reviewJob, jobErr := assemblyline.NewApplicationIntentReviewJob(reviewInput)
		if jobErr != nil {
			return zero, jobErr
		}
		review, callErr := runApplicationFrontDoorCall(
			runtime, reviewModel, fmt.Sprintf("application_intent_review_%d", round),
			reviewJob, identities,
			func(raw string) (assemblyline.ApplicationIntentReview, error) {
				return assemblyline.DecodeApplicationIntentReview(reviewInput, raw)
			},
		)
		if callErr != nil {
			return zero, callErr
		}
		if review.Decision == assemblyline.ApplicationIntentReviewAccept {
			reviewIndex++
			continue
		}
		if review.Decision == assemblyline.ApplicationIntentReviewRemove {
			retained, callErr = assemblyline.ApplyApplicationIntentReviewRemoval(authority, retained, review)
		} else {
			retained, callErr = assemblyline.ApplyApplicationIntentReviewReplacement(authority, retained, review)
		}
		if callErr != nil {
			return zero, callErr
		}
		if err := observeApplicationIntentCandidate(seen, retained); err != nil {
			return zero, err
		}
		reviewIndex = 0
	}
}

func runApplicationFrontDoorCall[T any](
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	job assemblyline.PortableJob,
	identities []assemblyline.ArtifactIdentity,
	decode func(string) (T, error),
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("application front door requires a portable execution runtime")
	}
	if err := runtime.Context.Err(); err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 0, err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, failDirectCodingSemanticCall(runtime, modelName, subject, 0, err)
	}
	if err := validateDirectCodingSemanticPrompt(prompt, identities); err != nil {
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
			err = fmt.Errorf("application front door call requires one decoder")
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

func observeApplicationIntentCandidate(
	seen map[string]struct{},
	candidate assemblyline.ApplicationIntentCandidate,
) error {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("encode application intent progress: %w", err)
	}
	identity := string(raw)
	if _, repeated := seen[identity]; repeated {
		return fmt.Errorf("application intent review and repair entered a repeated-state cycle")
	}
	seen[identity] = struct{}{}
	return nil
}
