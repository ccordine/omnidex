package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestReplayArtifactUsesProductionPlannerDecoders(t *testing.T) {
	t.Parallel()

	requirements, err := assemblyline.NewApplicationRequirementInterpretationJob(
		assemblyline.ApplicationRequirementInterpretationInput{
			UserRequest: "Build a schedule board with filters.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact, err := replayExactStationArtifact(requirements, `{
		"schema":"omnidex.application-requirements.v1",
		"items":[
			{"kind":"product","source_quote":"schedule board"},
			{"kind":"feature","source_quote":"filters"}
		]
	}`); err != nil || artifact.Kind != "application_requirements" {
		t.Fatalf("requirements artifact=%+v error=%v", artifact, err)
	}
	if _, err := replayExactStationArtifact(requirements, `{
		"schema":"omnidex.application-requirements.v1",
		"items":[
			{"kind":"product","source_quote":"schedule board"},
			{"kind":"feature","source_quote":"invented"}
		]
	}`); err == nil {
		t.Fatal("replay accepted ungrounded planner output")
	}
}
