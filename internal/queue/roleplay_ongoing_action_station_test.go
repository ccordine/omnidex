package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRoleplayOngoingActionHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	input := assemblyline.RoleplayOngoingActionInput{
		CharacterName:     "Mara",
		Source:            assemblyline.RoleplayOngoingActionSourceAssistantResponse,
		ExactContribution: "Mara continues hauling the line toward shore.",
	}
	job, err := assemblyline.NewRoleplayOngoingActionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if owner != station.RoleplayOngoingAction {
		t.Fatalf("station=%q want %q", owner, station.RoleplayOngoingAction)
	}
}
