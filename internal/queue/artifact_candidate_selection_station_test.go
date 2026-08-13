package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestArtifactCandidateSelectionHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewArtifactCandidateSelectionJob(
		assemblyline.ArtifactCandidateSelectionInput{
			RequirementQuote: "Remove either ARTIFACT_1 or ARTIFACT_2.",
			Candidates: []assemblyline.ArtifactCandidateEvidence{
				{CandidateID: "ARTIFACT_CANDIDATE_1", Declarations: []string{"function First: func First() int"}},
				{CandidateID: "ARTIFACT_CANDIDATE_2", Declarations: []string{"function Second: func Second() int"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingArtifactCandidateSelection {
		t.Fatalf("station=%q want=%q", got, station.CodingArtifactCandidateSelection)
	}
	correction, err := assemblyline.NewResponseCorrectionJob(job, "candidate is unavailable")
	if err != nil {
		t.Fatal(err)
	}
	got, err = StationForPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingArtifactCandidateSelection {
		t.Fatalf("correction station=%q want=%q", got, station.CodingArtifactCandidateSelection)
	}
}
