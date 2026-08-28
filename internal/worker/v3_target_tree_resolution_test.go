package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestInferredTargetTreeResolvesOneCompleteWorkloadCall(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application",
		"display a current count",
		"increment the current count",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
			calls++
			if model != "tree-model" {
				t.Fatalf("target-tree call model=%q", model)
			}
			for _, expected := range []string{
				"Product context: counter application",
				"Accepted goal 1:",
				"Accepted behavior: display a current count",
				"Accepted goal 2:",
				"Accepted behavior: increment the current count",
				"CURRENT_MANAGED_WORKLOAD_TREE:\nROOT",
			} {
				if !strings.Contains(prompt, expected) {
					t.Fatalf("complete target-tree prompt lacks %q:\n%s", expected, prompt)
				}
			}
			for _, forbidden := range []string{
				workload.Tasks[0].ID, workload.Tasks[1].ID,
				"task_id", "REUSABLE_ACCEPTED", "_JSON",
			} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("target-tree prompt contains forbidden %q:\n%s", forbidden, prompt)
				}
			}
			return strings.Join([]string{
				"ROOT",
				"  D src",
				"    F counter.tsx",
				"  D tests",
				"    F counter.test.tsx",
			}, "\n"), nil
		}),
	}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "repair-model", specification, workload, stack,
		[]string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"src/counter.tsx", "tests/counter.test.tsx"}
	if calls != 1 || !reflect.DeepEqual(tree.Paths, wantPaths) {
		t.Fatalf("calls=%d paths=%v want=%v", calls, tree.Paths, wantPaths)
	}
	wantOwners := []string{workload.Tasks[0].ID, workload.Tasks[1].ID}
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
					"complete replacement raw tree",
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
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
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
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
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
