package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestLoadRoleplayOngoingActionStationModelFromExactEnvironmentKey(t *testing.T) {
	t.Setenv("OMNI_ROLEPLAY_ONGOING_ACTION_MODEL", "action-model")
	models := loadStationModels(Config{})
	if got := models[station.RoleplayOngoingAction]; got != "action-model" {
		t.Fatalf("roleplay ongoing-action model=%q", got)
	}
}
