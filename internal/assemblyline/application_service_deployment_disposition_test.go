package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceDeploymentDispositionRequiresExactSemanticSequence(t *testing.T) {
	notRequired := ApplicationServiceContinuedAvailabilityResult{
		Schema:      ApplicationServiceContinuedAvailabilitySchemaV1,
		CandidateID: ApplicationServiceAvailabilityNotRequiredCandidate,
	}
	required := ApplicationServiceContinuedAvailabilityResult{
		Schema:      ApplicationServiceContinuedAvailabilitySchemaV1,
		CandidateID: ApplicationServiceAvailabilityRequiredCandidate,
	}
	current := ApplicationServicePersistenceDestinationResult{
		Schema:      ApplicationServicePersistenceDestinationSchemaV1,
		CandidateID: ApplicationServiceBuildEnvironmentDestinationCandidate,
	}
	other := ApplicationServicePersistenceDestinationResult{
		Schema:      ApplicationServicePersistenceDestinationSchemaV1,
		CandidateID: ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
	}
	for _, testCase := range []struct {
		name         string
		availability ApplicationServiceContinuedAvailabilityResult
		destination  *ApplicationServicePersistenceDestinationResult
		want         ApplicationServiceDeploymentDisposition
		wantError    string
	}{
		{name: "verify only", availability: notRequired, want: ApplicationServiceDeploymentVerifyOnly},
		{name: "current host", availability: required, destination: &current, want: ApplicationServiceDeploymentPersistCurrentHost},
		{name: "unresolved target", availability: required, destination: &other, want: ApplicationServiceDeploymentTargetUnresolved},
		{name: "forbidden extra destination", availability: notRequired, destination: &current, wantError: "destination is forbidden"},
		{name: "missing required destination", availability: required, wantError: "requires one persistence destination"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ResolveApplicationServiceDeploymentDisposition(
				testCase.availability, testCase.destination,
			)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error=%v want containing %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != testCase.want {
				t.Fatalf("disposition=%q want=%q", got, testCase.want)
			}
		})
	}
}
