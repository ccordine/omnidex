package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

type semanticCandidateExhaustedError struct {
	Subject  string
	Attempts int
	Err      error
}

func (err *semanticCandidateExhaustedError) Error() string {
	return fmt.Sprintf("%s candidate failed %d bounded corrections: %v", err.Subject, err.Attempts, err.Err)
}

func (err *semanticCandidateExhaustedError) Unwrap() error {
	return err.Err
}

func runDirectCodingSemanticCall[T any](
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	job assemblyline.PortableJob,
	identities []assemblyline.ArtifactIdentity,
	validate func(T) error,
) (T, error) {
	var zero T
	if runtime.Context == nil || runtime.Execute == nil {
		return zero, fmt.Errorf("coding semantic worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts < 1 || runtime.MaxAttempts > maxTypedWorkerAttempts {
		return zero, fmt.Errorf("coding semantic worker attempts must be between 1 and %d", maxTypedWorkerAttempts)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, fmt.Errorf("coding semantic worker requires one configured model")
	}
	basePrompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, err
	}
	if err := validateDirectCodingSemanticPrompt(basePrompt, identities); err != nil {
		return zero, err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: runtime.MaxAttempts,
		PromptBytes: len(basePrompt),
	})

	var lastErr error
	var lastCandidate string
	retainedCandidate := false
	lastCandidateRejected := false
	attemptsUsed := 0
	seenCandidates := make(map[string]struct{})
	seenJobs := make(map[string]struct{})
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		attemptsUsed = attempt
		if err := runtime.Context.Err(); err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, fmt.Errorf("authority ended: %w", err))
		}
		attemptJob := job
		if lastErr != nil {
			if !retainedCandidate {
				break
			}
			attemptJob, err = assemblyline.NewResponseCorrectionJob(
				job, trimForBudget(lastErr.Error(), 1200),
			)
			if err != nil {
				return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
			}
		}
		prompt, _, err := assemblyline.RenderPortableJob(attemptJob)
		if err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
		}
		if err := validateDirectCodingSemanticPrompt(prompt, identities); err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1, err)
		}
		if _, duplicate := seenJobs[attemptJob.ID]; duplicate {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt-1,
				fmt.Errorf("repeated identical semantic gap rejected before inference"))
		}
		seenJobs[attemptJob.ID] = struct{}{}

		result, err := runtime.Execute(attemptJob, modelName)
		if err != nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		if err = result.ValidateFor(attemptJob); err != nil {
			err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
		}
		candidateRejected := true
		candidate := strings.TrimSpace(result.Candidate)
		if attemptJob.Kind == assemblyline.WorkResponseCorrection {
			candidate, err = assemblyline.ApplyResponseCorrection(job, lastCandidate, candidate)
		}
		if err == nil {
			if _, duplicate := seenCandidates[candidate]; duplicate {
				err = fmt.Errorf("repeated identical candidate rejected; the correction made no progress")
			} else {
				seenCandidates[candidate] = struct{}{}
			}
		}
		if err == nil {
			var value T
			value, err = decodeDirectCodingSemanticJSON[T](candidate)
			if err == nil {
				lastCandidate = candidate
				retainedCandidate = true
				err = validate(value)
			}
			if err == nil {
				if err = finalizeTypedWorkerResult(runtime, attemptJob, result, nil); err != nil {
					return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, err)
				}
				emitTypedWorker(runtime, typedWorkerEvent{
					State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
					Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
				})
				return value, nil
			}
		}
		if contextErr := runtime.Context.Err(); contextErr != nil {
			contextErr = finalizeTypedWorkerResult(runtime, attemptJob, result, contextErr)
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, fmt.Errorf("authority ended: %w", contextErr))
		}
		err = finalizeTypedWorkerResult(runtime, attemptJob, result, err)
		if err == nil {
			return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attempt, fmt.Errorf("semantic candidate rejection lost its exact failure"))
		}
		lastErr = err
		lastCandidateRejected = candidateRejected
		emitDirectCodingSemanticRejection(runtime, modelName, subject, attempt, err)
		if strings.Contains(err.Error(), "repeated identical candidate") {
			break
		}
	}
	boundedErr := fmt.Errorf("failed after %d bounded attempts: %w", attemptsUsed, lastErr)
	if lastCandidateRejected {
		boundedErr = &semanticCandidateExhaustedError{
			Subject: subject, Attempts: attemptsUsed, Err: lastErr,
		}
	}
	return zero, failDirectCodingSemanticCall(runtime, modelName, subject, attemptsUsed, boundedErr)
}

func decodeDirectCodingSemanticJSON[T any](raw string) (T, error) {
	var value T
	content := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if content == "" {
		return value, fmt.Errorf("coding semantic response is empty")
	}
	if err := exactjson.ValidateObject(
		[]byte(content), value, "coding semantic response",
	); err != nil {
		return value, fmt.Errorf("decode coding semantic response: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode coding semantic response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("decode coding semantic response: trailing JSON value")
		}
		return value, fmt.Errorf("decode coding semantic response trailing data: %w", err)
	}
	return value, nil
}

func validateDirectCodingSemanticPrompt(prompt string, identities []assemblyline.ArtifactIdentity) error {
	for _, identity := range identities {
		if value := strings.TrimSpace(identity.Value); value != "" && strings.Contains(prompt, value) {
			return fmt.Errorf("coding semantic prompt exposes source identity behind %s", identity.Token)
		}
	}
	return nil
}

func emitDirectCodingSemanticRejection(runtime typedWorkerRuntime, modelName, subject string, attempt int, err error) {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerRejected, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
}

func failDirectCodingSemanticCall(runtime typedWorkerRuntime, modelName, subject string, attempt int, err error) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("coding semantic %s failed: %w", subject, err)
}
