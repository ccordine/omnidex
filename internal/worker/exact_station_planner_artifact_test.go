package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestReplayArtifactUsesProductionSemanticLeafDecoders(t *testing.T) {
	t.Parallel()

	request := "Build a schedule board with filters."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	productContext, err := assemblyline.NewApplicationProductContextJob(
		assemblyline.ApplicationProductContextInput{UserRequest: request, Context: applicationContext},
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact, err := replayExactStationArtifact(productContext, "A schedule board"); err != nil ||
		artifact.Kind != string(assemblyline.WorkApplicationProductContext) {
		t.Fatalf("product-context artifact=%+v error=%v", artifact, err)
	}
	if _, err := replayExactStationArtifact(
		productContext,
		`{"product_context":"A schedule board"}`,
	); err == nil {
		t.Fatal("replay accepted a structured wrapper around the product-context leaf")
	}
}
