package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoCommandLineStackCompilesComposesAndExecutesFocusedTests(t *testing.T) {
	specification, workload := goCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericGoCommandLineAdapter, VersionProfileID: goCommandLineVersionProfileV1,
		Paths: []string{"feature.go", "feature_test.go"},
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
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
		"go-fixture", specification, nil, map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated["feature.001"] = `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	if len(input.Arguments) == 0 {
		return TaskResult{Error: "one argument is required", ExitCode: 2}
	}
	return TaskResult{Output: input.Arguments[0], State: map[string]string{"value": input.Arguments[0]}}
}`
	program.Generated["acceptance.001"] = `func TestFeature001(t *testing.T) {
	result := Feature001(TaskInput{Arguments: []string{"ready"}}, CapabilityResults{})
	if result.Output != "ready" {
		t.Fatalf("output = %q", result.Output)
	}
}`
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingGoStageTimeout,
		"go", "test", "-count=1", "./...",
	)
	if err != nil {
		t.Fatalf("go test failed: %v\n%s", err, output)
	}
	if program.StackID != genericGoCommandLineAdapter || target.StackID != genericGoCommandLineAdapter {
		t.Fatalf("stack authority program/tree=%q/%q", program.StackID, target.StackID)
	}
}

func TestGoCommandLineTargetTreeRejectsNestedReservedOrUnpairedLeaves(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	for name, paths := range map[string][]string{
		"nested":   {"cmd/feature.go", "cmd/feature_test.go"},
		"reserved": {"main.go", "feature_test.go"},
		"unpaired": {"first.go", "second.go"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingFocusedTargetTree(stack, assemblyline.TargetTree{Paths: paths})
			if err == nil {
				t.Fatalf("accepted invalid Go target paths %v", paths)
			}
		})
	}
	if err := validateDirectCodingFocusedTargetTree(
		stack, assemblyline.TargetTree{Paths: []string{"feature.go", "feature_test.go"}},
	); err != nil {
		t.Fatal(err)
	}
}

func TestGoAcceptanceRequiresImplementationCallAndTestingAssertion(t *testing.T) {
	_, workload := goCommandLineStackFixture(t)
	stage := directCodingProgram{
		StackID: genericGoCommandLineAdapter, Workload: workload,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{ID: "feature", Path: "feature.go", AdapterID: "go", Preamble: "package main", Blocks: []assemblyline.SourceBlock{{
				ID: "feature.001", Signature: "func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult",
				Contract: "Return a result.", API: "func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult",
				TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
			}}},
			{ID: "acceptance", Path: "feature_test.go", AdapterID: "go", Preamble: "package main\nimport \"testing\"", Blocks: []assemblyline.SourceBlock{{
				ID: "acceptance.001", Signature: "func TestFeature001(t *testing.T)", Contract: "Verify.",
				API: "func TestFeature001(t *testing.T)", DependsOn: []string{"feature.001"},
				TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
			}}},
		}},
	}
	ref := assemblyline.SourceBlockRef{
		Document: stage.Source.Documents[1], Block: stage.Source.Documents[1].Blocks[0],
	}
	for name, source := range map[string]string{
		"no call":      `func TestFeature001(t *testing.T) { t.Fatal("missing") }`,
		"no assertion": `func TestFeature001(t *testing.T) { Feature001(TaskInput{}, CapabilityResults{}) }`,
		"detached failure": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			_ = result
			t.Fatal("detached")
		}`,
		"boolean shortcut": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output != "ready" || true { t.Fatal("tautology") }
		}`,
		"self comparison": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if result.Output == result.Output { t.Fatal("tautology") }
		}`,
		"unreachable nested proof": `func TestFeature001(t *testing.T) {
			result := Feature001(TaskInput{}, CapabilityResults{})
			if false { if result.Output != "ready" { t.Fatal("unreachable") } }
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDirectCodingGoAcceptance(&stage, ref, source); err == nil {
				t.Fatalf("accepted invalid verification source %s", source)
			}
		})
	}
}

func goCommandLineStackFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "argument echo command",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Print the first supplied argument",
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	if err := specification.Validate(); err != nil {
		t.Fatal(err)
	}
	return specification, workload
}

func TestProjectStackSelectionCanSelectGoWithoutExposingItsRegistryID(t *testing.T) {
	specification, _ := goCommandLineStackFixture(t)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			if strings.Contains(prompt, genericGoCommandLineAdapter) || !strings.Contains(prompt, "STACK_CANDIDATE_1") {
				t.Fatalf("stack constraint leaked or omitted authority: %s", prompt)
			}
			return "STACK_CANDIDATE_1", nil
		}),
	}
	selection, err := selectDirectCodingProject(
		runtime, func() (string, error) { return "constraint", nil }, specification, nil, nil,
	)
	if err != nil || selection.Stack.ID != genericGoCommandLineAdapter ||
		selection.VersionProfileID != goCommandLineVersionProfileV1 {
		t.Fatalf("selection=%+v error=%v", selection, err)
	}
}
