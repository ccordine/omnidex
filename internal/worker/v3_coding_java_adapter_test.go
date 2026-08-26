package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaCommandLineBlueprintCompilesRunsAndArchivesWithTheJDK(t *testing.T) {
	specification, workload := javaCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericJavaCommandLineAdapter, VersionProfileID: javaCommandLineVersionProfileV1,
		Paths: []string{"Echo.java", "EchoTest.java"},
	}
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"Echo.java": {workload.Tasks[0].ID}, "EchoTest.java": {workload.Tasks[0].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"Echo.java":     assemblyline.TargetArtifactImplementation,
			"EchoTest.java": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, staticFiles, err := compileGenericJavaCommandLineBlueprint(
		"java-fixture", specification, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{
		"feature.001": `static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies) {
  Object values = input.get("arguments");
  if (!(values instanceof List<?> arguments) || arguments.isEmpty()) {
    return Runtime.result("", "one argument is required", 2, Map.of());
  }
  String value = String.valueOf(arguments.get(0));
  return Runtime.result(value, "", 0, Map.<String, Object>of("value", value));
}`,
		"acceptance.001": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(
      Map.<String, Object>of("arguments", List.of("ready"), "standardInput", ""), Map.of());
  Runtime.requireResult(result);
  Runtime.require("ready".equals(result.get("output")), "expected the first argument");
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
		composed, composeErr := assemblyline.ComposeJavaDocument(document, assemblyline.SourceComposition{
			Generated: generated, Interfaces: interfaces,
		})
		if composeErr != nil {
			t.Fatalf("compose %s: %v", document.ID, composeErr)
		}
		files = append(files, directCodingFileTask{Path: composed.Path, Content: composed.Source})
	}
	assembly := directCodingAssembly{VersionProfileID: javaCommandLineVersionProfileV1, Files: files}
	if err := assembly.normalize(); err != nil {
		t.Fatal(err)
	}
	if err := validateJavaCommandLineAssembly(assembly); err != nil {
		t.Fatal(err)
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
	program := directCodingProgram{
		StackID: genericJavaCommandLineAdapter, VersionProfileID: javaCommandLineVersionProfileV1,
		Workload: workload,
		Source:   blueprint, StaticFiles: staticFiles, Generated: generated,
	}
	commands, err := javaCommandLineVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	want := []testCommand{
		{Family: "java", Name: "javac", Purpose: verificationBuild, Args: []string{
			"--release", "21", "-Xlint:all", "-Werror", "-d", "build/classes",
			"Echo.java", "EchoTest.java", "Main.java", "Runtime.java", "TestRunner.java",
		}},
		{Family: "java", Name: "java", Purpose: verificationTest, Args: []string{
			"-ea", "-cp", "build/classes", "TestRunner", "FeatureTest001", "verifyFeature001",
		}},
		{Family: "java", Name: "jar", Purpose: verificationBuild, Args: []string{
			"--create", "--file", "build/application.jar", "--main-class", "Main",
			"-C", "build/classes", ".",
		}},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("Java verification commands\n got: %#v\nwant: %#v", commands, want)
	}
	for _, command := range commands {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			t.Fatalf("Java command escaped the exact command boundary: %v", err)
		}
		output, commandErr := runDirectCodingStageCommand(
			context.Background(), root, directCodingJavaStageTimeout,
			command.Name, command.Args...,
		)
		if commandErr != nil {
			t.Fatalf("%s failed: %v\n%s", directCodingCommandLabel(command), commandErr, output)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "build", "application.jar")); err != nil {
		t.Fatalf("Java archive was not created: %v", err)
	}
}

func TestJavaCommandLineCoverageRejectsSharedLeaves(t *testing.T) {
	_, workload := javaTwoTaskWorkloadFixture(t)
	target := assemblyline.TargetTree{StackID: genericJavaCommandLineAdapter, Paths: []string{
		"First.java", "FirstTest.java", "Second.java", "SecondTest.java",
	}}
	kinds := make(map[string]assemblyline.TargetArtifactKind, len(target.Paths))
	for _, sourcePath := range target.Paths {
		kind := assemblyline.TargetArtifactImplementation
		if strings.HasSuffix(sourcePath, "Test.java") {
			kind = assemblyline.TargetArtifactVerification
		}
		kinds[sourcePath] = kind
	}
	plan, err := assemblyline.NewApplicationFileCoveragePlan(workload, target, map[string][]string{
		"First.java": {"task_001", "task_002"}, "FirstTest.java": {"task_001"},
		"Second.java": {"task_002"}, "SecondTest.java": {"task_002"},
	}, kinds)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJavaCommandLineCoverage(target, workload, plan); err == nil ||
		!strings.Contains(err.Error(), "exactly one task owner") {
		t.Fatalf("shared Java coverage error=%v", err)
	}
}

func TestJavaFinalVerificationRunsEachVerifierIndependently(t *testing.T) {
	_, workload := javaTwoTaskWorkloadFixture(t)
	program := directCodingProgram{
		StackID: genericJavaCommandLineAdapter, VersionProfileID: javaCommandLineVersionProfileV1,
		Workload: workload,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{Path: "First.java"}, {Path: "FirstTest.java"}, {Path: "Main.java"},
			{Path: "Runtime.java"}, {Path: "Second.java"}, {Path: "SecondTest.java"},
		}},
		StaticFiles: genericJavaCommandLineStaticFiles(),
	}
	commands, err := javaCommandLineVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("Java final command count=%d want 4", len(commands))
	}
	wantTargets := [][]string{
		{"-ea", "-cp", "build/classes", "TestRunner", "FeatureTest001", "verifyFeature001"},
		{"-ea", "-cp", "build/classes", "TestRunner", "FeatureTest002", "verifyFeature002"},
	}
	for index, want := range wantTargets {
		if !reflect.DeepEqual(commands[index+1].Args, want) {
			t.Fatalf("Java verifier %d args=%v want %v", index, commands[index+1].Args, want)
		}
	}
}

func javaCommandLineStackFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "argument echo command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Print the first supplied argument",
		}},
	}
	return specification, freezeJavaWorkload(t, specification, []assemblyline.ApplicationWorkloadTaskDraft{{
		RequirementID: "requirement_001", Objective: "Return the first command argument.",
		RequiredBehaviors:  []string{"Accept one command argument and expose it as output."},
		AcceptanceCriteria: []string{"The first command argument is returned unchanged."},
	}})
}

func javaTwoTaskWorkloadFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "two operation command",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Perform the first operation"},
			{ID: "requirement_002", SourceQuote: "Perform the second operation"},
		},
	}
	return specification, freezeJavaWorkload(t, specification, []assemblyline.ApplicationWorkloadTaskDraft{
		{RequirementID: "requirement_001", Objective: "Perform the first operation.", RequiredBehaviors: []string{"Return the first result."}, AcceptanceCriteria: []string{"The first result is present."}},
		{RequirementID: "requirement_002", Objective: "Perform the second operation.", RequiredBehaviors: []string{"Return the second result."}, AcceptanceCriteria: []string{"The second result is present."}},
	})
}

func freezeJavaWorkload(
	t *testing.T,
	specification assemblyline.ApplicationSpecification,
	tasks []assemblyline.ApplicationWorkloadTaskDraft,
) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	if err := specification.Validate(); err != nil {
		t.Fatal(err)
	}
	workload, err := assemblyline.FreezeApplicationWorkload(
		applicationWorkloadInput(specification),
		assemblyline.ApplicationWorkloadDraft{Schema: assemblyline.ApplicationWorkloadDraftSchemaV1, Tasks: tasks},
	)
	if err != nil {
		t.Fatal(err)
	}
	return workload
}
