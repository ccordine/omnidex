package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTTPServiceCompleteDockerFixture(t *testing.T) {
	if os.Getenv("OMNI_RUN_PHP_DOCKER_TEST") != "1" {
		t.Skip("set OMNI_RUN_PHP_DOCKER_TEST=1 to run the external Docker/PHP/NGINX/Tailwind fixture")
	}
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
    return TaskResult::success($input->route, ['route' => $input->route]);
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
	assembly := directCodingAssembly{
		VersionProfileID: phpServiceVersionProfileV1,
		Files:            files,
	}
	if err := assembly.normalize(); err != nil {
		t.Fatal(err)
	}
	if err := validateGenericPHPServiceAssembly(assembly); err != nil {
		t.Fatal(err)
	}
	for _, file := range assembly.Files {
		adapter, _, adapterErr := directCodingArtifactAdapterForPath(file.Path)
		if adapterErr != nil {
			t.Fatalf("select %s adapter: %v", file.Path, adapterErr)
		}
		if validateErr := validateDirectCodingArtifactSource(adapter, file.Path, []byte(file.Content)); validateErr != nil {
			t.Fatalf("validate %s: %v", file.Path, validateErr)
		}
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		targetPath := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cleanup := phpServiceCleanupCommands()[0]
	defer func() {
		output, cleanupErr := runDirectCodingStageCommand(
			context.Background(), root, cleanup.Timeout, cleanup.Name, cleanup.Args...,
		)
		if cleanupErr != nil {
			t.Errorf("exact PHP Docker cleanup failed: %v\n%s", cleanupErr, output)
		}
	}()
	program := directCodingProgram{
		StackID: genericPHPServiceAdapter, VersionProfileID: phpServiceVersionProfileV1,
		Workload: workload, Coverage: coverage,
		ServiceState:     testRequestLocalServiceStatePlan(workload),
		ServiceEndpoints: endpoints, Source: blueprint, StaticFiles: staticFiles,
		Generated: generated,
	}
	commands, err := phpServiceVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		timeout := command.Timeout
		if timeout == 0 {
			timeout = directCodingPHPStageTimeout
		}
		output, commandErr := runDirectCodingStageCommand(
			context.Background(), root, timeout, command.Name, command.Args...,
		)
		if commandErr != nil {
			t.Fatalf("PHP Docker command %v failed: %v\n%s", command.Args, commandErr, output)
		}
	}
}
