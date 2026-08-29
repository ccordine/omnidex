package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceEndpointLeavesHaveDistinctExactStationOwners(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceEndpointTaskAuthority{
		Surface:        assemblyline.ApplicationSurfaceService,
		ProductContext: "inventory service", RequirementQuote: "Clients can retrieve inventory records.",
	}
	tests := []struct {
		want station.ID
		job  assemblyline.PortableJob
	}{
		{station.CodingServiceEndpointExposure, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointExposureJob(
			assemblyline.ApplicationServiceEndpointExposureInput{Authority: authority},
		))},
		{station.CodingServiceEndpointMethod, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointMethodJob(
			assemblyline.ApplicationServiceEndpointMethodInput{Authority: authority},
		))},
		{station.CodingServiceEndpointRouteTemplate, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointRouteTemplateJob(
			assemblyline.ApplicationServiceEndpointRouteTemplateInput{Authority: authority},
		))},
		{station.CodingServiceEndpointRequestMedia, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointRequestMediaJob(
			assemblyline.ApplicationServiceEndpointRequestMediaInput{Authority: authority, Method: assemblyline.ApplicationServiceEndpointGET},
		))},
		{station.CodingServiceEndpointResponseMedia, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointResponseMediaJob(
			assemblyline.ApplicationServiceEndpointResponseMediaInput{
				Authority: authority, Method: assemblyline.ApplicationServiceEndpointGET,
			},
		))},
		{station.CodingServiceEndpointSuccessStatus, mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointSuccessStatusJob(
			assemblyline.ApplicationServiceEndpointSuccessStatusInput{
				Authority: authority, Method: assemblyline.ApplicationServiceEndpointGET,
				RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
				ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
			},
		))},
	}
	for _, test := range tests {
		got, err := StationForPortableJob(test.job)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("endpoint leaf %s station=%q want=%q", test.job.Kind, got, test.want)
		}
	}
}

func mustEndpointOwnerJob(job assemblyline.PortableJob, err error) assemblyline.PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
