package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceEndpointLeafStationsExposeOneSemanticFieldEach(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	jobs := []struct {
		name string
		kind WorkKind
		job  PortableJob
		leaf string
	}{
		leafJobFixture("exposure", WorkApplicationServiceEndpointExposure,
			mustEndpointLeafJob(NewApplicationServiceEndpointExposureJob(ApplicationServiceEndpointExposureInput{Task: authority}))),
		leafJobFixture("method", WorkApplicationServiceEndpointMethod,
			mustEndpointLeafJob(NewApplicationServiceEndpointMethodJob(ApplicationServiceEndpointMethodInput{Task: authority}))),
		leafJobFixture("route", WorkApplicationServiceEndpointRouteTemplate,
			mustEndpointLeafJob(NewApplicationServiceEndpointRouteTemplateJob(ApplicationServiceEndpointRouteTemplateInput{Task: authority}))),
		leafJobFixture("request media", WorkApplicationServiceEndpointRequestMedia,
			mustEndpointLeafJob(NewApplicationServiceEndpointRequestMediaJob(ApplicationServiceEndpointRequestMediaInput{
				Task: authority, Method: ApplicationServiceEndpointGET,
			}))),
		leafJobFixture("response media", WorkApplicationServiceEndpointResponseMedia,
			mustEndpointLeafJob(NewApplicationServiceEndpointResponseMediaJob(ApplicationServiceEndpointResponseMediaInput{Task: authority}))),
		leafJobFixture("success status", WorkApplicationServiceEndpointSuccessStatus,
			mustEndpointLeafJob(NewApplicationServiceEndpointSuccessStatusJob(ApplicationServiceEndpointSuccessStatusInput{
				Task: authority, Method: ApplicationServiceEndpointGET,
				RequestMedia: ApplicationServiceEndpointMediaNone, ResponseMedia: ApplicationServiceEndpointJSON,
			}))),
	}
	seenIDs := make(map[string]WorkKind, len(jobs))
	for _, item := range jobs {
		item := item
		t.Run(item.name, func(t *testing.T) {
			if previous, duplicate := seenIDs[item.job.ID]; duplicate {
				t.Fatalf("leaf work %s reused persisted identity owned by %s", item.kind, previous)
			}
			seenIDs[item.job.ID] = item.kind
			prompt, schema, err := RenderPortableJob(item.job)
			if err != nil {
				t.Fatal(err)
			}
			if item.job.Kind != item.kind || !strings.Contains(prompt, "LOCAL_ACCEPTED_AUTHORITY_JSON") {
				t.Fatalf("kind=%q prompt=%q", item.job.Kind, prompt)
			}
			properties := schema["properties"].(map[string]any)
			if len(properties) != 2 || properties["schema"] == nil || properties[item.leaf] == nil {
				t.Fatalf("leaf schema properties=%#v", properties)
			}
			for _, other := range []string{
				"exposure", "method", "route_template", "request_media", "response_media", "success_status",
			} {
				if other != item.leaf && properties[other] != nil {
					t.Fatalf("%s station also exposed %s", item.leaf, other)
				}
			}
		})
	}
}

func TestApplicationServiceEndpointLeafEnvelopesExcludeVerificationAuthority(t *testing.T) {
	t.Parallel()
	workloadInput, frozen := applicationTaskAuthorityProjectionFixture(t)
	runtimeAuthority, err := ProjectApplicationTaskRuntimeAuthority(
		workloadInput, frozen, "task_001",
	)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ProjectApplicationServiceEndpointTaskAuthority(runtimeAuthority)
	if err != nil {
		t.Fatal(err)
	}
	jobs := []PortableJob{
		mustEndpointLeafJob(NewApplicationServiceEndpointExposureJob(
			ApplicationServiceEndpointExposureInput{Task: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointMethodJob(
			ApplicationServiceEndpointMethodInput{Task: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointRouteTemplateJob(
			ApplicationServiceEndpointRouteTemplateInput{Task: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointRequestMediaJob(
			ApplicationServiceEndpointRequestMediaInput{
				Task: authority, Method: ApplicationServiceEndpointPOST,
			},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointResponseMediaJob(
			ApplicationServiceEndpointResponseMediaInput{Task: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointSuccessStatusJob(
			ApplicationServiceEndpointSuccessStatusInput{
				Task: authority, Method: ApplicationServiceEndpointPOST,
				RequestMedia:  ApplicationServiceEndpointJSON,
				ResponseMedia: ApplicationServiceEndpointJSON,
			},
		)),
	}
	criterion := frozen.Tasks[0].AcceptanceCriteria[0]
	for _, job := range jobs {
		prompt, _, renderErr := RenderPortableJob(job)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if strings.Contains(prompt, criterion) || strings.Contains(prompt, `"acceptance_criteria"`) {
			t.Fatalf("endpoint leaf %s exposed verification authority: %s", job.Kind, prompt)
		}
	}
}

func TestApplicationServiceEndpointLeafDecodersRejectAggregateAndInvalidPrerequisites(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	exposureInput := ApplicationServiceEndpointExposureInput{Task: authority}
	if _, err := DecodeApplicationServiceEndpointExposureResult(
		exposureInput,
		`{"schema":"omnidex.application-service-endpoint-exposure.v1","exposure":"public","method":"GET"}`,
	); err == nil {
		t.Fatal("exposure leaf accepted an aggregate field")
	}
	requestInput := ApplicationServiceEndpointRequestMediaInput{
		Task: authority, Method: ApplicationServiceEndpointGET,
	}
	if _, err := DecodeApplicationServiceEndpointRequestMediaResult(
		requestInput,
		`{"schema":"omnidex.application-service-endpoint-request-media.v1","request_media":"application/json"}`,
	); err == nil {
		t.Fatal("request-media leaf accepted a GET body")
	}
	routeInput := ApplicationServiceEndpointRouteTemplateInput{Task: authority}
	if result, err := DecodeApplicationServiceEndpointRouteTemplateResult(
		routeInput,
		`{"schema":"omnidex.application-service-endpoint-route-template.v1","route_template":"/records/{record_id}"}`,
	); err != nil || result.RouteTemplate != "/records/{record_id}" {
		t.Fatalf("typed route leaf result=%+v error=%v", result, err)
	}
	if _, err := DecodeApplicationServiceEndpointRouteTemplateResult(
		routeInput,
		`{"schema":"omnidex.application-service-endpoint-route-template.v1","route_template":"/Records"}`,
	); err == nil {
		t.Fatal("route leaf bypassed the typed HTTP-route validator")
	}
	statusInput := ApplicationServiceEndpointSuccessStatusInput{
		Task: authority, Method: ApplicationServiceEndpointPOST,
		RequestMedia: ApplicationServiceEndpointJSON, ResponseMedia: ApplicationServiceEndpointMediaNone,
	}
	if _, err := DecodeApplicationServiceEndpointSuccessStatusResult(
		statusInput,
		`{"schema":"omnidex.application-service-endpoint-success-status.v1","success_status":201}`,
	); err == nil {
		t.Fatal("success-status leaf accepted a payload status without response media")
	}
}

func TestApplicationServiceEndpointCandidateSetsExposeOnlyUnresolvedChoices(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	requestCandidates, err := ApplicationServiceEndpointRequestMediaCandidates(
		ApplicationServiceEndpointRequestMediaInput{
			Task: authority, Method: ApplicationServiceEndpointGET,
		},
	)
	if err != nil || len(requestCandidates) != 1 ||
		requestCandidates[0] != ApplicationServiceEndpointMediaNone {
		t.Fatalf("GET request candidates=%v error=%v", requestCandidates, err)
	}
	statusCandidates, err := ApplicationServiceEndpointSuccessStatusCandidates(
		ApplicationServiceEndpointSuccessStatusInput{
			Task: authority, Method: ApplicationServiceEndpointPOST,
			RequestMedia:  ApplicationServiceEndpointJSON,
			ResponseMedia: ApplicationServiceEndpointMediaNone,
		},
	)
	if err != nil || len(statusCandidates) != 1 || statusCandidates[0] != 204 {
		t.Fatalf("no-content status candidates=%v error=%v", statusCandidates, err)
	}
	if _, err := ApplicationServiceEndpointSuccessStatusCandidates(
		ApplicationServiceEndpointSuccessStatusInput{
			Task: authority, Method: ApplicationServiceEndpointGET,
			RequestMedia:  ApplicationServiceEndpointMediaNone,
			ResponseMedia: ApplicationServiceEndpointMediaNone,
		},
	); err == nil {
		t.Fatal("accepted a GET contract with no compatible response status")
	}
}

func leafJobFixture(
	name string,
	kind WorkKind,
	job PortableJob,
) struct {
	name string
	kind WorkKind
	job  PortableJob
	leaf string
} {
	leaves := map[WorkKind]string{
		WorkApplicationServiceEndpointExposure:      "exposure",
		WorkApplicationServiceEndpointMethod:        "method",
		WorkApplicationServiceEndpointRouteTemplate: "route_template",
		WorkApplicationServiceEndpointRequestMedia:  "request_media",
		WorkApplicationServiceEndpointResponseMedia: "response_media",
		WorkApplicationServiceEndpointSuccessStatus: "success_status",
	}
	return struct {
		name string
		kind WorkKind
		job  PortableJob
		leaf string
	}{name: name, kind: kind, job: job, leaf: leaves[kind]}
}

func mustEndpointLeafJob(job PortableJob, err error) PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
