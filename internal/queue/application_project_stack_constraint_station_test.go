package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationProjectStackConstraintHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationProjectStackConstraintJob(
		assemblyline.ApplicationProjectStackConstraintInput{
			ProductContext:       "browser inventory console",
			AcceptedRequirements: []string{"Show current inventory"},
			Candidates: []assemblyline.ApplicationProjectStackCandidate{{
				CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "TypeScript with React for a browser application",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertProjectStackConstraintOwner(t, job)
}

func assertProjectStackConstraintOwner(t *testing.T, job assemblyline.PortableJob) {
	t.Helper()
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingProjectStackConstraint {
		t.Fatalf("station=%q want=%q", got, station.CodingProjectStackConstraint)
	}
}
