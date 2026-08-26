package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestReplayArtifactValidatesServiceEndpointRequirement(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewApplicationServiceEndpointRequirementJob(
		assemblyline.ApplicationServiceEndpointRequirementInput{
			ProductContext:     "inventory service",
			RequirementQuote:   "Records are normalized before storage.",
			Objective:          "Normalize accepted record values.",
			RequiredBehaviors:  []string{"Normalize each accepted record value."},
			AcceptanceCriteria: []string{"Equivalent values have one normalized representation."},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"schema":"omnidex.application-service-endpoint-requirement.v1","endpoint_requirement":"support_only"}`
	artifact, err := replayExactStationArtifact(job, valid)
	if err != nil || artifact.Kind != "application_service_endpoint_requirement" {
		t.Fatalf("endpoint requirement artifact=%+v error=%v", artifact, err)
	}
	invalid := `{"schema":"omnidex.application-service-endpoint-requirement.v1","endpoint_requirement":"route_later"}`
	if _, err := replayExactStationArtifact(job, invalid); err == nil {
		t.Fatal("replay accepted unregistered endpoint requirement")
	}
}
