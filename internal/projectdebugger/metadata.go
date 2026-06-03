package projectdebugger

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	JobSource           = "project_debugger"
	MetadataProjectID   = "project_id"
	MetadataAgentSystem = "agent_system"
	MetadataModel       = "model"
	MetadataTicketModel = "ticket_model"
)

type ParsedMetadata struct {
	ProjectID     int64
	AgentSystem   string
	AnalyzerModel string
	TicketModel   string
}

func JobMetadata(projectID int64, agentSystem, analyzerModel, ticketModel string) ([]byte, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	payload := map[string]any{
		"source":              JobSource,
		MetadataProjectID:     projectID,
		MetadataAgentSystem:   strings.TrimSpace(agentSystem),
		MetadataModel:         strings.TrimSpace(analyzerModel),
		MetadataTicketModel:   strings.TrimSpace(ticketModel),
	}
	return json.Marshal(payload)
}

func IsJobMetadata(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromAny(payload["source"])) == JobSource
}

func ParseMetadata(raw json.RawMessage) (ParsedMetadata, error) {
	if len(raw) == 0 {
		return ParsedMetadata{}, fmt.Errorf("job metadata is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedMetadata{}, fmt.Errorf("parse job metadata: %w", err)
	}
	if strings.TrimSpace(stringFromAny(payload["source"])) != JobSource {
		return ParsedMetadata{}, fmt.Errorf("not a project debugger job")
	}
	out := ParsedMetadata{
		AgentSystem:   strings.TrimSpace(stringFromAny(payload[MetadataAgentSystem])),
		AnalyzerModel: strings.TrimSpace(stringFromAny(payload[MetadataModel])),
		TicketModel:   strings.TrimSpace(stringFromAny(payload[MetadataTicketModel])),
	}
	switch v := payload[MetadataProjectID].(type) {
	case float64:
		out.ProjectID = int64(v)
	case int64:
		out.ProjectID = v
	case string:
		out.ProjectID, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	}
	if out.ProjectID <= 0 {
		return ParsedMetadata{}, fmt.Errorf("project_id is required")
	}
	return out, nil
}

func Pipeline() string {
	return model.PipelineProjectDebugger
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
