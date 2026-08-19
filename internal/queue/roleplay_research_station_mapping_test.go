package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRoleplayGroundedResponseUsesTheBoundedResponseStation(t *testing.T) {
	t.Parallel()
	got, err := stationForPortableWorkKind(assemblyline.WorkRoleplayGroundedResponse)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.ConversationResponse {
		t.Fatalf("station=%q want %q", got, station.ConversationResponse)
	}
}
