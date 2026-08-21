package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestDeclarationArtifactBoundaryHasOneExactStationOwner(t *testing.T) {
	t.Parallel()

	input := assemblyline.DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Normalize(input string) string has an independent artifact boundary",
		GoSignature:      "func Normalize(input string) string",
		DeclarationID:    "DECLARATION_1",
	}
	job, err := assemblyline.NewDeclarationArtifactBoundaryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingDeclarationArtifactBoundary {
		t.Fatalf("station=%q want=%q", got, station.CodingDeclarationArtifactBoundary)
	}
	correction, err := assemblyline.NewRetainedResponseCorrectionJob(
		job, "boundary is unsupported",
		`{"schema":"omnidex.declaration-artifact-boundary.v1","declaration_id":"DECLARATION_1","boundary":"none"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err = StationForPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingDeclarationArtifactBoundary {
		t.Fatalf("correction station=%q want=%q", got, station.CodingDeclarationArtifactBoundary)
	}
}
