package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTTPRouterOrdersLiteralRoutesBeforeParameterRoutes(t *testing.T) {
	bindings := []phpServiceFeatureBinding{
		{
			Sequence: 1, RequirementID: "requirement_001", FeatureNumber: "101",
			FeatureName: "feature101", HasEndpoint: true,
			Endpoint: phpServiceRouteContract("/items/{item_id}"),
		},
		{
			Sequence: 2, RequirementID: "requirement_002", FeatureNumber: "202",
			FeatureName: "feature202", HasEndpoint: true,
			Endpoint: phpServiceRouteContract("/items/new"),
		},
	}
	ordered := phpServiceRouteOrder(bindings)
	if ordered[0].Endpoint.RouteTemplate != "/items/new" {
		t.Fatalf("PHP HTTP route order=%v", []string{
			ordered[0].Endpoint.RouteTemplate, ordered[1].Endpoint.RouteTemplate,
		})
	}
	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "parameter route"},
		{ID: "requirement_002", SourceQuote: "literal route"},
	}
	byRequirement := map[string]phpServiceFeatureBinding{
		"requirement_001": bindings[0], "requirement_002": bindings[1],
	}
	source, err := phpServiceRouterSource(
		ordered, requirements,
		directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil},
		byRequirement,
	)
	if err != nil {
		t.Fatal(err)
	}
	literal := strings.Index(source, "'/items/new'")
	parameter := strings.Index(source, "'/items/{item_id}'")
	if literal < 0 || parameter < 0 || literal >= parameter {
		t.Fatalf("literal route does not precede parameter route:\n%s", source)
	}
}

func TestPHPHTTPEndpointExecutionUsesOnlyDirectCapabilityClosure(t *testing.T) {
	requirements := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "first capability"},
		{ID: "requirement_002", SourceQuote: "second capability"},
		{ID: "requirement_003", SourceQuote: "unrelated capability"},
	}
	bindings := map[string]phpServiceFeatureBinding{
		"requirement_001": {
			Sequence: 1, RequirementID: "requirement_001", FeatureNumber: "410", FeatureName: "feature410",
		},
		"requirement_002": {
			Sequence: 2, RequirementID: "requirement_002", FeatureNumber: "720", FeatureName: "feature720",
		},
		"requirement_003": {
			Sequence: 3, RequirementID: "requirement_003", FeatureNumber: "930", FeatureName: "feature930",
		},
	}
	capabilities := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{
			RequirementID: "requirement_001", CapabilityID: "capability_001",
			Purpose: "first capability",
		}},
		"requirement_003": nil,
	}
	order, err := phpServiceEndpointExecutionOrderWithState(
		bindings["requirement_002"], requirements, capabilities, bindings,
		directCodingServiceStatePlan{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0].FeatureNumber != "410" || order[1].FeatureNumber != "720" {
		t.Fatalf("PHP endpoint execution order=%+v", order)
	}
	var source strings.Builder
	for _, binding := range order {
		invocation, invocationErr := phpServiceFeatureInvocationWithState(
			binding, capabilities, bindings, directCodingServiceStatePlan{},
		)
		if invocationErr != nil {
			t.Fatal(invocationErr)
		}
		source.WriteString(invocation)
	}
	compiled := source.String()
	if !strings.Contains(compiled, "FEATURE_720_CAPABILITY_410 => $results['capability_001']") ||
		strings.Contains(compiled, "feature930") {
		t.Fatalf("PHP direct capability projection is not exact:\n%s", compiled)
	}
}

func TestPHPHTTPCompilerRejectsUnenforceableExposure(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		exposure assemblyline.ApplicationServiceEndpointExposure
	}{
		{name: "authenticated", exposure: assemblyline.ApplicationServiceEndpointAuthenticated},
		{name: "internal", exposure: assemblyline.ApplicationServiceEndpointInternal},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
			contract := endpoints.ByTask[workload.Tasks[0].ID]
			contract.Exposure = testCase.exposure
			endpoints.ByTask[workload.Tasks[0].ID] = contract
			_, _, err := compileGenericPHPServiceBlueprint(
				"php-service", specification, map[string]directCodingSkillBinding{}, workload,
				directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
				testRequestLocalServiceStatePlan(workload), endpoints,
			)
			if err == nil || !strings.Contains(err.Error(), "requires "+string(testCase.exposure)+" exposure") {
				t.Fatalf("%s PHP endpoint error=%v", testCase.name, err)
			}
		})
	}
}

func TestPHPHTTPSupportOnlyTaskBuildsAndTestsWithoutInventingARoute(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: "HTTP service with reusable processing",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Normalize one typed value for reuse"},
			{ID: "requirement_002", SourceQuote: "Return one processed representation"},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1, Paths: []string{
		"src/Feature111.php", "src/Feature222.php",
		"tests/Feature111Test.php", "tests/Feature222Test.php",
	}}
	owners := map[string][]string{
		"src/Feature111.php": {workload.Tasks[0].ID}, "tests/Feature111Test.php": {workload.Tasks[0].ID},
		"src/Feature222.php": {workload.Tasks[1].ID}, "tests/Feature222Test.php": {workload.Tasks[1].ID},
	}
	kinds := map[string]assemblyline.TargetArtifactKind{
		"src/Feature111.php":       assemblyline.TargetArtifactImplementation,
		"tests/Feature111Test.php": assemblyline.TargetArtifactVerification,
		"src/Feature222.php":       assemblyline.TargetArtifactImplementation,
		"tests/Feature222Test.php": assemblyline.TargetArtifactVerification,
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(workload, target, owners, kinds)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := testServiceEndpointPlan(
		t, genericPHPServiceAdapter, workload,
		map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceSupportOnly,
			workload.Tasks[1].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[1].ID: testHTTPServiceEndpointContract(
				assemblyline.ApplicationServiceEndpointGET, "/processed",
				assemblyline.ApplicationServiceEndpointMediaNone,
				assemblyline.ApplicationServiceEndpointJSON, 200,
			),
		},
	)
	if err := endpoints.ValidateFor(workload); err != nil {
		t.Fatal(err)
	}
	blueprint, _, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil},
		target, coverage, testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]assemblyline.SourceDocument, len(blueprint.Documents))
	for _, document := range blueprint.Documents {
		paths[document.Path] = document
	}
	for _, required := range []string{
		"src/Feature111.php", "tests/Feature111Test.php",
		"src/Feature222.php", "tests/Feature222Test.php",
	} {
		if _, exists := paths[required]; !exists {
			t.Fatalf("support-aware blueprint omits %s", required)
		}
	}
	supportContract := paths["src/Feature111.php"].Blocks[0].Contract
	endpointContract := paths["src/Feature222.php"].Blocks[0].Contract
	endpointNarrative := phpServiceEndpointInputContract(
		endpoints.ByTask[workload.Tasks[1].ID],
	)
	if strings.Contains(supportContract, endpointNarrative) ||
		!strings.Contains(endpointContract, endpointNarrative) {
		t.Fatal("endpoint narrative was not confined to the endpoint-required task")
	}
	router := paths["public/index.php"]
	routerSource := router.Preamble + "\n" + router.Blocks[0].Static
	endpointRoute := endpoints.ByTask[workload.Tasks[1].ID].RouteTemplate
	if strings.Contains(routerSource, "Feature111") || strings.Contains(routerSource, "feature111") ||
		!strings.Contains(routerSource, "Feature222") ||
		!strings.Contains(routerSource, phpSingleQuoted(endpointRoute)) {
		t.Fatalf("support-only task leaked into code-owned HTTP behavior:\n%s", routerSource)
	}
	runner := paths["tests/TestRunner.php"].Blocks[0].Static
	if !strings.Contains(runner, "verifyFeature111") || !strings.Contains(runner, "verifyFeature222") {
		t.Fatal("final verification runner does not retain both task acceptances")
	}
}

func phpServiceRouteContract(route string) assemblyline.ApplicationServiceEndpointContract {
	return assemblyline.ApplicationServiceEndpointContract{
		Schema:        assemblyline.ApplicationServiceEndpointContractSchemaV1,
		Exposure:      assemblyline.ApplicationServiceEndpointPublic,
		Method:        assemblyline.ApplicationServiceEndpointGET,
		RouteTemplate: route,
		RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
		ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
		SuccessStatus: 200,
	}
}
