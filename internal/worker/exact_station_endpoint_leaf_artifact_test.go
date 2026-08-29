package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestReplayArtifactsValidateEachServiceEndpointLeaf(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceEndpointTaskAuthority{
		Surface:        assemblyline.ApplicationSurfaceService,
		ProductContext: "inventory service", RequirementQuote: "Create one inventory record.",
	}
	fixtures := []struct {
		job assemblyline.PortableJob
		raw string
	}{
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointExposureJob(
			assemblyline.ApplicationServiceEndpointExposureInput{Authority: authority},
		)), "public"},
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointMethodJob(
			assemblyline.ApplicationServiceEndpointMethodInput{Authority: authority},
		)), "POST"},
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointRouteTemplateJob(
			assemblyline.ApplicationServiceEndpointRouteTemplateInput{Authority: authority},
		)), "/records"},
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointRequestMediaJob(
			assemblyline.ApplicationServiceEndpointRequestMediaInput{
				Authority: authority, Method: assemblyline.ApplicationServiceEndpointPOST,
			},
		)), "application/json"},
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointResponseMediaJob(
			assemblyline.ApplicationServiceEndpointResponseMediaInput{
				Authority: authority, Method: assemblyline.ApplicationServiceEndpointPOST,
			},
		)), "application/json"},
		{mustReplayEndpointJob(assemblyline.NewApplicationServiceEndpointSuccessStatusJob(
			assemblyline.ApplicationServiceEndpointSuccessStatusInput{
				Authority: authority, Method: assemblyline.ApplicationServiceEndpointPOST,
				RequestMedia:  assemblyline.ApplicationServiceEndpointJSON,
				ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
			},
		)), "201"},
	}
	for _, fixture := range fixtures {
		artifact, err := replayExactStationArtifact(fixture.job, fixture.raw)
		if err != nil {
			t.Fatalf("replay %s: %v", fixture.job.Kind, err)
		}
		if artifact.Kind != string(fixture.job.Kind) {
			t.Fatalf("replay artifact kind=%q want=%q", artifact.Kind, fixture.job.Kind)
		}
		if _, err := replayExactStationArtifact(fixture.job, `{"aggregate":true}`); err == nil {
			t.Fatalf("replay %s accepted aggregate output", fixture.job.Kind)
		}
	}
}

func mustReplayEndpointJob(
	job assemblyline.PortableJob,
	err error,
) assemblyline.PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
