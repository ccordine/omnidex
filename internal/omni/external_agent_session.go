package omni

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/agentstream"
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

type ExternalAgentSession interface {
	Start(ctx context.Context, job ExternalAgentJob) (<-chan agentstream.Event, error)
	Cancel(ctx context.Context, reason string) error
	Cleanup(ctx context.Context) error
}

func resultFromExternalAgentEvents(events <-chan agentstream.Event) ExternalCodingResult {
	result := ExternalCodingResult{}
	output := make([]string, 0, 32)
	for event := range events {
		if event.Type == agentstream.EventCompleted && result.Summary == "" {
			result.Summary = event.Message
		}
		if event.SessionID != "" {
			result.RunID = event.SessionID
		}
		if event.Agent != "" {
			result.AgentID = event.Agent
		}
		if event.Message != "" {
			output = append(output, event.Message)
		}
	}
	result.Output = strings.Join(output, "\n")
	return result
}
