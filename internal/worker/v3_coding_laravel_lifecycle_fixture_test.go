package worker

import (
	"fmt"
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
			RootField:        "inspections", ValueField: "reference", DetailField: "status",
			WriteRoute: "/equipment/inspections", ReadRoute: "/equipment/current",
			WriteMethod: assemblyline.ApplicationServiceEndpointPOST,
		},
		{
			Package: "clinic-schedule", Product: "Clinic schedule registry",
			WriteRequirement: "Retain an accepted clinic schedule between requests.",
			ReadRequirement:  "Present the current clinic schedule.",
			RootField:        "schedule", ValueField: "slot", DetailField: "provider",
			WriteRoute: "/clinic/schedule", ReadRoute: "/clinic/current",
			WriteMethod: assemblyline.ApplicationServiceEndpointPUT,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Package, func(t *testing.T) {
			program := laravelLifecycleFixtureProgram(t, fixture)
			files := validateLaravelFixtureAssembly(t, program)
			verifier := files["tests/HttpVerifier.php"]
			for _, required := range []string{
				"verificationLifecycleRequest(", "verifyLifecycleSentinel(",
				phpSingleQuoted(fixture.WriteRoute), phpSingleQuoted(fixture.ReadRoute),
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
	Package, Product                   string
	WriteRequirement, ReadRequirement  string
	RootField, ValueField, DetailField string
	WriteRoute, ReadRoute              string
	WriteMethod                        assemblyline.ApplicationServiceEndpointMethod
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
	workload, err := assemblyline.FreezeApplicationWorkload(
		applicationWorkloadInput(specification), assemblyline.ApplicationWorkloadDraft{
			Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
			Tasks: []assemblyline.ApplicationWorkloadTaskDraft{
				{
					RequirementID: "requirement_001", Objective: "Persist one accepted record.",
					RequiredBehaviors: []string{"Store the accepted record in durable state."},
					AcceptanceCriteria: []string{
						"A valid record produces a successful result.",
						"The accepted record is present in the result state.",
					},
				},
				{
					RequirementID: "requirement_002", Objective: "Return current durable records.",
					RequiredBehaviors: []string{"Read the shared durable state."},
					AcceptanceCriteria: []string{
						"The current state produces a successful result.",
						"The returned state is observable.",
					},
				},
			},
		},
	)
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
	state = bindTestServiceStateInterfaces(t, workload, capabilities, state,
		[]assemblyline.ApplicationServiceStateField{{
			Name: fixture.RootField, Kind: assemblyline.ApplicationServiceStateRecordList,
			RecordFields: []assemblyline.ApplicationServiceStateRecordField{{
				Name: fixture.ValueField, Kind: assemblyline.ApplicationServiceStateString,
			}, {
				Name: fixture.DetailField, Kind: assemblyline.ApplicationServiceStateString,
			}},
		}},
	)
	endpoints := laravelLifecycleEndpoints(workload, fixture)
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

func laravelLifecycleEndpoints(
	workload assemblyline.FrozenApplicationWorkload,
	fixture laravelLifecycleFixture,
) directCodingServiceEndpointPlan {
	contract := func(method assemblyline.ApplicationServiceEndpointMethod, route string,
		request assemblyline.ApplicationServiceEndpointMedia, status int,
	) assemblyline.ApplicationServiceEndpointContract {
		return assemblyline.ApplicationServiceEndpointContract{
			Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
			Exposure: assemblyline.ApplicationServiceEndpointPublic, Method: method,
			RouteTemplate: route, RequestMedia: request,
			ResponseMedia: assemblyline.ApplicationServiceEndpointJSON, SuccessStatus: status,
		}
	}
	return directCodingServiceEndpointPlan{
		WorkloadSHA256: workload.SHA256, ProductContext: fixture.Product,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
			workload.Tasks[1].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[0].ID: contract(fixture.WriteMethod, fixture.WriteRoute, assemblyline.ApplicationServiceEndpointJSON, 201),
			workload.Tasks[1].ID: contract(assemblyline.ApplicationServiceEndpointGET, fixture.ReadRoute, assemblyline.ApplicationServiceEndpointMediaNone, 200),
		},
	}
}

func laravelLifecycleGeneratedSource(fixture laravelLifecycleFixture) map[string]string {
	root := phpSingleQuoted(fixture.RootField)
	value, detail := phpSingleQuoted(fixture.ValueField), phpSingleQuoted(fixture.DetailField)
	return map[string]string{
		"feature.001": fmt.Sprintf(`function feature101(TaskInput $input, array $dependencies): TaskResult {
    $state = is_array($input->payload) && array_key_exists(%s, $input->payload)
        ? $input->payload : [%s => [[%s => $input->payload['fixture'] ?? 'value', %s => 'accepted']]];
    FeatureState101::save($state);
    return TaskResult::success('Stored', $state);
}`, root, root, value, detail),
		"feature.002": `function feature202(TaskInput $input, array $dependencies): TaskResult {
    return TaskResult::success('Current', FeatureState202::load());
}`,
		"acceptance.001": fmt.Sprintf(`function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->error === '', 'expected successful write');
    RuntimeAssertions::require($result, $result->state === [%s => [[%s => 'value', %s => 'accepted']]], 'expected stored fixture');
}`, root, value, detail),
		"acceptance.002": `function verifyFeature202(): void {
    $result = feature202(taskInputFixture202(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->error === '', 'expected successful read');
    RuntimeAssertions::require($result, $result->state === [], 'expected isolated initial state');
}`,
	}
}
