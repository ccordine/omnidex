package worker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestCommandLineStacksProjectExactFocusedTreesMechanically(t *testing.T) {
	t.Parallel()
	tests := []struct {
		stackID string
		want    []string
	}{
		{genericGoCommandLineAdapter, []string{"feature001.go", "feature001_test.go"}},
		{genericJavaScriptCommandLineAdapter, []string{"feature001.mjs", "feature001.test.mjs"}},
		{genericRustCommandLineAdapter, []string{"src/feature001.rs", "tests/feature001_test.rs"}},
		{genericJavaCommandLineAdapter, []string{"Feature001.java", "Feature001Test.java"}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.stackID, func(t *testing.T) {
			t.Parallel()
			stack, err := directCodingProjectStackByID(testCase.stackID)
			if err != nil {
				t.Fatal(err)
			}
			if stack.ProjectFocusedTargetTree == nil {
				t.Fatal("exact command-line grammar retained an inference-only target-tree path")
			}
			target, err := stack.ProjectFocusedTargetTree(1, directCodingTargetTreeOccupation{
				FilePaths: stack.TargetTreeReservedPaths,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDirectCodingFocusedTargetTree(stack, target); err != nil {
				t.Fatalf("projected target is invalid: %v", err)
			}
			if !reflect.DeepEqual(target.Paths, testCase.want) {
				t.Fatalf("paths=%v want=%v", target.Paths, testCase.want)
			}
		})
	}
}

func TestMechanicalCompleteTreeAllocatorSkipsOccupiedPairsAcrossArtifactGrammars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		occupied []string
		pair     mechanicalTargetTreePair
		want     []string
	}{
		{
			name: "root ECMAScript modules",
			occupied: []string{
				"feature001.mjs", "feature002.test.mjs",
			},
			pair: func(ordinal int) (string, string) {
				return fmt.Sprintf("feature%03d.mjs", ordinal),
					fmt.Sprintf("feature%03d.test.mjs", ordinal)
			},
			want: []string{"feature003.mjs", "feature003.test.mjs"},
		},
		{
			name: "split Rust package directories",
			occupied: []string{
				"src/feature001.rs", "tests/feature002_test.rs",
			},
			pair: func(ordinal int) (string, string) {
				return fmt.Sprintf("src/feature%03d.rs", ordinal),
					fmt.Sprintf("tests/feature%03d_test.rs", ordinal)
			},
			want: []string{"src/feature003.rs", "tests/feature003_test.rs"},
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			target, err := projectMechanicalCompleteTargetTree(
				testCase.name,
				directCodingTargetTreeOccupation{FilePaths: testCase.occupied},
				testCase.pair,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(target.Paths, testCase.want) {
				t.Fatalf("paths=%v want=%v", target.Paths, testCase.want)
			}
		})
	}
}

func TestCommandLineTargetTreeResolutionPerformsNoInference(t *testing.T) {
	t.Parallel()
	for _, stackID := range []string{
		genericGoCommandLineAdapter,
		genericJavaScriptCommandLineAdapter,
		genericRustCommandLineAdapter,
		genericJavaCommandLineAdapter,
	} {
		stackID := stackID
		t.Run(stackID, func(t *testing.T) {
			t.Parallel()
			stack, err := directCodingProjectStackByID(stackID)
			if err != nil {
				t.Fatal(err)
			}
			specification, workload := testApplicationFileCoverageAuthority(
				t, assemblyline.ApplicationSurfaceCommandLine,
				"command-line transformer", "transform one input value",
			)
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: exactSemanticLeafCalls,
				Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
					calls++
					return assemblyline.PortableResult{}, fmt.Errorf("forbidden target-tree inference")
				},
			}
			target, coverage, err := resolveDirectCodingTargetTree(
				runtime, "", "", specification, workload, stack, nil, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatalf("target-tree inference calls=%d want=0", calls)
			}
			if err := coverage.ValidateFor(target, workload); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMechanicalFocusedTreeSkipsAnyPartiallyOccupiedPair(t *testing.T) {
	t.Parallel()
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if stack.ProjectFocusedTargetTree == nil {
		t.Fatal("Go stack lacks its mechanical focused-tree projector")
	}
	target, err := stack.ProjectFocusedTargetTree(1, directCodingTargetTreeOccupation{
		FilePaths: []string{"feature001.go", "feature002_test.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature003.go", "feature003_test.go"}
	if !reflect.DeepEqual(target.Paths, want) {
		t.Fatalf("paths=%v want=%v", target.Paths, want)
	}
}

func TestMechanicalTargetTreeResolutionSkipsExistingDirectoryLeaf(t *testing.T) {
	t.Parallel()
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"command-line transformer", "transform one input value",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{}, fmt.Errorf("forbidden target-tree inference")
		},
	}
	target, coverage, err := resolveDirectCodingTargetTree(
		runtime, "", "", specification, workload, stack, nil,
		[]string{"feature001.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature002.go", "feature002_test.go"}
	if calls != 0 || !reflect.DeepEqual(target.Paths, want) {
		t.Fatalf("calls=%d paths=%v want=%v", calls, target.Paths, want)
	}
	if err := coverage.ValidateFor(target, workload); err != nil {
		t.Fatal(err)
	}
}

func TestMechanicalMultiTaskCommandLineTreeCompilesItsUnion(t *testing.T) {
	t.Parallel()
	for _, stackID := range []string{
		genericGoCommandLineAdapter,
		genericJavaScriptCommandLineAdapter,
	} {
		stackID := stackID
		t.Run(stackID, func(t *testing.T) {
			t.Parallel()
			stack, err := directCodingProjectStackByID(stackID)
			if err != nil {
				t.Fatal(err)
			}
			specification, workload := testApplicationFileCoverageAuthority(
				t, assemblyline.ApplicationSurfaceCommandLine,
				"command-line transformer",
				"transform the first input value",
				"transform the second input value",
			)
			target, coverage, err := resolveDirectCodingTargetTree(
				typedWorkerRuntime{Context: context.Background(), MaxAttempts: 1},
				"", "", specification, workload, stack, nil, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			target.StackID = stack.ID
			target.VersionProfileID = stack.DefaultVersionProfileID
			capabilities := make(directCodingCapabilityGraph, len(specification.Requirements))
			for _, requirement := range specification.Requirements {
				capabilities[requirement.ID] = nil
			}
			program, err := compileDirectCodingProgram(
				"multi-task-command", specification, nil,
				map[string]directCodingSkillBinding{}, workload,
				capabilities, target, coverage,
			)
			if err != nil {
				t.Fatalf("compile four-path target union: %v", err)
			}
			if len(target.Paths) != 4 || len(program.TargetTree.Paths) != 4 {
				t.Fatalf("target paths=%v program paths=%v", target.Paths, program.TargetTree.Paths)
			}
		})
	}
}
