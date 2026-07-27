package projectdebugger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const JobSource = "project_debugger"

type ParsedMetadata struct {
	ProjectID     int64
	AgentSystem   string
	AnalyzerModel string
	TicketModel   string
}

type metadataPayload struct {
	Source        string `json:"source"`
	ProjectID     int64  `json:"project_id"`
	AgentSystem   string `json:"agent_system"`
	AnalyzerModel string `json:"model"`
	TicketModel   string `json:"ticket_model"`
}

func JobMetadata(projectID int64, agentSystem, analyzerModel, ticketModel string) ([]byte, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	agentSystem = strings.TrimSpace(agentSystem)
	analyzerModel = strings.TrimSpace(analyzerModel)
	ticketModel = strings.TrimSpace(ticketModel)
	if agentSystem == "" || analyzerModel == "" || ticketModel == "" {
		return nil, fmt.Errorf("agent_system, model, and ticket_model are required")
	}
	payload := metadataPayload{Source: JobSource, ProjectID: projectID, AgentSystem: agentSystem, AnalyzerModel: analyzerModel, TicketModel: ticketModel}
	return json.Marshal(payload)
}

func IsJobMetadata(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	metadata, err := ParseMetadata(raw)
	return err == nil && metadata.ProjectID > 0
}

func ParseMetadata(raw json.RawMessage) (ParsedMetadata, error) {
	if len(raw) == 0 {
		return ParsedMetadata{}, fmt.Errorf("job metadata is empty")
	}
	var payload metadataPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ParsedMetadata{}, fmt.Errorf("parse job metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ParsedMetadata{}, fmt.Errorf("project debugger metadata contains trailing JSON")
		}
		return ParsedMetadata{}, fmt.Errorf("project debugger metadata contains trailing data: %w", err)
	}
	if strings.TrimSpace(payload.Source) != JobSource {
		return ParsedMetadata{}, fmt.Errorf("not a project debugger job")
	}
	out := ParsedMetadata{
		ProjectID:     payload.ProjectID,
		AgentSystem:   strings.TrimSpace(payload.AgentSystem),
		AnalyzerModel: strings.TrimSpace(payload.AnalyzerModel),
		TicketModel:   strings.TrimSpace(payload.TicketModel),
	}
	if out.ProjectID <= 0 {
		return ParsedMetadata{}, fmt.Errorf("project_id is required")
	}
	if out.AgentSystem == "" || out.AnalyzerModel == "" || out.TicketModel == "" {
		return ParsedMetadata{}, fmt.Errorf("agent_system, model, and ticket_model are required")
	}
	return out, nil
}

func Pipeline() string {
	return model.PipelineProjectDebugger
}
