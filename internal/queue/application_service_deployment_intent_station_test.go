package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceDeploymentIntentHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationServiceDeploymentIntentJob(
		assemblyline.ApplicationServiceDeploymentIntentInput{
			UserRequest: "Build a public status display and keep it running in this environment.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertApplicationServiceDeploymentIntentOwner(t, job)
	correction, err := assemblyline.NewRetainedResponseCorrectionJob(
		job,
		"candidate is unavailable",
		`{"schema":"omnidex.application-service-deployment-intent.v1","candidate_id":"DEPLOYMENT_CANDIDATE_1"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertApplicationServiceDeploymentIntentOwner(t, correction)
}

func assertApplicationServiceDeploymentIntentOwner(t *testing.T, job assemblyline.PortableJob) {
	t.Helper()
	got, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if got != station.CodingServiceDeploymentIntent {
		t.Fatalf("station=%q want=%q", got, station.CodingServiceDeploymentIntent)
	}
}
