package worker

import (
	"context"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const stepEventWriteTimeout = 5 * time.Second

func (s *Service) emitStepStream(authority model.StepAttemptAuthority, stream, message string) {
	key := "tool_stdout"
	if strings.EqualFold(strings.TrimSpace(stream), "stderr") {
		key = "tool_stderr"
	}
	s.emitStepContext(authority, key, message)
}

func (s *Service) emitStepContext(authority model.StepAttemptAuthority, key, value string) {
	s.emitStepContextWithBudget(authority, key, value, 1800)
}

func (s *Service) emitStepContextWithBudget(authority model.StepAttemptAuthority, key, value string, maxChars int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if maxChars <= 0 {
		maxChars = 1800
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepEventWriteTimeout)
	defer cancel()
	if err := s.repo.AddStepContext(ctx, authority, key, trimForBudget(value, maxChars)); err != nil {
		s.logger.Printf("step=%d attempt=%d context key=%s write error: %v", authority.StepID, authority.Attempt, key, err)
	}
}

func (s *Service) emitStepEvent(authority model.StepAttemptAuthority, eventType, message string) {
	payload := strings.TrimSpace(strings.Join([]string{
		"time=" + time.Now().UTC().Format(time.RFC3339),
		"event=" + strings.TrimSpace(eventType),
		strings.TrimSpace(message),
	}, " "))
	if eventType != "step_complete" && eventType != "step_canceled" {
		s.emitStepContext(authority, "event", payload)
	}
}
