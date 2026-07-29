package omni

import (
	"context"
	"encoding/json"
	"strings"
)

type ExternalCodingRequest struct {
	Instruction string
	Context     string
	Workspace   string
}

type ExternalCodingResult struct {
	Summary string `json:"summary"`
	AgentID string `json:"agent_id"`
	RunID   string `json:"run_id"`
	Output  string `json:"output"`
}

type ExternalCodingAgent interface {
	RunCodingTask(ctx context.Context, request ExternalCodingRequest) (ExternalCodingResult, error)
}

type ExternalAgentJob struct {
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Prompt    string `json:"prompt"`
	Workspace string `json:"workspace"`
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
	case AgentEventStarted, AgentEventStatus, AgentEventMessage, AgentEventThinking,
		AgentEventCommand, AgentEventTool, AgentEventFileChange, AgentEventCompleted,
		AgentEventError, AgentEventInterrupted:
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
	Cancel(ctx context.Context, reason string) error
	Cleanup(ctx context.Context) error
}

func resultFromExternalAgentEvents(events <-chan AgentEvent) ExternalCodingResult {
	result := ExternalCodingResult{}
	output := make([]string, 0, 32)
	for event := range events {
		if event.Type == AgentEventCompleted && result.Summary == "" {
			result.Summary = strings.TrimSpace(event.Message)
		}
		if event.SessionID != "" {
			result.RunID = event.SessionID
		}
		if event.Agent != "" {
			result.AgentID = event.Agent
		}
		if message := strings.TrimSpace(event.Message); message != "" {
			output = append(output, message)
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
