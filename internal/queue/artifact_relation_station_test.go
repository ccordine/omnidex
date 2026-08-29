package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestArtifactRelationsHaveSeparateExactStationOwners(t *testing.T) {
	t.Parallel()
	absence, err := assemblyline.NewRepositoryArtifactAbsenceJob(
		assemblyline.RepositoryArtifactAbsenceInput{
			RequirementQuote: "One known semantic artifact must no longer exist.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	creation, err := assemblyline.NewPlainTextArtifactCreationJob(
		assemblyline.PlainTextArtifactCreationInput{
			RequirementQuote: "Create ARTIFACT_1 containing the complete note: Release ready.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		job  assemblyline.PortableJob
		want station.ID
	}{
		{job: absence, want: station.CodingRepositoryArtifactAbsence},
		{job: creation, want: station.CodingPlainTextArtifactCreation},
	} {
		got, err := StationForPortableJob(fixture.job)
		if err != nil {
			t.Fatal(err)
		}
		if got != fixture.want {
			t.Fatalf("work kind %q station=%q want=%q", fixture.job.Kind, got, fixture.want)
		}
	}
}
