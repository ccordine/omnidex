package worker

import (
	"context"
	"strings"
	"time"
)

const stepEventWriteTimeout = 5 * time.Second

func (s *Service) emitStepStream(stepID int64, stream, message string) {
	key := "tool_stdout"
	if strings.EqualFold(strings.TrimSpace(stream), "stderr") {
		key = "tool_stderr"
	}
	s.emitStepContext(stepID, key, message)
}

func (s *Service) emitStepContext(stepID int64, key, value string) {
	s.emitStepContextWithBudget(stepID, key, value, 1800)
}

func (s *Service) emitStepContextWithBudget(stepID int64, key, value string, maxChars int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if maxChars <= 0 {
		maxChars = 1800
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepEventWriteTimeout)
	defer cancel()
	if err := s.repo.AddStepContext(ctx, stepID, key, trimForBudget(value, maxChars)); err != nil {
		s.logger.Printf("step=%d context key=%s write error: %v", stepID, key, err)
	}
}

func (s *Service) emitStepEvent(stepID int64, eventType, message string) {
	payload := strings.TrimSpace(strings.Join([]string{
		"time=" + time.Now().UTC().Format(time.RFC3339),
		"event=" + strings.TrimSpace(eventType),
		strings.TrimSpace(message),
	}, " "))
	if eventType != "step_complete" && eventType != "step_canceled" {
		s.emitStepContext(stepID, "event", payload)
	}
	if s.repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.repo.RecordTelemetryStepEvent(ctx, stepID, eventType, message); err != nil {
		s.logger.Printf("step=%d telemetry event=%s write error: %v", stepID, eventType, err)
	}
}
