package queue

import (
	"encoding/json"
	"fmt"
)

var removedProjectPlanningSettingKeys = []string{
	"planning_chat",
	"planning_chat_config",
	"planning_draft_queue",
}

func validateProjectSettings(settings json.RawMessage) error {
	if len(settings) == 0 {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return fmt.Errorf("project settings must be a JSON object: %w", err)
	}
	if payload == nil {
		return fmt.Errorf("project settings must be a JSON object, received null")
	}
	for _, key := range removedProjectPlanningSettingKeys {
		if _, exists := payload[key]; exists {
			return fmt.Errorf("project setting %q was removed; use the PostgreSQL project planning API", key)
		}
	}
	return nil
}
