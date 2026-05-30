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
)

func JobMetadata(projectID int64, agentSystem, modelName string) ([]byte, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	payload := map[string]any{
		"source":              JobSource,
		MetadataProjectID:     projectID,
		MetadataAgentSystem:   strings.TrimSpace(agentSystem),
		MetadataModel:         strings.TrimSpace(modelName),
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

func ParseMetadata(raw json.RawMessage) (projectID int64, agentSystem, modelName string, err error) {
	if len(raw) == 0 {
		return 0, "", "", fmt.Errorf("job metadata is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, "", "", fmt.Errorf("parse job metadata: %w", err)
	}
	if strings.TrimSpace(stringFromAny(payload["source"])) != JobSource {
		return 0, "", "", fmt.Errorf("not a project debugger job")
	}
	switch v := payload[MetadataProjectID].(type) {
	case float64:
		projectID = int64(v)
	case int64:
		projectID = v
	case string:
		projectID, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	}
	if projectID <= 0 {
		return 0, "", "", fmt.Errorf("project_id is required")
	}
	agentSystem = strings.TrimSpace(stringFromAny(payload[MetadataAgentSystem]))
	modelName = strings.TrimSpace(stringFromAny(payload[MetadataModel]))
	return projectID, agentSystem, modelName, nil
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
