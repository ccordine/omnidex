package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceDeploymentSemanticsHaveSeparateExactStationOwners(t *testing.T) {
	t.Parallel()
	request := "Build a public status display and keep it running in this environment."
	availability, err := assemblyline.NewApplicationServiceContinuedAvailabilityJob(
		assemblyline.ApplicationServiceContinuedAvailabilityInput{UserRequest: request},
	)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := assemblyline.NewApplicationServicePersistenceDestinationJob(
		assemblyline.ApplicationServicePersistenceDestinationInput{
			UserRequest: request,
			ContinuedAvailability: assemblyline.ApplicationServiceContinuedAvailabilityResult{
				Schema:      assemblyline.ApplicationServiceContinuedAvailabilitySchemaV1,
				CandidateID: assemblyline.ApplicationServiceAvailabilityRequiredCandidate,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name      string
		job       assemblyline.PortableJob
		station   station.ID
		candidate string
	}{
		{
			name: "continued availability", job: availability,
			station:   station.CodingServiceContinuedAvailability,
			candidate: "AVAILABILITY_CANDIDATE_1",
		},
		{
			name: "persistence destination", job: destination,
			station:   station.CodingServicePersistenceDestination,
			candidate: "DESTINATION_CANDIDATE_1",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertApplicationServiceDeploymentSemanticOwner(t, testCase.job, testCase.station)
			correction, err := assemblyline.NewRetainedResponseCorrectionJob(
				testCase.job, "candidate is unavailable", testCase.candidate,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertApplicationServiceDeploymentSemanticOwner(t, correction, testCase.station)
		})
	}
}

func assertApplicationServiceDeploymentSemanticOwner(
	t *testing.T, job assemblyline.PortableJob, want station.ID,
) {
	t.Helper()
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("station=%q want=%q", got, want)
	}
}
