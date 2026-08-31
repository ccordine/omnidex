package api

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

const (
	maxRealtimeRuntimeEventBytes  = 128
	maxRealtimeRuntimeDetailBytes = 1800
	maxRealtimeFilePathBytes      = 4096
)

// JobRuntimeEvent is a worker observation projected through the existing
// server-owned realtime hub. It cannot change job or filesystem state.
type JobRuntimeEvent struct {
	JobID          int64
	ChannelID      model.ChannelID
	StepID         int64
	Attempt        int64
	RuntimeEvent   string
	Detail         string
	FileOperation  string
	FilePath       string
	FileSourcePath string
}

func (event JobRuntimeEvent) validate() error {
	if event.JobID <= 0 || event.StepID <= 0 || event.Attempt <= 0 {
		return fmt.Errorf("job runtime event requires positive job, step, and attempt identities")
	}
	if event.ChannelID != "" {
		if err := event.ChannelID.Validate(); err != nil {
			return fmt.Errorf("job runtime event channel: %w", err)
		}
	}
	if !canonicalRealtimeRuntimeEvent(event.RuntimeEvent) {
		return fmt.Errorf("job runtime event name %q is not canonical", event.RuntimeEvent)
	}
	if len(event.Detail) > maxRealtimeRuntimeDetailBytes || !utf8.ValidString(event.Detail) ||
		strings.ContainsRune(event.Detail, '\x00') {
		return fmt.Errorf("job runtime event detail exceeds its exact UTF-8 boundary")
	}
	if event.FileOperation == "" && event.FilePath == "" && event.FileSourcePath == "" {
		if event.RuntimeEvent == "workspace_file_changed" {
			return fmt.Errorf("workspace file runtime event requires an operation and path")
		}
		return nil
	}
	if event.RuntimeEvent != "workspace_file_changed" || event.FileOperation == "" || event.FilePath == "" {
		return fmt.Errorf("job runtime file operation and path require workspace_file_changed authority")
	}
	switch event.FileOperation {
	case "create", "replace", "delete":
		if event.FileSourcePath != "" {
			return fmt.Errorf("non-move workspace event cannot carry a source path")
		}
	case "move":
		if event.FileSourcePath == "" {
			return fmt.Errorf("workspace move event requires a source path")
		}
	default:
		return fmt.Errorf("job runtime file operation %q is not registered", event.FileOperation)
	}
	if event.FilePath != strings.TrimSpace(event.FilePath) ||
		len(event.FilePath) > maxRealtimeFilePathBytes || !utf8.ValidString(event.FilePath) ||
		strings.ContainsRune(event.FilePath, '\x00') {
		return fmt.Errorf("job runtime file path is outside its exact UTF-8 boundary")
	}
	if event.FileSourcePath != "" && (event.FileSourcePath != strings.TrimSpace(event.FileSourcePath) ||
		len(event.FileSourcePath) > maxRealtimeFilePathBytes || !utf8.ValidString(event.FileSourcePath) ||
		strings.ContainsRune(event.FileSourcePath, '\x00')) {
		return fmt.Errorf("job runtime source path is outside its exact UTF-8 boundary")
	}
	return nil
}

func canonicalRealtimeRuntimeEvent(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxRealtimeRuntimeEventBytes {
		return false
	}
	for _, character := range []byte(value) {
		if character < 'a' || character > 'z' {
			if character < '0' || character > '9' {
				if character != '_' {
					return false
				}
			}
		}
	}
	return true
}

// PublishJobRuntimeEvent exposes committed/observed worker activity without
// making the transport part of workflow control flow.
func (s *Server) PublishJobRuntimeEvent(event JobRuntimeEvent) error {
	if err := event.validate(); err != nil {
		return err
	}
	summary := strings.TrimSpace(event.Detail)
	if summary == "" {
		summary = event.RuntimeEvent
	}
	topics := []string{realtimeTopicUI, realtimeTopicJobs}
	if event.ChannelID != "" {
		channelTopic, topicErr := realtimeChannelTopic(event.ChannelID)
		if topicErr != nil {
			return topicErr
		}
		topics = append(topics, channelTopic)
	}
	_, err := s.broadcastRealtimeChecked(
		topics,
		realtimeMessage{
			EventName: "job-runtime-event", JobID: event.JobID,
			ChannelID: string(event.ChannelID),
			Phase:     realtimeJobChanged, Summary: summary,
			StepID: event.StepID, Attempt: event.Attempt,
			RuntimeEvent: event.RuntimeEvent, Detail: event.Detail,
			FileOperation: event.FileOperation, FilePath: event.FilePath,
			FileSourcePath: event.FileSourcePath,
		},
	)
	return err
}
