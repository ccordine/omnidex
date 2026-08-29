package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTTPServiceBlueprintComposesAParserValidatedServerRenderedProject(t *testing.T) {
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	blueprint, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{
		"feature.001": `function feature101(TaskInput $input, array $dependencies): TaskResult {
    $dependencyCount = count($dependencies);
    return TaskResult::success($input->route, ['dependencies' => $dependencyCount]);
}`,
		"representation.html.001": phpServiceHTMLRendererFixture(),
		"acceptance.001": `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === '/', 'expected endpoint route output');
    RuntimeAssertions::require($result, $result->error === '', 'expected successful endpoint result');
}`,
	}
	interfaces := make(map[string]string)
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			interfaces[block.ID] = block.API
		}
	}
	files := append([]directCodingFileTask(nil), staticFiles...)
	for _, document := range blueprint.Documents {
		composed, composeErr := assemblyline.ComposePHPDocument(
			document,
			assemblyline.SourceComposition{Generated: generated, Interfaces: interfaces},
		)
		if composeErr != nil {
			t.Fatalf("compose %s: %v", document.ID, composeErr)
		}
		files = append(files, directCodingFileTask{Path: composed.Path, Content: composed.Source})
	}
	assembly := directCodingAssembly{VersionProfileID: phpServiceVersionProfileV1, Files: files}
	if err := assembly.normalize(); err != nil {
		t.Fatal(err)
	}
	if err := validateGenericPHPServiceAssembly(assembly); err != nil {
		t.Fatal(err)
	}
	for _, file := range assembly.Files {
		adapter, _, err := directCodingArtifactAdapterForPath(file.Path)
		if err != nil {
			t.Fatalf("select %s adapter: %v", file.Path, err)
		}
		if err := validateDirectCodingArtifactSource(adapter, file.Path, []byte(file.Content)); err != nil {
			t.Fatalf("validate %s: %v", file.Path, err)
		}
	}
	public := phpServiceFileContent(t, assembly.Files, "public/index.php")
	implementation := phpServiceFileContent(t, assembly.Files, "src/Feature101.php")
	if !strings.Contains(public, "RuntimeHttp::isStaticFileRequest") ||
		!strings.Contains(public, "return false") ||
		!strings.Contains(public, "renderFeature101HTML($result101)") ||
		!strings.Contains(implementation, "RuntimeHtml::document") ||
		strings.Contains(strings.ToLower(implementation), "<script") {
		t.Fatal("PHP HTTP project does not provide server-rendered HTML and CLI-server asset pass-through")
	}
	if !phpServiceHasHTMLResponse(endpoints) {
		t.Fatal("fixture lost its HTML response contract")
	}
}

func phpServiceHTMLRendererFixture() string {
	return `function renderFeature101HTML(TaskResult $result): string {
    return RuntimeHtml::document(
        'Requested record',
        '<main class="min-h-screen px-4 py-8 md:px-8"><h1 class="text-2xl font-semibold">Requested record</h1><p class="mt-4 whitespace-pre-wrap">' .
            RuntimeHtml::escape($result->output) .
            '</p></main>',
    );
}`
}

func TestPHPHTTPServiceCommandsAreExactAndCarryDockerLifetimes(t *testing.T) {
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	blueprint, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
		testRequestLocalServiceStatePlan(workload), endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Workload: workload, Coverage: coverage,
		ServiceState:     testRequestLocalServiceStatePlan(workload),
		ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
	}
	commands, err := phpServiceVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) < 8 || commands[0].Args[1] != "config" || commands[1].Args[1] != "build" ||
		commands[1].Timeout != 5*time.Minute {
		t.Fatalf("PHP HTTP verification command set=%+v", commands)
	}
	for _, command := range append(commands, phpServiceCleanupCommands()...) {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("command %v escaped exact Docker boundary: %v", command.Args, err)
		}
	}
	cleanup := phpServiceCleanupCommands()[0]
	if strings.Join(cleanup.Args, " ") !=
		"compose down --rmi local --volumes --remove-orphans" || cleanup.Timeout != directCodingPHPStageTimeout {
		t.Fatalf("PHP HTTP cleanup=%+v", cleanup)
	}
}

func TestPHPHTTPServiceTargetAndCoverageRejectSharedOrMismatchedLeaves(t *testing.T) {
	valid := assemblyline.TargetTree{Paths: []string{
		"src/Feature901.php", "tests/Feature901Test.php",
	}}
	if err := validateGenericPHPServiceTargetTree(valid); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{
		{"src/Feature901.php", "tests/Feature902Test.php"},
		{"Feature901.php", "tests/Feature901Test.php"},
		{"src/feature901.php", "tests/Feature901Test.php"},
	} {
		if err := validateGenericPHPServiceTargetTree(assemblyline.TargetTree{Paths: paths}); err == nil {
			t.Fatalf("accepted invalid PHP target paths %v", paths)
		}
	}
}

func phpServiceStackFixture(t *testing.T) (
	assemblyline.ApplicationSpecification,
	assemblyline.FrozenApplicationWorkload,
	assemblyline.TargetTree,
	assemblyline.ApplicationFileCoveragePlan,
	directCodingServiceEndpointPlan,
) {
	return phpServiceStackFixtureForSurface(t, assemblyline.ApplicationSurfaceBrowser)
}

func phpServiceStackFixtureForSurface(
	t *testing.T,
	surface assemblyline.ApplicationSurface,
) (
	assemblyline.ApplicationSpecification,
	assemblyline.FrozenApplicationWorkload,
	assemblyline.TargetTree,
	assemblyline.ApplicationFileCoveragePlan,
	directCodingServiceEndpointPlan,
) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      surface,
		ProductQuote: "HTTP service that returns a requested record representation",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Return the requested record representation",
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Paths: []string{"src/Feature101.php", "tests/Feature101Test.php"},
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"src/Feature101.php":       {workload.Tasks[0].ID},
			"tests/Feature101Test.php": {workload.Tasks[0].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"src/Feature101.php":       assemblyline.TargetArtifactImplementation,
			"tests/Feature101Test.php": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	responseMedia := assemblyline.ApplicationServiceEndpointJSON
	if surface == assemblyline.ApplicationSurfaceBrowser {
		responseMedia = assemblyline.ApplicationServiceEndpointHTML
	}
	endpoints := testServiceEndpointPlan(
		t, genericPHPServiceAdapter, workload,
		map[string]assemblyline.ApplicationServiceEndpointRequirement{
			workload.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		map[string]assemblyline.ApplicationServiceEndpointContract{
			workload.Tasks[0].ID: testHTTPServiceEndpointContract(
				assemblyline.ApplicationServiceEndpointGET, "/",
				assemblyline.ApplicationServiceEndpointMediaNone, responseMedia, 200,
			),
		},
	)
	return specification, workload, target, coverage, endpoints
}

func testRequestLocalServiceStatePlan(
	workload assemblyline.FrozenApplicationWorkload,
) directCodingServiceStatePlan {
	byTask := make(map[string]assemblyline.ApplicationServiceStateLifetime, len(workload.Tasks))
	for _, task := range workload.Tasks {
		byTask[task.ID] = assemblyline.ApplicationServiceStateRequestLocalOnly
	}
	return directCodingServiceStatePlan{WorkloadSHA256: workload.SHA256, ByTask: byTask}
}

func phpServiceFileContent(t *testing.T, files []directCodingFileTask, path string) string {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("missing file %s", path)
	return ""
}
