package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestReplayArtifactUsesProductionPlannerDecoders(t *testing.T) {
	t.Parallel()

	request := "Build a schedule board with filters."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := assemblyline.NewApplicationIntentJob(
		assemblyline.ApplicationIntentInput{UserRequest: request, Context: applicationContext},
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact, err := replayExactStationArtifact(intent, `{
		"schema":"omnidex.application-intent.v1",
		"product_context":"A schedule board",
		"requirements":["Allow users to filter the schedule."]
	}`); err != nil || artifact.Kind != "application_intent" {
		t.Fatalf("intent artifact=%+v error=%v", artifact, err)
	}
	if _, err := replayExactStationArtifact(intent, `{
		"schema":"omnidex.application-intent.v1",
		"product_context":"A schedule board",
		"requirements":[]
	}`); err == nil {
		t.Fatal("replay accepted structurally invalid semantic intent")
	}
}
