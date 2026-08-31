package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/modelconfig"
)

var removedProjectSettingKeys = []string{
	"planning_chat",
	"planning_chat_config",
	"planning_draft_queue",
	"scrum_auto_play_through",
	"scrum_auto_review",
	"agent_config",
}

func validateProjectSettings(settings json.RawMessage) error {
	if len(settings) == 0 {
		return fmt.Errorf("project settings must be a JSON object")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return fmt.Errorf("project settings must be a JSON object: %w", err)
	}
	if payload == nil {
		return fmt.Errorf("project settings must be a JSON object, received null")
	}
	for _, key := range removedProjectSettingKeys {
		if _, exists := payload[key]; exists {
			return fmt.Errorf("project setting %q was removed and has no compatibility path", key)
		}
	}
	if _, err := modelconfig.FromSettingsJSON(settings); err != nil {
		return fmt.Errorf("project settings model routing: %w", err)
	}
	return nil
}
