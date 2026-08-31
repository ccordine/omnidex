package worker

import (
	"strings"
	"time"
	"unicode/utf8"

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
	if s == nil {
		return
	}
	eventType = strings.TrimSpace(eventType)
	message = boundedRuntimeEventDetail(message)
	s.emitRuntimeEvent(RuntimeEvent{
		JobID: authority.JobID, StepID: authority.StepID, Attempt: authority.Attempt,
		Kind: eventType, Detail: message,
	})
	payload := strings.TrimSpace(strings.Join([]string{
		"time=" + time.Now().UTC().Format(time.RFC3339),
		"event=" + eventType,
		message,
	}, " "))
	if payload == "" || s.logger == nil {
		return
	}
	s.logf(
		"job=%d step=%d attempt=%d %s",
		authority.JobID, authority.StepID, authority.Attempt, trimForBudget(payload, 1800),
	)
}

func (s *Service) emitWorkspaceFileChange(
	authority model.StepAttemptAuthority,
	operation, sourcePath, path string,
) {
	if s == nil {
		return
	}
	detail := operation + " " + path
	if sourcePath != "" {
		detail = operation + " " + sourcePath + " -> " + path
	}
	s.emitRuntimeEvent(RuntimeEvent{
		JobID: authority.JobID, StepID: authority.StepID, Attempt: authority.Attempt,
		Kind: "workspace_file_changed", Detail: boundedRuntimeEventDetail(detail),
		FileOperation: operation, FilePath: path, FileSourcePath: sourcePath,
	})
}

func boundedRuntimeEventDetail(value string) string {
	const maxBytes = 1800
	const suffix = "\n...[truncated]"
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes - len(suffix)
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + suffix
}

func (s *Service) emitRuntimeEvent(event RuntimeEvent) {
	if s == nil || s.runtimeEventSink == nil {
		return
	}
	event.ChannelID = s.runtimeEventChannel(event.JobID)
	if err := s.runtimeEventSink(event); err != nil {
		s.logf(
			"job=%d step=%d attempt=%d runtime event=%q publication failed: %v",
			event.JobID, event.StepID, event.Attempt, event.Kind, err,
		)
	}
}
