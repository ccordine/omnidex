package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func TestApplicationServiceEndpointLeavesHaveDistinctExactStationOwners(t *testing.T) {
	t.Parallel()
	authority := assemblyline.ApplicationServiceEndpointTaskAuthority{
		ProductContext:    "inventory service",
		RequirementQuote:  "Clients can retrieve inventory records.",
		Objective:         "Expose retrieval of inventory records.",
		RequiredBehaviors: []string{"Return the requested record."},
	}
	tests := []struct {
		name string
		want station.ID
		job  assemblyline.PortableJob
	}{
		leafOwnerFixture("exposure", station.CodingServiceEndpointExposure,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointExposureJob(
				assemblyline.ApplicationServiceEndpointExposureInput{Task: authority},
			))),
		leafOwnerFixture("method", station.CodingServiceEndpointMethod,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointMethodJob(
				assemblyline.ApplicationServiceEndpointMethodInput{Task: authority},
			))),
		leafOwnerFixture("route", station.CodingServiceEndpointRouteTemplate,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointRouteTemplateJob(
				assemblyline.ApplicationServiceEndpointRouteTemplateInput{Task: authority},
			))),
		leafOwnerFixture("request media", station.CodingServiceEndpointRequestMedia,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointRequestMediaJob(
				assemblyline.ApplicationServiceEndpointRequestMediaInput{
					Task: authority, Method: assemblyline.ApplicationServiceEndpointGET,
				},
			))),
		leafOwnerFixture("response media", station.CodingServiceEndpointResponseMedia,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointResponseMediaJob(
				assemblyline.ApplicationServiceEndpointResponseMediaInput{Task: authority},
			))),
		leafOwnerFixture("success status", station.CodingServiceEndpointSuccessStatus,
			mustEndpointOwnerJob(assemblyline.NewApplicationServiceEndpointSuccessStatusJob(
				assemblyline.ApplicationServiceEndpointSuccessStatusInput{
					Task: authority, Method: assemblyline.ApplicationServiceEndpointGET,
					RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
					ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
				},
			))),
	}
	for _, test := range tests {
		got, err := StationForPortableJob(test.job)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("%s station=%q want=%q", test.name, got, test.want)
		}
		correction, err := assemblyline.NewRetainedResponseCorrectionJob(
			test.job, "the one leaf is invalid", "invalid",
		)
		if err != nil {
			t.Fatalf("%s correction: %v", test.name, err)
		}
		correctedOwner, err := StationForPortableJob(correction)
		if err != nil || correctedOwner != test.want {
			t.Fatalf(
				"%s correction station=%q want=%q error=%v",
				test.name, correctedOwner, test.want, err,
			)
		}
	}
}

func leafOwnerFixture(
	name string,
	want station.ID,
	job assemblyline.PortableJob,
) struct {
	name string
	want station.ID
	job  assemblyline.PortableJob
} {
	return struct {
		name string
		want station.ID
		job  assemblyline.PortableJob
	}{name: name, want: want, job: job}
}

func mustEndpointOwnerJob(job assemblyline.PortableJob, err error) assemblyline.PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
