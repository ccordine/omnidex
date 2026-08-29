package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceEndpointLeafEnvelopesContainOnlyExactAuthorityAndPrerequisites(t *testing.T) {
	t.Parallel()
	runtimeAuthority := ApplicationTaskRuntimeAuthority{
		WorkloadSHA256: strings.Repeat("a", 64),
		TaskID:         "task_sensitive_001", RequirementID: "requirement_sensitive_001",
		Surface: ApplicationSurfaceBrowser, ProductQuote: "inventory browser",
		RequirementQuote: "Clients can create one inventory record.",
	}
	authority, err := ProjectApplicationServiceEndpointTaskAuthority(runtimeAuthority)
	if err != nil {
		t.Fatal(err)
	}
	jobs := []PortableJob{
		mustEndpointLeafJob(NewApplicationServiceEndpointExposureJob(
			ApplicationServiceEndpointExposureInput{Authority: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointMethodJob(
			ApplicationServiceEndpointMethodInput{Authority: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointRouteTemplateJob(
			ApplicationServiceEndpointRouteTemplateInput{Authority: authority},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointRequestMediaJob(
			ApplicationServiceEndpointRequestMediaInput{Authority: authority, Method: ApplicationServiceEndpointPOST},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointResponseMediaJob(
			ApplicationServiceEndpointResponseMediaInput{Authority: authority, Method: ApplicationServiceEndpointPOST},
		)),
		mustEndpointLeafJob(NewApplicationServiceEndpointSuccessStatusJob(
			ApplicationServiceEndpointSuccessStatusInput{
				Authority: authority, Method: ApplicationServiceEndpointPOST,
				RequestMedia: ApplicationServiceEndpointJSON, ResponseMedia: ApplicationServiceEndpointHTML,
			},
		)),
	}
	seen := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		if _, duplicate := seen[job.ID]; duplicate {
			t.Fatalf("endpoint leaf job identity %s was reused", job.ID)
		}
		seen[job.ID] = struct{}{}
		prompt, renderErr := RenderPortableJob(job)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		for _, required := range []string{
			"PRODUCT CONTEXT:\ninventory browser",
			"EXACT ENDPOINT REQUIREMENT:\nClients can create one inventory record.",
			"ACCEPTED APPLICATION SURFACE:\nbrowser_application",
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("endpoint leaf %s omitted %q: %s", job.Kind, required, prompt)
			}
		}
		for _, forbidden := range []string{
			runtimeAuthority.WorkloadSHA256, runtimeAuthority.TaskID, runtimeAuthority.RequirementID,
			"workload_sha256", "task_id", "requirement_id", "LOCAL_ACCEPTED", "AUTHORITY_JSON",
			"acceptance", "objective", "behavior", "workspace", "ordinal", "sequence",
		} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("endpoint leaf %s leaked %q: %s", job.Kind, forbidden, prompt)
			}
		}
		if job.Kind == WorkApplicationServiceEndpointResponseMedia &&
			!strings.Contains(prompt, "ACCEPTED HTTP METHOD:\nPOST") {
			t.Fatalf("response-media leaf omitted accepted method: %s", prompt)
		}
	}
}

func TestApplicationServiceEndpointLeafDecodersAndCompositionStayNarrow(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	exposure, err := DecodeApplicationServiceEndpointExposureResult(
		ApplicationServiceEndpointExposureInput{Authority: authority}, "public",
	)
	if err != nil {
		t.Fatal(err)
	}
	method, err := DecodeApplicationServiceEndpointMethodResult(
		ApplicationServiceEndpointMethodInput{Authority: authority}, "POST",
	)
	if err != nil {
		t.Fatal(err)
	}
	route, err := DecodeApplicationServiceEndpointRouteTemplateResult(
		ApplicationServiceEndpointRouteTemplateInput{Authority: authority}, "/records/{record_id}",
	)
	if err != nil {
		t.Fatal(err)
	}
	requestMedia, err := DecodeApplicationServiceEndpointRequestMediaResult(
		ApplicationServiceEndpointRequestMediaInput{Authority: authority, Method: method.Method},
		"application/json",
	)
	if err != nil {
		t.Fatal(err)
	}
	responseMedia, err := DecodeApplicationServiceEndpointResponseMediaResult(
		ApplicationServiceEndpointResponseMediaInput{Authority: authority, Method: method.Method}, "application/json",
	)
	if err != nil {
		t.Fatal(err)
	}
	status, err := DecodeApplicationServiceEndpointSuccessStatusResult(
		ApplicationServiceEndpointSuccessStatusInput{
			Authority: authority, Method: method.Method,
			RequestMedia: requestMedia.RequestMedia, ResponseMedia: responseMedia.ResponseMedia,
		}, "201",
	)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := ComposeApplicationServiceEndpointContract(
		authority, exposure, method, route, requestMedia, responseMedia, status,
	)
	if err != nil {
		t.Fatal(err)
	}
	if contract.RouteTemplate != "/records/{record_id}" || contract.SuccessStatus != 201 {
		t.Fatalf("composed endpoint contract=%+v", contract)
	}
	if _, err := DecodeApplicationServiceEndpointExposureResult(
		ApplicationServiceEndpointExposureInput{Authority: authority},
		`{"exposure":"public","method":"GET"}`,
	); err == nil {
		t.Fatal("exposure leaf accepted an aggregate contract")
	}
}

func TestApplicationServiceEndpointCompatibleCandidatesAreCodeOwned(t *testing.T) {
	t.Parallel()
	authority := testServiceEndpointTaskAuthority()
	requestCandidates, err := ApplicationServiceEndpointRequestMediaCandidates(
		ApplicationServiceEndpointRequestMediaInput{Authority: authority, Method: ApplicationServiceEndpointGET},
	)
	if err != nil || len(requestCandidates) != 1 || requestCandidates[0] != ApplicationServiceEndpointMediaNone {
		t.Fatalf("GET request candidates=%v error=%v", requestCandidates, err)
	}
	statusCandidates, err := ApplicationServiceEndpointSuccessStatusCandidates(
		ApplicationServiceEndpointSuccessStatusInput{
			Authority: authority, Method: ApplicationServiceEndpointPOST,
			RequestMedia: ApplicationServiceEndpointJSON, ResponseMedia: ApplicationServiceEndpointMediaNone,
		},
	)
	if err != nil || len(statusCandidates) != 1 || statusCandidates[0] != 204 {
		t.Fatalf("no-content status candidates=%v error=%v", statusCandidates, err)
	}
}

func mustEndpointLeafJob(job PortableJob, err error) PortableJob {
	if err != nil {
		panic(err)
	}
	return job
}
