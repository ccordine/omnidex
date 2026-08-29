package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTTPTaskFixturesDeriveAcceptedRouteAndMediaShapes(t *testing.T) {
	testCases := []struct {
		name        string
		media       assemblyline.ApplicationServiceEndpointMedia
		body        string
		payloadPart string
		post        bool
	}{
		{name: "none", media: assemblyline.ApplicationServiceEndpointMediaNone, payloadPart: "null"},
		{name: "json", media: assemblyline.ApplicationServiceEndpointJSON, body: `{"fixture":"value"}`, payloadPart: "'fixture' => 'value'"},
		{name: "form", media: assemblyline.ApplicationServiceEndpointForm, body: "fixture=value", payloadPart: "'fixture' => 'value'", post: true},
		{name: "multipart", media: assemblyline.ApplicationServiceEndpointMultipart, payloadPart: "'fields' => ['fixture' => 'value']", post: true},
		{name: "xml", media: assemblyline.ApplicationServiceEndpointXML, body: "<fixture>value</fixture>", payloadPart: "<fixture>value</fixture>"},
		{name: "text", media: assemblyline.ApplicationServiceEndpointText, body: "value", payloadPart: "'value'"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			contract := assemblyline.ApplicationServiceEndpointContract{
				Schema:        assemblyline.ApplicationServiceEndpointContractSchemaV1,
				Exposure:      assemblyline.ApplicationServiceEndpointPublic,
				Method:        assemblyline.ApplicationServiceEndpointPOST,
				RouteTemplate: "/values/{value_id}", RequestMedia: testCase.media,
				ResponseMedia: assemblyline.ApplicationServiceEndpointJSON, SuccessStatus: 201,
			}
			fixture, err := phpServiceTaskInputFixture(
				assemblyline.ApplicationServiceEndpointRequired, contract,
			)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.Method != "POST" || fixture.Path != "/values/1" || fixture.Body != testCase.body ||
				len(fixture.RouteParameters) != 1 || fixture.RouteParameters[0].Key != "value_id" ||
				len(fixture.Query) != 1 || !strings.Contains(fixture.Payload, testCase.payloadPart) ||
				(testCase.post != (len(fixture.Post) == 1)) {
				t.Fatalf("derived PHP fixture=%+v", fixture)
			}
			hasContentType := false
			for _, header := range fixture.Headers {
				if header.Key == "content-type" && header.Value == string(testCase.media) {
					hasContentType = true
				}
			}
			if (testCase.media != assemblyline.ApplicationServiceEndpointMediaNone) != hasContentType {
				t.Fatalf("derived PHP fixture headers=%+v", fixture.Headers)
			}
		})
	}
	local, err := phpServiceTaskInputFixture(
		assemblyline.ApplicationServiceSupportOnly, assemblyline.ApplicationServiceEndpointContract{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if local.Method != "LOCAL" || local.Path != "/" || len(local.Headers) != 0 || local.Payload == "null" {
		t.Fatalf("support-only PHP fixture=%+v", local)
	}
}

func TestPHPHTTPFinalRunnerOwnsDispatchContractSmoke(t *testing.T) {
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	blueprint, _, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]assemblyline.SourceDocument, len(blueprint.Documents))
	for _, document := range blueprint.Documents {
		byPath[document.Path] = document
	}
	fixture := byPath["tests/Feature101Test.php"].Blocks[0].Static
	if !strings.Contains(fixture, "taskInputFixture101") ||
		!strings.Contains(fixture, "'GET', '/'") ||
		strings.Contains(fixture, "'record_id' => '1'") ||
		strings.Contains(fixture, "TaskInput::example") {
		t.Fatalf("endpoint-shaped acceptance fixture is not exact:\n%s", fixture)
	}
	router := byPath["public/index.php"].Blocks[0].Static
	if !strings.Contains(router, "function dispatchApplicationHttp(HttpRequest $request): HttpResponse") {
		t.Fatal("public entrypoint lacks its code-owned dispatch seam")
	}
	runner := byPath["tests/TestRunner.php"].Blocks[0].Static
	for _, required := range []string{
		"dispatchApplicationHttp", "->status !== 200", "->media !== 'text/html'",
		"'OPTIONS'", "->status !== 405", "->status !== 404",
	} {
		if !strings.Contains(runner, required) {
			t.Fatalf("PHP HTTP final runner lacks %s smoke assertion:\n%s", required, runner)
		}
	}
}
