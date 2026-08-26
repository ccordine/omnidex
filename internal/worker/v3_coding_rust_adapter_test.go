package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRustCommandLineStackCompilesAndExecutesLockedOffline(t *testing.T) {
	specification, workload := rustCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericRustCommandLineAdapter, VersionProfileID: rustCommandLineVersionProfileV1,
		Paths: []string{"src/echo.rs", "tests/echo_test.rs"},
	}
	stack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target,
		map[string][]string{workload.Tasks[0].ID: append([]string(nil), target.Paths...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(
		"rust-fixture", specification, nil, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated = map[string]string{
		"feature.001": `pub fn feature_001(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult {
    let _ = dependencies;
    if input.arguments.is_empty() {
        return TaskResult { error: "one argument is required".to_string(), exit_code: 2, ..TaskResult::default() };
    }
    let mut result = TaskResult::default();
    result.output = input.arguments[0].clone();
    result.state.insert("value".to_string(), input.arguments[0].clone());
    result
}`,
		"acceptance.001": `fn verify_feature_001() {
    let input = TaskInput { arguments: vec!["ready".to_string()], standard_input: String::new() };
	let result = feature_001(&input, &representative_capability_results_for_feature_001());
    assert_eq!(result.output, "ready");
}`,
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
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
	for _, args := range [][]string{
		{"test", "--locked", "--offline", "--test", "echo_test"},
		{"check", "--locked", "--offline", "--all-targets"},
		{"build", "--locked", "--offline"},
	} {
		output, err := runDirectCodingStageCommand(
			context.Background(), root, directCodingRustStageTimeout, "cargo", args...,
		)
		if err != nil {
			t.Fatalf("cargo %v failed: %v\n%s", args, err, output)
		}
	}
	contexts, err := directCodingApplicationTaskContexts(applicationWorkloadInput(specification), workload)
	if err != nil {
		t.Fatal(err)
	}
	focusedProgram, err := projectDirectCodingApplicationTaskStage(
		program, contexts["requirement_001"],
	)
	if err != nil {
		t.Fatal(err)
	}
	focusedAssembly, err := directCodingAssemblyFromProgram(focusedProgram)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range focusedAssembly.Files {
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, []byte(file.Content)); err != nil {
			t.Fatal(err)
		}
	}
	focusedRoot := t.TempDir()
	for _, file := range focusedAssembly.Files {
		targetPath := filepath.Join(focusedRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := runDirectCodingStageCommand(
		context.Background(), focusedRoot, directCodingRustStageTimeout,
		"cargo", "test", "--locked", "--offline", "--test", "echo_test",
	)
	if err != nil {
		t.Fatalf("focused cargo test failed: %v\n%s", err, output)
	}
}

func rustCommandLineStackFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "argument echo command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Print the first supplied argument",
		}},
	}
	input := applicationWorkloadInput(specification)
	workload, err := assemblyline.FreezeApplicationWorkload(input, assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{{
			RequirementID: "requirement_001", Objective: "Return the first command argument.",
			RequiredBehaviors:  []string{"Accept one command argument and expose it as output."},
			AcceptanceCriteria: []string{"The first command argument is returned unchanged."},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return specification, workload
}
