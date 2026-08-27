package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeResolutionReturnsPathTreeAndCodeOwnedCoverage(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a count",
	)
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 1, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		if model != "tree-model" || !strings.Contains(prompt, "TypeScript React") {
			t.Fatalf("tree model=%q prompt=%s", model, prompt)
		}
		for _, expected := range []string{
			"Product context: counter application",
			"Accepted behavior: display a count",
			"Structural objective: Deliver display a count.",
		} {
			if !strings.Contains(prompt, expected) {
				t.Fatalf("target-tree prompt missing %q: %s", expected, prompt)
			}
		}
		for _, forbidden := range []string{workload.Tasks[0].ID, `"task_id"`, "ownership", "provenance"} {
			if strings.Contains(prompt, forbidden) {
				t.Fatalf("target-tree prompt contains code-owned %q: %s", forbidden, prompt)
			}
		}
		return `{"schema":"omnidex.target-tree.v1","paths":["src/counter.tsx","tests/counter.test.tsx"]}`, nil
	})}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "review-model", specification, workload, stack, []string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tree.Paths, ",") != "src/counter.tsx,tests/counter.test.tsx" {
		t.Fatalf("paths=%v", tree.Paths)
	}
	if err := coverage.ValidateFor(tree, workload); err != nil {
		t.Fatal(err)
	}
	for _, path := range tree.Paths {
		owners, err := coverage.TasksForPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(owners, []string{workload.Tasks[0].ID}) {
			t.Fatalf("path %s owners=%v", path, owners)
		}
	}
}

func TestTargetTreeResolutionUnionsPluralLeavesAndRetainsSharedProvenance(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a current count", "increment the current count",
	)
	calls := 0
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 1, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		calls++
		if model != "tree-model" {
			t.Fatalf("tree model=%q", model)
		}
		switch {
		case strings.Contains(prompt, "Accepted behavior: display a current count"):
			return `{"schema":"omnidex.target-tree.v1","paths":["src/shared.tsx","tests/display.test.tsx"]}`, nil
		case strings.Contains(prompt, "Accepted behavior: increment the current count"):
			if !strings.Contains(
				prompt,
				"REUSABLE_ACCEPTED_PATHS_JSON:\n[\"src/shared.tsx\",\"tests/display.test.tsx\"]",
			) {
				t.Fatalf("shared-path stack lacks reusable earlier-task authority: %s", prompt)
			}
			return `{"schema":"omnidex.target-tree.v1","paths":["src/shared.tsx","tests/increment.test.tsx"]}`, nil
		default:
			t.Fatalf("unexpected focused target-tree prompt: %s", prompt)
			return "", nil
		}
	})}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "review-model", specification, workload, stack, []string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != len(workload.Tasks) {
		t.Fatalf("focused target-tree calls=%d tasks=%d", calls, len(workload.Tasks))
	}
	wantPaths := []string{"src/shared.tsx", "tests/display.test.tsx", "tests/increment.test.tsx"}
	if !reflect.DeepEqual(tree.Paths, wantPaths) {
		t.Fatalf("union paths=%v want=%v", tree.Paths, wantPaths)
	}
	sharedOwners, err := coverage.TasksForPath("src/shared.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{workload.Tasks[0].ID, workload.Tasks[1].ID}; !reflect.DeepEqual(sharedOwners, want) {
		t.Fatalf("shared owners=%v want=%v", sharedOwners, want)
	}
	for index, path := range []string{"tests/display.test.tsx", "tests/increment.test.tsx"} {
		owners, err := coverage.TasksForPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{workload.Tasks[index].ID}; !reflect.DeepEqual(owners, want) {
			t.Fatalf("path %s owners=%v want=%v", path, owners, want)
		}
	}
}

func TestTargetTreeResolutionCorrectsUnsupportedLeafBeforeCoverageConstruction(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a count",
	)
	calls := 0
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 2, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		calls++
		switch calls {
		case 1:
			if model != "tree-model" {
				t.Fatalf("initial model=%q", model)
			}
			return `{"schema":"omnidex.target-tree.v1","paths":["src/styles/globals.css"]}`, nil
		case 2:
			if model != "review-model" {
				t.Fatalf("correction model=%q", model)
			}
			for _, expected := range []string{"CURRENT_TARGET_TREE_CANDIDATE_JSON", "VALIDATION_FAILURE", "src/styles/globals.css", "selected project stack"} {
				if !strings.Contains(prompt, expected) {
					t.Fatalf("correction prompt missing %q: %s", expected, prompt)
				}
			}
			return `{"schema":"omnidex.target-tree.v1","paths":["src/counter.tsx","tests/counter.test.tsx"]}`, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return "", nil
		}
	})}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "review-model", specification, workload, stack, []string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Join(tree.Paths, ",") != "src/counter.tsx,tests/counter.test.tsx" {
		t.Fatalf("calls=%d paths=%v", calls, tree.Paths)
	}
	if err := coverage.ValidateFor(tree, workload); err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveCommandLineTargetTreeProjectsDistinctTaskPairsWithoutInference(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"two command behaviors", "return the first value", "return the second value",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{}, fmt.Errorf("forbidden Rust target-tree inference")
		},
	}
	stack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "review-model", specification, workload, stack, nil, nil,
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
	if err := coverage.ValidateFor(tree, workload); err != nil {
		t.Fatal(err)
	}
}

func TestPHPFocusedTargetTreeIsMechanicallyProjectedWithoutInference(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceService,
		"record service", "return the first record", "return the second record",
	)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: testPortableExecutor(func(_ string, _, _ string, _ map[string]any) (string, error) {
			calls++
			return "", fmt.Errorf("PHP rigid target-tree grammar must not invoke inference")
		}),
	}
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "", "", specification, workload, stack,
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
	if calls != 0 || !reflect.DeepEqual(tree.Paths, want) {
		t.Fatalf("inference calls=%d paths=%v want=%v", calls, tree.Paths, want)
	}
	for index, artifactPath := range []string{"src/Feature002.php", "src/Feature004.php"} {
		owners, ownerErr := coverage.TasksForPath(artifactPath)
		if ownerErr != nil {
			t.Fatal(ownerErr)
		}
		if !reflect.DeepEqual(owners, []string{workload.Tasks[index].ID}) {
			t.Fatalf("path %s owners=%v", artifactPath, owners)
		}
	}
}
