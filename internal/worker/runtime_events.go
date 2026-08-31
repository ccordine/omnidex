package worker

import (
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Service) logf(format string, values ...any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Printf(format, values...)
}

func (s *Service) emitStepStream(authority model.StepAttemptAuthority, stream, message string) {
	message = strings.TrimSpace(message)
	if message == "" || s == nil || s.logger == nil {
		return
	}
	stream = strings.ToLower(strings.TrimSpace(stream))
	if stream != "stderr" {
		stream = "stdout"
	}
	s.logf(
		"job=%d step=%d attempt=%d %s: %s",
		authority.JobID, authority.StepID, authority.Attempt, stream,
		trimForBudget(message, 1800),
	)
}

func (s *Service) emitStepEvent(authority model.StepAttemptAuthority, eventType, message string) {
	payload := strings.TrimSpace(strings.Join([]string{
		"time=" + time.Now().UTC().Format(time.RFC3339),
		"event=" + strings.TrimSpace(eventType),
		strings.TrimSpace(message),
	}, " "))
	if payload == "" || s == nil || s.logger == nil {
		return
	}
	s.logf(
		"job=%d step=%d attempt=%d %s",
		authority.JobID, authority.StepID, authority.Attempt, trimForBudget(payload, 1800),
	)
}
