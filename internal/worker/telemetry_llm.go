package worker

import (
	"context"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const workerLLMContextCharsPerToken = 4

func (s *Service) recordWorkerLLMCall(ctx context.Context, stepID int64, scope, modelName string, promptChars, attempt int, success bool, callErr error, latency time.Duration) {
	if s.repo == nil || stepID <= 0 {
		return
	}
	runID, jobID, _ := s.repo.TelemetryJobContextForStep(ctx, stepID)
	errorClass := ""
	if callErr != nil {
		errorClass = classifyWorkerLLMError(callErr)
	}
	limit := s.inferenceContextTokens * workerLLMContextCharsPerToken
	_ = s.repo.RecordLLMContextUsage(ctx, queue.LLMContextUsageRecord{
		Source:            "worker:" + strings.TrimSpace(scope),
		Model:             strings.TrimSpace(modelName),
		Provider:          "ollama",
		RunID:             runID,
		JobID:             jobID,
		StepID:            stepID,
		Scope:             strings.TrimSpace(scope),
		Attempt:           attempt,
		PromptChars:       promptChars,
		SentChars:         promptChars,
		ContextLimitChars: limit,
		Success:           success,
		ErrorClass:        errorClass,
		LatencyMS:         latency.Milliseconds(),
		Metadata: map[string]any{
			"worker_context_budget": s.contextBudget,
			"context_tokens":        s.inferenceContextTokens,
		},
	})
	if runID == "" {
		return
	}
	finished := time.Now().UTC()
	started := finished.Add(-latency)
	latencyMS := latency.Milliseconds()
	successVal := success
	_ = s.repo.RecordTelemetryModelCall(ctx, queue.TelemetryModelCallRecord{
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
			"step_id":        stepID,
			"prompt_chars":   promptChars,
			"error_class":    errorClass,
			"attempt":        attempt,
			"context_tokens": s.inferenceContextTokens,
		},
	})
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
