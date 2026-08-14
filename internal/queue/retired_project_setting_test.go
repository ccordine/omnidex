package queue

import (
	"encoding/json"
	"testing"
)

func TestProjectSettingsRejectRetiredAgentAuthority(t *testing.T) {
	t.Parallel()
	if err := validateProjectSettings(json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`)); err == nil {
		t.Fatal("project settings accepted retired agent authority")
	}
}
