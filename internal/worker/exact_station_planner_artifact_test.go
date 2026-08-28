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

func TestReplayArtifactValidatesEachServiceEndpointLeaf(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceEndpointTaskAuthority{
		ProductContext:    "inventory service",
		RequirementQuote:  "Clients can retrieve an inventory record.",
		Objective:         "Expose retrieval of one inventory record.",
		RequiredBehaviors: []string{"Return the requested record."},
	}
	tests := []struct {
		kind string
		job  assemblyline.PortableJob
		raw  string
	}{
		replayLeafFixture(t, "application_service_endpoint_exposure", `{"schema":"omnidex.application-service-endpoint-exposure.v1","exposure":"public"}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointExposureJob(
				assemblyline.ApplicationServiceEndpointExposureInput{Task: authority},
			))),
		replayLeafFixture(t, "application_service_endpoint_method", `{"schema":"omnidex.application-service-endpoint-method.v1","method":"GET"}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointMethodJob(
				assemblyline.ApplicationServiceEndpointMethodInput{Task: authority},
			))),
		replayLeafFixture(t, "application_service_endpoint_route_template", `{"schema":"omnidex.application-service-endpoint-route-template.v1","route_template":"/inventory/{record_id}"}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointRouteTemplateJob(
				assemblyline.ApplicationServiceEndpointRouteTemplateInput{Task: authority},
			))),
		replayLeafFixture(t, "application_service_endpoint_request_media", `{"schema":"omnidex.application-service-endpoint-request-media.v1","request_media":"none"}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointRequestMediaJob(
				assemblyline.ApplicationServiceEndpointRequestMediaInput{
					Task: authority, Method: assemblyline.ApplicationServiceEndpointGET,
				},
			))),
		replayLeafFixture(t, "application_service_endpoint_response_media", `{"schema":"omnidex.application-service-endpoint-response-media.v1","response_media":"application/json"}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointResponseMediaJob(
				assemblyline.ApplicationServiceEndpointResponseMediaInput{Task: authority},
			))),
		replayLeafFixture(t, "application_service_endpoint_success_status", `{"schema":"omnidex.application-service-endpoint-success-status.v1","success_status":200}`,
			mustReplayLeafJob(assemblyline.NewApplicationServiceEndpointSuccessStatusJob(
				assemblyline.ApplicationServiceEndpointSuccessStatusInput{
					Task: authority, Method: assemblyline.ApplicationServiceEndpointGET,
					RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
					ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
				},
			))),
	}
	for _, test := range tests {
		artifact, err := replayExactStationArtifact(test.job, test.raw)
		if err != nil || artifact.Kind != test.kind {
			t.Fatalf("%s artifact=%+v error=%v", test.kind, artifact, err)
		}
	}
}

func replayLeafFixture(
	t *testing.T,
	kind string,
	raw string,
	job assemblyline.PortableJob,
) struct {
	kind string
	job  assemblyline.PortableJob
	raw  string
} {
	t.Helper()
	return struct {
		kind string
		job  assemblyline.PortableJob
		raw  string
	}{kind: kind, job: job, raw: raw}
}

func mustReplayLeafJob(job assemblyline.PortableJob, err error) assemblyline.PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
