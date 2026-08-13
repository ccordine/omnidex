package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestKnownArtifactTruthHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewKnownArtifactTruthJob(assemblyline.KnownArtifactTruthInput{
		RequirementQuote: "One known semantic artifact must no longer exist.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingKnownArtifactTruth {
		t.Fatalf("station=%q want=%q", got, station.CodingKnownArtifactTruth)
	}
	correction, err := assemblyline.NewResponseCorrectionJob(job, "truth is unsupported")
	if err != nil {
		t.Fatal(err)
	}
	got, err = StationForPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingKnownArtifactTruth {
		t.Fatalf("correction station=%q want=%q", got, station.CodingKnownArtifactTruth)
	}
}
