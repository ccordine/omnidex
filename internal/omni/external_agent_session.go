package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type ExternalAgentJob struct {
	SessionID string                     `json:"session_id,omitempty"`
	Agent     string                     `json:"agent"`
	Mode      string                     `json:"mode"`
	Packet    CursorImplementationPacket `json:"packet"`
	Prompt    string                     `json:"prompt"`
	Workspace string                     `json:"workspace"`
}

type HumanCorrection struct {
	Message               string   `json:"message"`
	Authority             string   `json:"authority,omitempty"`
	ForbiddenDependencies []string `json:"forbidden_dependencies,omitempty"`
	AllowedFiles          []string `json:"allowed_files,omitempty"`
}

type AgentEventType string

const (
	AgentEventStarted     AgentEventType = "started"
	AgentEventStatus      AgentEventType = "status"
	AgentEventMessage     AgentEventType = "message"
	AgentEventThinking    AgentEventType = "thinking"
	AgentEventCommand     AgentEventType = "command"
	AgentEventTool        AgentEventType = "tool"
	AgentEventFileChange  AgentEventType = "file_change"
	AgentEventCompleted   AgentEventType = "completed"
	AgentEventError       AgentEventType = "error"
	AgentEventInterrupted AgentEventType = "interrupted"
)

func (eventType AgentEventType) Valid() bool {
	switch eventType {
	case AgentEventStarted,
		AgentEventStatus,
		AgentEventMessage,
		AgentEventThinking,
		AgentEventCommand,
		AgentEventTool,
		AgentEventFileChange,
		AgentEventCompleted,
		AgentEventError,
		AgentEventInterrupted:
		return true
	default:
		return false
	}
}

type AgentEvent struct {
	SessionID string          `json:"session_id,omitempty"`
	Agent     string          `json:"agent"`
	Type      AgentEventType  `json:"type"`
	Message   string          `json:"message,omitempty"`
	Command   string          `json:"command,omitempty"`
	Files     []string        `json:"files,omitempty"`
	Evidence  []string        `json:"evidence,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

type ExternalAgentSession interface {
	Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error)
	Interrupt(ctx context.Context, correction HumanCorrection) error
	Cancel(ctx context.Context, reason string) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Cleanup(ctx context.Context) error
}

func resultFromExternalAgentEvents(events <-chan AgentEvent) CursorArchitectAgentResult {
	result := CursorArchitectAgentResult{}
	output := []string{}
	for event := range events {
		if (event.Type == AgentEventCompleted || event.Type == AgentEventInterrupted) && result.Summary == "" && !strings.Contains(event.Message, "session ended") {
			if event.Message != "" {
				result.Summary = event.Message
			}
		}
		if event.SessionID != "" {
			result.RunID = event.SessionID
		}
		if event.Agent != "" {
			result.AgentID = event.Agent
		}
		if event.Message != "" {
			output = append(output, event.Message)
		} else if len(event.Raw) > 0 {
			output = append(output, string(event.Raw))
		}
	}
	result.Output = strings.Join(output, "\n")
	if result.Summary == "" {
		result.Summary = result.Output
	}
	return result
}

func structuredEventFromExternalAgentEvent(event AgentEvent) StructuredCommandEvent {
	details := map[string]string{
		"session_id": event.SessionID,
		"agent":      event.Agent,
		"type":       string(event.Type),
	}
	if event.Command != "" {
		details["command"] = truncateStructuredTimelineValue(event.Command)
	}
	if len(event.Files) > 0 {
		details["files"] = strings.Join(event.Files, ",")
	}
	return StructuredCommandEvent{
		Type:    "external_agent_" + strings.ReplaceAll(strings.TrimSpace(string(event.Type)), ".", "_"),
		Summary: firstNonEmpty(event.Message, "External agent event"),
		Details: details,
	}
}

func applyHumanCorrectionToExternalAgentInput(input CursorArchitectAgentInput, correction HumanCorrection) CursorArchitectAgentInput {
	message := strings.TrimSpace(correction.Message)
	if strings.TrimSpace(correction.Authority) == "" {
		correction.Authority = "user"
	}
	if message != "" {
		input.Packet.PreparedContext = appendUniqueStrings(input.Packet.PreparedContext, fmt.Sprintf("human_correction[%s]: %s", strings.TrimSpace(correction.Authority), message))
	}
	for _, dep := range correction.ForbiddenDependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		input.Packet.Forbidden = appendUniqueStrings(input.Packet.Forbidden, "do not add dependency: "+dep)
	}
	if len(correction.AllowedFiles) > 0 {
		allowed := make([]string, 0, len(correction.AllowedFiles))
		for _, path := range correction.AllowedFiles {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				allowed = append(allowed, filepath.ToSlash(trimmed))
			}
		}
		if len(allowed) > 0 {
			input.Packet.EditSurface = appendUniqueStrings(nil, allowed...)
		}
	}
	return input
}

func restartExternalAgentSessionWithCorrection(ctx context.Context, active ExternalAgentSession, provider ExternalAgentSessionProvider, agentName string, input CursorArchitectAgentInput, correction HumanCorrection) (<-chan AgentEvent, CursorArchitectAgentInput, error) {
	if active != nil {
		if err := errors.Join(
			active.Cancel(ctx, "human correction invalidated active external-agent plan"),
			active.Cleanup(ctx),
		); err != nil {
			return nil, input, fmt.Errorf("stop external agent before applying human correction: %w", err)
		}
	}
	revised := applyHumanCorrectionToExternalAgentInput(input, correction)
	session, err := provider.NewExternalAgentSession(revised)
	if err != nil {
		return nil, revised, err
	}
	job := ExternalAgentJob{
		SessionID: agentName + "-architect-corrected",
		Agent:     strings.TrimSuffix(agentName, "_sdk"),
		Mode:      "implementation",
		Packet:    revised.Packet,
		Prompt:    externalAgentPromptForName(agentName, revised),
		Workspace: revised.Workspace,
	}
	events, err := session.Start(ctx, job)
	if err != nil {
		return nil, revised, err
	}
	return events, revised, nil
}
