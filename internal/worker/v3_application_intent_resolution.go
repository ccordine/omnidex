package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingApplicationContext(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	context assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationContext, error) {
	input := assemblyline.ApplicationContextNeedInput{
		UserRequest: authority, Context: context,
	}
	job, err := assemblyline.NewApplicationContextNeedJob(input)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	decision, err := runDirectCodingSemanticCall[assemblyline.ApplicationContextNeedDecision](
		runtime, modelName, "application_context_needs", job, identities,
		func(value assemblyline.ApplicationContextNeedDecision) error { return value.Validate() },
	)
	if err != nil {
		return assemblyline.ApplicationContext{}, err
	}
	if len(decision.Questions) > 0 {
		return assemblyline.ApplicationContext{}, fmt.Errorf(
			"application context has %d unresolved evidence needs: %s",
			len(decision.Questions), strings.Join(decision.Questions, " | "),
		)
	}
	return context, nil
}

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
	for round := 1; ; round++ {
		reviewInput := assemblyline.ApplicationIntentReviewInput{
			Authority: authority, Candidate: retained,
		}
		reviewJob, jobErr := assemblyline.NewApplicationIntentReviewJob(reviewInput)
		if jobErr != nil {
			return zero, jobErr
		}
		review, callErr := runApplicationFrontDoorCall(
			runtime, reviewModel, fmt.Sprintf("application_intent_review_%d", round),
			reviewJob, identities,
			func(raw string) (assemblyline.ApplicationIntentReviewDecision, error) {
				return assemblyline.DecodeApplicationIntentReviewDecision(reviewInput, raw)
			},
		)
		if callErr != nil {
			return zero, callErr
		}
		if review.Outcome == assemblyline.ApplicationIntentReviewAccept {
			return assemblyline.ResolveApplicationIntent(authority, retained)
		}
		repairInput := assemblyline.ApplicationIntentRepairInput{
			Authority: authority, Finding: review,
		}
		repairJob, jobErr := assemblyline.NewApplicationIntentRepairJob(repairInput)
		if jobErr != nil {
			return zero, jobErr
		}
		retainedBeforeRepair := retained
		retained, callErr = runApplicationFrontDoorCall(
			runtime, intentModel, fmt.Sprintf("application_intent_repair_%d", round),
			repairJob, identities,
			func(raw string) (assemblyline.ApplicationIntentCandidate, error) {
				repair, decodeErr := assemblyline.DecodeApplicationIntentRepairDecision(repairInput, raw)
				if decodeErr != nil {
					return assemblyline.ApplicationIntentCandidate{}, decodeErr
				}
				return assemblyline.ApplyApplicationIntentRepair(
					authority, retainedBeforeRepair, review, repair,
				)
			},
		)
		if callErr != nil {
			return zero, callErr
		}
		if err := observeApplicationIntentCandidate(seen, retained); err != nil {
			return zero, err
		}
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
