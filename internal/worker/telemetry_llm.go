package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const workerLLMContextCharsPerToken = 4

func (s *Service) recordWorkerLLMCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	scope, modelName string,
	promptChars, attempt int,
	success bool,
	callErr error,
	latency time.Duration,
) error {
	if s.repo == nil || authority.StepID <= 0 {
		return nil
	}
	runID, jobID, err := s.repo.TelemetryJobContextForStep(ctx, authority.StepID)
	if err != nil {
		return fmt.Errorf("load worker LLM telemetry authority: %w", err)
	}
	if jobID != authority.JobID {
		return fmt.Errorf("worker LLM telemetry job %d disagrees with attempt job %d", jobID, authority.JobID)
	}
	errorClass := ""
	if callErr != nil {
		errorClass = classifyWorkerLLMError(callErr)
	}
	limit := s.inferenceContextTokens * workerLLMContextCharsPerToken
	contextErr := s.repo.RecordLLMContextUsageByStepAttempt(ctx, authority, queue.LLMContextUsageRecord{
		Source:            "worker:" + strings.TrimSpace(scope),
		Model:             strings.TrimSpace(modelName),
		Provider:          "ollama",
		RunID:             runID,
		JobID:             jobID,
		StepID:            authority.StepID,
		Scope:             strings.TrimSpace(scope),
		Attempt:           attempt,
		PromptChars:       promptChars,
		SentChars:         promptChars,
		ContextLimitChars: limit,
		Success:           success,
		ErrorClass:        errorClass,
		LatencyMS:         latency.Milliseconds(),
		Metadata: map[string]any{
			"context_tokens": s.inferenceContextTokens,
		},
	})
	if runID == "" {
		return contextErr
	}
	finished := time.Now().UTC()
	started := finished.Add(-latency)
	latencyMS := latency.Milliseconds()
	successVal := success
	modelErr := s.repo.RecordTelemetryModelCallByStepAttempt(ctx, authority, queue.TelemetryModelCallRecord{
		RunID:      runID,
		Role:       strings.TrimSpace(scope),
		Provider:   "ollama",
		Model:      strings.TrimSpace(modelName),
		StartedAt:  &started,
		FinishedAt: &finished,
		LatencyMS:  &latencyMS,
		Success:    &successVal,
		Metadata: map[string]any{
			"job_id":         jobID,
			"step_id":        authority.StepID,
			"prompt_chars":   promptChars,
			"error_class":    errorClass,
			"attempt":        attempt,
			"context_tokens": s.inferenceContextTokens,
		},
	})
	return errors.Join(contextErr, modelErr)
}

func classifyWorkerLLMError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "context") && (strings.Contains(msg, "length") || strings.Contains(msg, "overflow") || strings.Contains(msg, "exceed")):
		return "context_overflow"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "connection") || strings.Contains(msg, "eof") || strings.Contains(msg, "unavailable"):
		return "backend_unavailable"
	default:
		return "llm_error"
	}
}
