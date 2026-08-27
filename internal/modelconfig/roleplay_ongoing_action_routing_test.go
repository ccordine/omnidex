package modelconfig

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestApplyRoleplayOngoingActionStationRouting(t *testing.T) {
	applied := Apply(Routing{}, Config{
		"roleplay_ongoing_action_model": "action-model",
	})
	if got := applied.Stations[station.RoleplayOngoingAction]; got != "action-model" {
		t.Fatalf("roleplay ongoing-action model=%q", got)
	}
}
