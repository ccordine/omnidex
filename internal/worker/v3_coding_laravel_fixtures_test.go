package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLaravelProfileQualifiesTwoUnrelatedBoundedFixtures(t *testing.T) {
	t.Parallel()

	weather := laravelFixtureProgram(t, laravelWeatherFixtureInput())
	weatherFiles := validateLaravelFixtureAssembly(t, weather)
	for _, forbidden := range []string{
		"config/database.php", laravelStateMigrationPath, phpServiceStateVerificationPath,
	} {
		if _, exists := weatherFiles[forbidden]; exists {
			t.Fatalf("request-local weather fixture retained %s", forbidden)
		}
	}
	for _, forbidden := range []string{"pdo_pgsql", "  db:", "service-state:", "DATABASE_PASSWORD"} {
		if strings.Contains(weatherFiles["Dockerfile"]+weatherFiles["docker-compose.yml"]+
			weatherFiles[laravelVerificationEnvPath], forbidden) {
			t.Fatalf("request-local weather fixture retained %q", forbidden)
		}
	}
	if !strings.Contains(weatherFiles["routes/web.php"], "Route::match") ||
		!strings.Contains(weatherFiles["src/Feature101.php"], "RuntimeHtml::document") {
		t.Fatal("weather fixture lost Laravel routing or server rendering")
	}
	if strings.Count(weatherFiles["routes/web.php"], laravelReadinessRouteSource()) != 1 ||
		!strings.Contains(weatherFiles["tests/HttpVerifier.php"],
			"performHttpRequest('GET', "+phpSingleQuoted(directCodingDeploymentReadinessPath)+", [], '')") ||
		!strings.Contains(weatherFiles["bootstrap/app.php"], "Route::group([], base_path('routes/web.php'))") {
		t.Fatal("weather fixture lost its middleware-free Laravel readiness route")
	}
	if strings.Count(weatherFiles["docker-compose.yml"], "    healthcheck:\n") != 2 ||
		strings.Count(weatherFiles["docker-compose.yml"], laravelNginxHealthcheck()) != 1 {
		t.Fatal("request-local weather fixture lost exact app or NGINX health authority")
	}
	if !strings.Contains(weatherFiles["nginx/nginx.conf"], "include /etc/nginx/mime.types;") ||
		!strings.Contains(weatherFiles["nginx/nginx.conf"], "default_type application/octet-stream;") {
		t.Fatal("request-local weather fixture lost NGINX static media authority")
	}

	checkout := laravelFixtureProgram(t, laravelCheckoutFixtureInput())
	checkoutFiles := validateLaravelFixtureAssembly(t, checkout)
	if checkoutFiles[laravelStateMigrationPath] != laravelServiceStateMigrationSource() {
		t.Fatal("durable checkout fixture lost the canonical Laravel migration wrapper")
	}
	for _, required := range []string{
		"final class RuntimeState", "Illuminate\\Support\\Facades\\DB::table",
	} {
		if !strings.Contains(checkoutFiles["src/Runtime.php"], required) {
			t.Fatalf("durable checkout runtime omits %s", required)
		}
	}
	runner := checkoutFiles["tests/TestRunner.php"]
	if strings.Count(runner, phpServiceStateResetFunctionName+"();") != 2 ||
		!strings.Contains(checkoutFiles[phpServiceStateVerificationPath], "LaravelBootstrap.php") {
		t.Fatal("durable checkout fixture lacks per-verifier or process-boundary isolation")
	}
	for _, required := range []string{
		"  db:", "service-state:/var/lib/postgresql", "docker-php-ext-install mbstring pdo_pgsql",
		"${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}",
	} {
		if !strings.Contains(checkoutFiles["Dockerfile"]+checkoutFiles["docker-compose.yml"], required) {
			t.Fatalf("durable checkout fixture omits %s", required)
		}
	}
	if strings.Count(checkoutFiles["docker-compose.yml"], "    healthcheck:\n") != 3 ||
		strings.Count(checkoutFiles["docker-compose.yml"], laravelNginxHealthcheck()) != 1 {
		t.Fatal("durable checkout fixture lost exact database, app, or NGINX health authority")
	}
}

func TestLaravelReservedReadinessRouteRejectsGeneratedCollisions(t *testing.T) {
	t.Parallel()

	for _, route := range []string{
		directCodingDeploymentReadinessPath,
		"/{scope}/health",
		"/__omnidex/{probe}",
	} {
		if err := validateLaravelReservedEndpointRoute("task_fixture", route); err == nil ||
			!strings.Contains(err.Error(), "reserved readiness route") {
			t.Fatalf("reserved route collision %q error=%v", route, err)
		}
	}
	if err := validateLaravelReservedEndpointRoute("task_fixture", "/regional/health"); err != nil {
		t.Fatalf("unrelated route rejected: %v", err)
	}
}

func laravelWeatherFixtureInput() laravelFixtureInput {
	return laravelFixtureInput{
		PackageName: "regional-weather", Product: "Regional weather display",
		Requirement: "Show the current forecast for a selected region.",
		Objective:   "Return a server-rendered regional forecast.",
		Behaviors:   []string{"Read the accepted region route parameter."},
		Criteria: []string{
			"The response identifies the selected region.",
			"The response reports no failure for a valid region.",
		},
		Route: "/forecast/{region_code}", Method: assemblyline.ApplicationServiceEndpointGET,
		RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
		ResponseMedia: assemblyline.ApplicationServiceEndpointHTML, Status: 200,
		FeatureSource: `function feature101(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success('Forecast ' . ($input->routeParameters['region_code'] ?? ''), ['region' => $input->routeParameters['region_code'] ?? '']);
}`,
		RepresentationSource: `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document(
        'Regional forecast',
        '<main class="min-h-screen px-4 py-8 md:px-8"><h1 class="text-2xl font-semibold">Regional forecast</h1><p class="mt-4">' . RuntimeHtml::escape($result->output) . '</p></main>',
    );
}`,
		AcceptanceSource: `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === 'Forecast 1', 'expected selected region output');
    RuntimeAssertions::require($result, $result->error === '', 'expected successful forecast');
}`,
	}
}

func laravelCheckoutFixtureInput() laravelFixtureInput {
	return laravelFixtureInput{
		PackageName: "equipment-checkout", Product: "Equipment checkout endpoint",
		Requirement: "Record an equipment checkout across requests.",
		Objective:   "Persist and report each accepted equipment checkout.",
		Behaviors:   []string{"Increment the durable checkout count."},
		Criteria: []string{
			"A valid checkout reports that it was recorded.",
			"The returned state contains the incremented durable count.",
		},
		Route: "/equipment/checkouts", Method: assemblyline.ApplicationServiceEndpointPOST,
		RequestMedia:  assemblyline.ApplicationServiceEndpointJSON,
		ResponseMedia: assemblyline.ApplicationServiceEndpointJSON, Status: 201,
		Durable: true,
		FeatureSource: `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $state = FeatureState101::load();
    $count = (int) ($state['count'] ?? 0) + 1;
    FeatureState101::save(['count' => $count]);
    return TaskResult::success('Checkout recorded', ['count' => $count]);
}`,
		AcceptanceSource: `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === 'Checkout recorded', 'expected recorded checkout');
    RuntimeAssertions::require($result, $result->state === ['count' => 1], 'expected first durable count');
}`,
	}
}

type laravelFixtureInput struct {
	PackageName, Product, Requirement, Objective string
	Behaviors, Criteria                          []string
	Route                                        string
	Method                                       assemblyline.ApplicationServiceEndpointMethod
	RequestMedia                                 assemblyline.ApplicationServiceEndpointMedia
	ResponseMedia                                assemblyline.ApplicationServiceEndpointMedia
	Status                                       int
	Durable                                      bool
	FeatureSource, RepresentationSource          string
	AcceptanceSource                             string
}

func laravelFixtureProgram(t *testing.T, fixture laravelFixtureInput) directCodingProgram {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: fixture.Product,
		Requirements: []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: fixture.Requirement}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(
		applicationWorkloadInput(specification), assemblyline.ApplicationWorkloadDraft{
			Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
			Tasks: []assemblyline.ApplicationWorkloadTaskDraft{{
				RequirementID: "requirement_001", Objective: fixture.Objective,
				RequiredBehaviors: fixture.Behaviors, AcceptanceCriteria: fixture.Criteria,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID: laravelHTTPServiceAdapter, VersionProfileID: laravelVersionProfileV1,
		Paths: []string{"src/Feature101.php", "tests/Feature101Test.php"},
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"src/Feature101.php": {workload.Tasks[0].ID}, "tests/Feature101Test.php": {workload.Tasks[0].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"src/Feature101.php":       assemblyline.TargetArtifactImplementation,
			"tests/Feature101Test.php": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	state := testRequestLocalServiceStatePlan(workload)
	capabilities := directCodingCapabilityGraph{"requirement_001": nil}
	if fixture.Durable {
		state.ByTask[workload.Tasks[0].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
		state = bindTestServiceStateInterfaces(
			t, workload, capabilities, state, testIntegerServiceStateField("count"),
		)
	}
	endpoints := directCodingServiceEndpointPlan{
		WorkloadSHA256: workload.SHA256, ProductContext: specification.ProductQuote,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[0].ID: {
				Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
				Exposure: assemblyline.ApplicationServiceEndpointPublic,
				Method:   fixture.Method, RouteTemplate: fixture.Route,
				RequestMedia: fixture.RequestMedia, ResponseMedia: fixture.ResponseMedia,
				SuccessStatus: fixture.Status,
			},
		},
	}
	blueprint, staticFiles, err := compileGenericLaravelServiceBlueprint(
		fixture.PackageName, specification, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage, state, endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{
		"feature.001": fixture.FeatureSource, "acceptance.001": fixture.AcceptanceSource,
	}
	if fixture.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
		generated["representation.html.001"] = fixture.RepresentationSource
	}
	return directCodingProgram{
		StackID: laravelHTTPServiceAdapter, VersionProfileID: laravelVersionProfileV1,
		Workload: workload, TargetTree: target, Coverage: coverage,
		ServiceState: state, ServiceEndpoints: endpoints,
		Source: blueprint, StaticFiles: staticFiles, Generated: generated,
	}
}

func validateLaravelFixtureAssembly(t *testing.T, program directCodingProgram) map[string]string {
	t.Helper()
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	return directCodingAssemblyFiles(assembly)
}
