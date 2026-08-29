package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptCompleteTargetTreeIsTaskNeutralAndCompiles(t *testing.T) {
	tests := []struct {
		name         string
		product      string
		requirements []string
	}{
		{
			name:    "unit conversion",
			product: "unit conversion browser",
			requirements: []string{
				"accept a distance expressed in miles",
				"show the equivalent distance in kilometers",
			},
		},
		{
			name:    "reading list",
			product: "reading list browser",
			requirements: []string{
				"display the saved book titles",
				"let a reader add another title",
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			specification, workload := testApplicationFileCoverageAuthority(
				t, assemblyline.ApplicationSurfaceBrowser,
				testCase.product, testCase.requirements...,
			)
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 2,
				Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
					calls++
					return "ROOT\n  D src\n    F App.test.tsx\n    F App.tsx", nil
				}),
			}
			stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
			if err != nil {
				t.Fatal(err)
			}
			tree, coverage, err := resolveDirectCodingTargetTree(
				runtime, "", "", specification, workload, stack,
				[]string{}, []string{},
			)
			if err != nil {
				t.Fatal(err)
			}
			wantPaths := []string{"src/feature001.test.tsx", "src/feature001.tsx"}
			if calls != 0 || !reflect.DeepEqual(tree.Paths, wantPaths) {
				t.Fatalf("calls=%d paths=%v want=%v", calls, tree.Paths, wantPaths)
			}
			wantOwners := make([]string, len(workload.Tasks))
			for index, task := range workload.Tasks {
				wantOwners[index] = task.ID
			}
			for _, artifactPath := range tree.Paths {
				owners, ownerErr := coverage.TasksForPath(artifactPath)
				if ownerErr != nil {
					t.Fatal(ownerErr)
				}
				if !reflect.DeepEqual(owners, wantOwners) {
					t.Fatalf("path %s owners=%v want=%v", artifactPath, owners, wantOwners)
				}
			}
			if err := coverage.ValidateFor(tree, workload); err != nil {
				t.Fatal(err)
			}
			tree.StackID = stack.ID
			tree.VersionProfileID = stack.DefaultVersionProfileID
			capabilities := make(directCodingCapabilityGraph, len(specification.Requirements))
			for _, requirement := range specification.Requirements {
				capabilities[requirement.ID] = nil
			}
			program, err := compileDirectCodingProgram(
				"task-neutral-browser", specification, nil,
				map[string]directCodingSkillBinding{}, workload,
				capabilities, tree, coverage,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(program.Source.Documents) == 0 {
				t.Fatal("compiled TypeScript blueprint has no source documents")
			}
		})
	}
}

func TestTargetTreeCorrectionReplacesTheSameCompleteSafeTree(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a count",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
			calls++
			switch calls {
			case 1:
				return "ROOT\n  D src\n    F counter.css\n  D tests\n    F counter.test.css", nil
			case 2:
				if model != "repair-model" {
					t.Fatalf("correction model=%q", model)
				}
				for _, expected := range []string{
					"CURRENT_SAFE_TREE_CANDIDATE:",
					"F counter.css",
					"VALIDATION_FAILURE:",
					"complete replacement basename hierarchy",
				} {
					if !strings.Contains(prompt, expected) {
						t.Fatalf("correction prompt lacks %q:\n%s", expected, prompt)
					}
				}
				return "ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx", nil
			default:
				t.Fatalf("unexpected target-tree call %d", calls)
				return "", nil
			}
		}),
	}
	stack := inferredTypeScriptTargetTreeFixture(t)
	tree, _, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "repair-model", specification, workload, stack,
		[]string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !reflect.DeepEqual(
		tree.Paths, []string{"src/counter.tsx", "tests/counter.test.tsx"},
	) {
		t.Fatalf("calls=%d tree=%v", calls, tree.Paths)
	}
}

func TestTargetTreeCorrectionNeverExposesConstructedPaths(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a count",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return "ROOT\n  D src\n    F App.tsx\n  D tests\n    F App.test.tsx", nil
			}
			if strings.Contains(prompt, "src/App.tsx") ||
				strings.Contains(prompt, "tests/App.test.tsx") {
				t.Fatalf("constructed path leaked into correction prompt:\n%s", prompt)
			}
			if err := assemblyline.ValidatePathFreeModelContext(
				"captured target-tree correction prompt", prompt,
			); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, "duplicates a basename hierarchy in CODE_RESERVED_TREE") {
				t.Fatalf("correction prompt lacks path-free exact defect:\n%s", prompt)
			}
			return "ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx", nil
		}),
	}
	stack := inferredTypeScriptTargetTreeFixture(t)
	if _, _, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "repair-model", specification, workload, stack,
		[]string{}, []string{},
	); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want=2", calls)
	}
}

func TestTargetTreeCorrectionDoesNotEchoUnsafeCandidate(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a count",
	)
	const unsafe = "/private/workspace/secret.tsx"
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return "ROOT\n  F " + unsafe + "\n  F counter.test.tsx", nil
			}
			if strings.Contains(prompt, unsafe) || strings.Contains(prompt, "CURRENT_SAFE_TREE_CANDIDATE") {
				t.Fatalf("unsafe candidate was echoed into correction:\n%s", prompt)
			}
			if !strings.Contains(prompt, "violates the exact raw target-tree grammar") {
				t.Fatalf("unsafe correction lacks bounded defect:\n%s", prompt)
			}
			return "ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx", nil
		}),
	}
	stack := inferredTypeScriptTargetTreeFixture(t)
	if _, _, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "repair-model", specification, workload, stack,
		[]string{}, []string{},
	); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want=2", calls)
	}
}

func TestInferredTargetTreeValidatesExistingFileClosureBeforeAcceptance(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"annotation browser", "display one annotation",
	)
	calls := 0
	finalizedValid := make([]bool, 0, 2)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return "ROOT\n  D src\n    F feature.test.tsx\n    F feature.tsx", nil
			}
			const exactDefect = "One response node crosses a basename hierarchy already held by an existing workspace file."
			if !strings.Contains(prompt, exactDefect) {
				t.Fatalf("existing-file correction prompt lacks exact defect:\n%s", prompt)
			}
			if strings.Contains(prompt, "src/feature") {
				t.Fatalf("constructed path leaked into existing-file correction prompt:\n%s", prompt)
			}
			return "ROOT\n  D work\n    F feature.test.tsx\n    F feature.tsx", nil
		}),
		Finalize: func(
			_ assemblyline.PortableJob,
			_ assemblyline.PortableResult,
			validationErr error,
		) error {
			finalizedValid = append(finalizedValid, validationErr == nil)
			return nil
		},
	}
	stack := inferredTypeScriptTargetTreeFixture(t)
	target, _, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "repair-model", specification, workload, stack,
		[]string{"src"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !reflect.DeepEqual(finalizedValid, []bool{false, true}) {
		t.Fatalf("calls=%d finalized-valid=%v", calls, finalizedValid)
	}
	want := []string{"work/feature.test.tsx", "work/feature.tsx"}
	if !reflect.DeepEqual(target.Paths, want) {
		t.Fatalf("paths=%v want=%v", target.Paths, want)
	}
}

func inferredTypeScriptTargetTreeFixture(t *testing.T) directCodingProjectStack {
	t.Helper()
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	stack.ProjectCompleteTargetTree = nil
	return stack
}

func TestExclusiveCommandLineTreeProjectsTaskPairsWithoutInference(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"two command behaviors", "return the first value", "return the second value",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{}, fmt.Errorf("forbidden target-tree inference")
		},
	}
	stack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "", "", specification, workload, stack, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"src/feature001.rs", "src/feature002.rs",
		"tests/feature001_test.rs", "tests/feature002_test.rs",
	}
	if calls != 0 || !reflect.DeepEqual(tree.Paths, want) {
		t.Fatalf("calls=%d paths=%v want=%v", calls, tree.Paths, want)
	}
	for index, task := range workload.Tasks {
		files, filesErr := coverage.FilesForTask(task.ID)
		if filesErr != nil {
			t.Fatal(filesErr)
		}
		if len(files) != 2 || !strings.Contains(files[0].Path+files[1].Path, fmt.Sprintf("%03d", index+1)) {
			t.Fatalf("task %s mechanical coverage=%v", task.ID, files)
		}
	}
}

func TestPHPCompleteTreeIsMechanicallyProjectedWithoutInference(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceService,
		"record service", "return the first record", "return the second record",
	)
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		typedWorkerRuntime{Context: context.Background(), MaxAttempts: 1},
		"", "", specification, workload, stack,
		[]string{"composer.json", "src/Feature001.php", "tests/Feature003Test.php"},
		[]string{"src", "tests"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"src/Feature002.php", "src/Feature004.php",
		"tests/Feature002Test.php", "tests/Feature004Test.php",
	}
	if !reflect.DeepEqual(tree.Paths, want) {
		t.Fatalf("paths=%v want=%v", tree.Paths, want)
	}
	if err := coverage.ValidateFor(tree, workload); err != nil {
		t.Fatal(err)
	}
}
