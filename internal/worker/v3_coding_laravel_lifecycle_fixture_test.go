package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLaravelHTTPLifecycleQualifiesTwoUnrelatedAssemblies(t *testing.T) {
	t.Parallel()
	fixtures := []laravelLifecycleFixture{
		{
			Package: "equipment-history", Product: "Equipment inspection history",
			WriteRequirement: "Retain an accepted equipment inspection between requests.",
			ReadRequirement:  "Present the current equipment inspection history.",
		},
		{
			Package: "clinic-schedule", Product: "Clinic schedule registry",
			WriteRequirement: "Retain an accepted clinic schedule between requests.",
			ReadRequirement:  "Present the current clinic schedule.",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Package, func(t *testing.T) {
			program := laravelLifecycleFixtureProgram(t, fixture)
			files := validateLaravelFixtureAssembly(t, program)
			verifier := files["tests/HttpVerifier.php"]
			writeRoute := program.ServiceEndpoints.ByTask[program.Workload.Tasks[0].ID].RouteTemplate
			readRoute := program.ServiceEndpoints.ByTask[program.Workload.Tasks[1].ID].RouteTemplate
			for _, required := range []string{
				"verificationLifecycleRequest(", "verifyLifecycleSentinel(",
				phpSingleQuoted(writeRoute), phpSingleQuoted(readRoute),
				"odx-1-", "odx-2-",
			} {
				if !strings.Contains(verifier, required) {
					t.Fatalf("Laravel lifecycle verifier omitted %q", required)
				}
			}
			if strings.Contains(verifier, "no mechanically verifiable cross-endpoint lifecycle") ||
				strings.Count(verifier, "verificationLifecycleRequest(") != 3 {
				t.Fatalf("Laravel lifecycle verifier is not one two-phase proof:\n%s", verifier)
			}
			routes := files["routes/web.php"]
			if !strings.Contains(routes, "TaskResult::success('', FeatureState202::load())") ||
				strings.Count(routes, "feature101($input") != 1 {
				t.Fatalf("Laravel GET route re-invokes its durable writer:\n%s", routes)
			}
		})
	}
}

type laravelLifecycleFixture struct {
	Package, Product                  string
	WriteRequirement, ReadRequirement string
}

func laravelLifecycleFixtureProgram(
	t *testing.T,
	fixture laravelLifecycleFixture,
) directCodingProgram {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: fixture.Product,
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: fixture.WriteRequirement},
			{ID: "requirement_002", SourceQuote: fixture.ReadRequirement},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID: laravelHTTPServiceAdapter, VersionProfileID: laravelVersionProfileV1,
		Paths: []string{
			"src/Feature101.php", "src/Feature202.php",
			"tests/Feature101Test.php", "tests/Feature202Test.php",
		},
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"src/Feature101.php": {workload.Tasks[0].ID}, "tests/Feature101Test.php": {workload.Tasks[0].ID},
			"src/Feature202.php": {workload.Tasks[1].ID}, "tests/Feature202Test.php": {workload.Tasks[1].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"src/Feature101.php": assemblyline.TargetArtifactImplementation, "tests/Feature101Test.php": assemblyline.TargetArtifactVerification,
			"src/Feature202.php": assemblyline.TargetArtifactImplementation, "tests/Feature202Test.php": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{
			RequirementID: "requirement_001", CapabilityID: genericApplicationCapabilityID(1),
			Purpose: fixture.WriteRequirement,
		}},
	}
	state := testRequestLocalServiceStatePlan(workload)
	state.ByTask[workload.Tasks[0].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	state = bindTestServiceStateInterfaces(
		t, workload, capabilities, state,
		testStringServiceStateField("durable feature state"),
	)
	endpoints := testServiceEndpointPlan(
		t, laravelHTTPServiceAdapter, workload,
		map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
			workload.Tasks[1].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[0].ID: testHTTPServiceEndpointContract(
				assemblyline.ApplicationServiceEndpointPOST, "/fixture-001",
				assemblyline.ApplicationServiceEndpointJSON,
				assemblyline.ApplicationServiceEndpointJSON, 200,
			),
			workload.Tasks[1].ID: testHTTPServiceEndpointContract(
				assemblyline.ApplicationServiceEndpointGET, "/fixture-002",
				assemblyline.ApplicationServiceEndpointMediaNone,
				assemblyline.ApplicationServiceEndpointJSON, 200,
			),
		},
	)
	blueprint, staticFiles, err := compileGenericLaravelServiceBlueprint(
		fixture.Package, specification, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage, state, endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	return directCodingProgram{
		StackID: laravelHTTPServiceAdapter, VersionProfileID: laravelVersionProfileV1,
		Workload: workload, TargetTree: target, Coverage: coverage,
		ServiceState: state, ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
		Generated: laravelLifecycleGeneratedSource(fixture),
	}
}

func laravelLifecycleGeneratedSource(fixture laravelLifecycleFixture) map[string]string {
	_ = fixture
	return map[string]string{
		"feature.001": `function feature101(TaskInput $input, array $dependencies): TaskResult {
	$value = $input->payload['state_001'] ?? $input->payload['fixture'] ?? 'value';
	$state = ['state_001' => (string) $value];
	FeatureState101::save($state);
	return TaskResult::success('Stored', $state);
}`,
		"feature.002": `function feature202(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success('Current', FeatureState202::load());
}`,
		"acceptance.001": `function verifyFeature101(): void {
	$result = feature101(taskInputFixture101(), []);
	RuntimeAssertions::requireResult($result);
	RuntimeAssertions::require($result, $result->error === '', 'expected successful write');
	RuntimeAssertions::require($result, $result->state === ['state_001' => 'value'], 'expected stored fixture');
}`,
		"acceptance.002": `function verifyFeature202(): void {
    $result = feature202(taskInputFixture202(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->error === '', 'expected successful read');
    RuntimeAssertions::require($result, $result->state === [], 'expected isolated initial state');
}`,
	}
}
