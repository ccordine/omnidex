package client

import (
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"path"
	"strings"
	"unicode/utf8"
)

func (event RealtimeEvent) validate(expectedChannelID model.ChannelID) error {
	if event.ChannelID != expectedChannelID {
		return fmt.Errorf(
			"realtime event channel %q differs from subscribed channel %q",
			event.ChannelID,
			expectedChannelID,
		)
	}
	switch event.EventName {
	case RealtimeConnected:
		if event.AgentActivity != nil || event.AgentCallActivity != nil {
			return fmt.Errorf("connection frame carries an agent projection")
		}
		if event.ID != 0 || event.JobID != 0 || event.ReplayCount < 0 {
			return fmt.Errorf("realtime connection frame carries invalid event authority")
		}
		return nil
	case RealtimeJobProgress:
		if event.AgentActivity != nil || event.AgentCallActivity != nil {
			return fmt.Errorf("progress frame carries an agent projection")
		}
		if event.ID == 0 || event.JobID < 1 || strings.TrimSpace(event.Summary) == "" {
			return fmt.Errorf("realtime job progress frame is incomplete")
		}
		switch event.Phase {
		case "queued", "state_changed", "finished":
		default:
			return fmt.Errorf("realtime job progress has unsupported phase %q", event.Phase)
		}
		return nil
	case RealtimeJobRuntimeEvent:
		if event.ID == 0 || event.JobID < 1 || event.StepID < 1 || event.Attempt < 1 ||
			strings.TrimSpace(event.RuntimeEvent) == "" {
			return fmt.Errorf("realtime job runtime frame is incomplete")
		}
		if event.RuntimeEvent == "agent_call_activity" {
			if event.AgentCallActivity == nil {
				return fmt.Errorf("agent call event lacks its typed journal projection")
			}
			if event.AgentActivity != nil || event.Detail != "" || event.FileOperation != "" || event.FilePath != "" || event.FileSourcePath != "" {
				return fmt.Errorf("agent call event carries unrelated runtime fields")
			}
			return event.AgentCallActivity.Validate()
		}
		if event.AgentCallActivity != nil {
			return fmt.Errorf("non-call runtime event carries an agent call projection")
		}
		if event.RuntimeEvent == "agent_activity" {
			if event.AgentActivity == nil {
				return fmt.Errorf("agent runtime event lacks its typed projection")
			}
			if event.Detail != "" || event.FileOperation != "" || event.FilePath != "" || event.FileSourcePath != "" {
				return fmt.Errorf("agent runtime event carries unrelated fields")
			}
			return event.AgentActivity.Validate()
		}
		if event.AgentActivity != nil {
			return fmt.Errorf("non-agent runtime event carries an agent projection")
		}
		if event.RuntimeEvent == "workspace_file_changed" {
			return event.validateWorkspaceChange()
		}
		if event.RuntimeEvent == "workspace_directory_read" || event.RuntimeEvent == "workspace_file_read" {
			return event.validateWorkspaceRead()
		}
		if event.FileOperation != "" || event.FilePath != "" || event.FileSourcePath != "" {
			return fmt.Errorf("non-file runtime event carries file authority")
		}
		return nil
	default:
		return fmt.Errorf("unsupported jobs realtime event %q", event.EventName)
	}
}

func (event RealtimeEvent) validateWorkspaceRead() error {
	operation := "read"
	rootDirectory := event.RuntimeEvent == "workspace_directory_read" && event.FilePath == "."
	if event.RuntimeEvent == "workspace_directory_read" {
		operation = "list"
	}
	if event.FileOperation != operation || event.FileSourcePath != "" || !rootDirectory && (!canonicalRealtimeRelativePath(event.FilePath) || strings.ContainsAny(event.FilePath, "\\\x00\r\n")) {
		return fmt.Errorf("workspace read event changed its exact relative path or observed operation")
	}
	if len(event.FilePath) > 4096 || !utf8.ValidString(event.FilePath) {
		return fmt.Errorf("workspace read event exceeds its exact path boundary")
	}
	return nil
}

func (event RealtimeEvent) validateWorkspaceChange() error {
	switch event.FileOperation {
	case "create", "replace", "delete":
		if event.FileSourcePath != "" {
			return fmt.Errorf("non-move workspace event carries a source path")
		}
	case "move":
		if !canonicalRealtimeRelativePath(event.FileSourcePath) {
			return fmt.Errorf("workspace move event has invalid source path")
		}
	default:
		return fmt.Errorf("workspace file event has unsupported operation %q", event.FileOperation)
	}
	if !canonicalRealtimeRelativePath(event.FilePath) {
		return fmt.Errorf("workspace file event has invalid destination path")
	}
	return nil
}

func canonicalRealtimeRelativePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value == path.Clean(value) &&
		value != "." && !path.IsAbs(value) && value != ".." && !strings.HasPrefix(value, "../")
}
