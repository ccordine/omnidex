package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPRendererReceivesOnlyOwnAndDirectDependencyRoutes(t *testing.T) {
	t.Parallel()
	endpoint := func(method assemblyline.ApplicationServiceEndpointMethod) assemblyline.ApplicationServiceEndpointContract {
		return assemblyline.ApplicationServiceEndpointContract{
			Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
			Exposure: assemblyline.ApplicationServiceEndpointPublic,
			Method:   method, RouteTemplate: "/registered",
			RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
			ResponseMedia: assemblyline.ApplicationServiceEndpointHTML, SuccessStatus: 200,
		}
	}
	owner := phpServiceFeatureBinding{
		TaskID: "task_001", RequirementID: "requirement_001", HasEndpoint: true,
		RequirementQuote: "Show the current inventory record.",
		RouteBlockID:     "runtime.route.001", RouteName: "routeFeature101",
		Endpoint: endpoint(assemblyline.ApplicationServiceEndpointGET),
	}
	direct := phpServiceFeatureBinding{
		TaskID: "task_002", RequirementID: "requirement_002", HasEndpoint: true,
		RequirementQuote: "Create a new inventory record.",
		RouteBlockID:     "runtime.route.002", RouteName: "routeFeature102",
		Endpoint: endpoint(assemblyline.ApplicationServiceEndpointPOST),
	}
	unrelated := phpServiceFeatureBinding{
		TaskID: "task_003", RequirementID: "requirement_003", HasEndpoint: true,
		RequirementQuote: "Delete an archived inventory record.",
		RouteBlockID:     "runtime.route.003", RouteName: "routeFeature103",
		Endpoint: endpoint(assemblyline.ApplicationServiceEndpointDELETE),
	}
	routes, err := phpServiceRendererRouteBindings(
		owner,
		[]directCodingCapabilityBinding{{RequirementID: direct.RequirementID}},
		map[string]phpServiceFeatureBinding{
			owner.RequirementID: owner, direct.RequirementID: direct,
			unrelated.RequirementID: unrelated,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[0].TaskID != owner.TaskID || routes[1].TaskID != direct.TaskID {
		t.Fatalf("renderer routes=%+v", routes)
	}
	contract := strings.Join(phpServiceRendererRouteContract(routes), "\n")
	if strings.Contains(contract, unrelated.RouteName) || strings.Contains(contract, "/registered") {
		t.Fatalf("route contract exposed unrelated or concrete route authority: %s", contract)
	}
	for _, required := range []string{
		owner.RequirementQuote, owner.RouteName, string(owner.Endpoint.Method),
		direct.RequirementQuote, direct.RouteName, string(direct.Endpoint.Method),
	} {
		if !strings.Contains(contract, required) {
			t.Fatalf("route contract omitted semantic binding %q: %s", required, contract)
		}
	}
	for _, forbidden := range []string{owner.TaskID, direct.TaskID, "code-owned", "exact accepted requirement"} {
		if strings.Contains(contract, forbidden) {
			t.Fatalf("route contract exposed meta-authority %q: %s", forbidden, contract)
		}
	}
}

func TestPHPHTMLRendererBuildsUnrelatedDynamicScheduleLinks(t *testing.T) {
	t.Parallel()
	input := assemblyline.FragmentGenerationInput{
		Language: "php", Dialect: "PHP >=8.2,<9",
		Signature: "function renderFeature201HTML(TaskResult $result): string",
		Behavior:  "Render scheduled sessions with registered detail links.",
		PermittedSymbols: []string{
			phpServiceTaskResultAPI(), phpServiceHTMLRuntimeAPI(),
			"function routeFeature202(string $session_id): RuntimeRoute",
		},
	}
	source := `function renderFeature201HTML(TaskResult $result): string {
    $sessions = '';
    foreach (RuntimeHtml::records($result->state, 'sessions') as $session) {
        $sessions .= '<li>' . RuntimeHtml::escape(RuntimeHtml::field($session, 'starts_at')) .
            RuntimeHtml::link(routeFeature202(RuntimeHtml::field($session, 'id')), 'Open session') . '</li>';
    }
    return RuntimeHtml::document(
        'Schedule',
        '<main class="min-h-screen p-4 lg:p-10"><h1>Schedule</h1><ul>' . $sessions . '</ul></main>',
    );
}`
	if _, err := validateDirectCodingPHPFragment(input, source); err != nil {
		t.Fatal(err)
	}
}
