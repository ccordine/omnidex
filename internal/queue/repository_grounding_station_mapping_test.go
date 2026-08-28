package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryGroundingPortableWorkHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	for kind, want := range map[assemblyline.WorkKind]station.ID{
		assemblyline.WorkRepositoryEvidenceRelevanceLeaf: station.RepositoryEvidenceRelevance,
		assemblyline.WorkRepositoryGroundedIssueDetail:   station.RepositoryGroundedReview,
		assemblyline.WorkRepositoryGroundedIssueKind:     station.RepositoryGroundedReview,
		assemblyline.WorkRepositoryGroundedCorrection:    station.RepositoryGroundedCorrection,
	} {
		if got, err := stationForPortableWorkKind(kind); err != nil || got != want {
			t.Fatalf("work=%q station=%q error=%v want=%q", kind, got, err, want)
		}
	}
}
