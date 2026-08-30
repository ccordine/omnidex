package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestGroundedAnswerParagraphSieveWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkGroundedAnswerParagraphInventory,
		assemblyline.WorkGroundedAnswerParagraphEvidenceRelation,
		assemblyline.WorkGroundedAnswerParagraphAuthorization,
	} {
		got, err := stationForPortableWorkKind(kind)
		if err != nil || got != station.GroundedAnswer {
			t.Fatalf("work=%q station=%q error=%v", kind, got, err)
		}
	}
}
