package worker

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRegisteredStackReservationsEqualCompiledCodeOwnedTargetPaths(t *testing.T) {
	tests := []struct {
		stackID string
		surface assemblyline.ApplicationSurface
		paths   []string
	}{
		{genericTypeScriptBrowserAdapter, assemblyline.ApplicationSurfaceBrowser, []string{"src/feature.tsx", "src/feature.test.tsx"}},
		{genericGoCommandLineAdapter, assemblyline.ApplicationSurfaceCommandLine, []string{"feature.go", "feature_test.go"}},
		{genericJavaScriptCommandLineAdapter, assemblyline.ApplicationSurfaceCommandLine, []string{"feature.mjs", "feature.test.mjs"}},
		{genericRustCommandLineAdapter, assemblyline.ApplicationSurfaceCommandLine, []string{"src/feature.rs", "tests/feature_test.rs"}},
		{genericJavaCommandLineAdapter, assemblyline.ApplicationSurfaceCommandLine, []string{"Feature.java", "FeatureTest.java"}},
	}
	for _, test := range tests {
		t.Run(test.stackID, func(t *testing.T) {
			stack, err := directCodingProjectStackByID(test.stackID)
			if err != nil {
				t.Fatal(err)
			}
			specification, workload := testApplicationFileCoverageAuthority(
				t, test.surface, "reserved path closure", "return one observable value",
			)
			target := assemblyline.TargetTree{
				StackID: stack.ID, VersionProfileID: stack.DefaultVersionProfileID,
				Paths: append([]string(nil), test.paths...),
			}
			sort.Strings(target.Paths)
			coverage, err := buildDirectCodingApplicationFileCoveragePlan(
				stack, workload, target,
				map[string][]string{workload.Tasks[0].ID: append([]string(nil), target.Paths...)},
			)
			if err != nil {
				t.Fatal(err)
			}
			capabilities := directCodingCapabilityGraph{
				specification.Requirements[0].ID: nil,
			}
			program, err := compileDirectCodingProgram(
				"reservation-closure", specification, nil, map[string]directCodingSkillBinding{},
				workload, capabilities, target, coverage,
			)
			if err != nil {
				t.Fatal(err)
			}
			got := compiledCodeOwnedTargetPaths(t, stack, program, target)
			if !reflect.DeepEqual(got, stack.TargetTreeReservedPaths) {
				t.Fatalf(
					"compiled code-owned target paths=%v reservations=%v",
					got, stack.TargetTreeReservedPaths,
				)
			}
		})
	}
}

func TestTypeScriptTargetTreeContractCanReconcileExistingWorkloadPaths(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"existing browser", "preserve and update the existing behavior",
	)
	existing := []string{"src/feature.test.tsx", "src/feature.tsx"}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			for _, expected := range []string{
				"CURRENT_MANAGED_WORKLOAD_TREE:\nROOT\n  D src\n    F feature.test.tsx\n    F feature.tsx",
				"CODE_RESERVED_TREE:\nROOT\n  D src\n    F App.test.tsx\n    F App.tsx\n    F main.tsx\n    F runtime.test.tsx\n    F runtime.tsx",
			} {
				if !strings.Contains(prompt, expected) {
					t.Fatalf("inferred target-tree prompt lacks %q: %s", expected, prompt)
				}
			}
			return "ROOT\n  D src\n    F feature.tsx\n    F feature.test.tsx", nil
		}),
	}
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	tree, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "tree-model", specification, workload, stack,
		existing, []string{"src"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tree.Paths, existing) {
		t.Fatalf("target paths=%v want existing=%v", tree.Paths, existing)
	}
	if err := coverage.ValidateFor(tree, workload); err != nil {
		t.Fatal(err)
	}
	input, err := directCodingTargetTreeInput(
		specification, workload, stack, existing, []string{"src"},
	)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := assemblyline.DiffTargetTree(input, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTransitions := []assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeReconcile, Path: "src/feature.test.tsx"},
		{Kind: assemblyline.TargetTreeReconcile, Path: "src/feature.tsx"},
	}
	if !reflect.DeepEqual(transitions, wantTransitions) {
		t.Fatalf("final transitions=%v want=%v", transitions, wantTransitions)
	}
	for _, reservedPath := range stack.TargetTreeReservedPaths {
		if containsTargetTreePath(tree.Paths, reservedPath) ||
			containsTargetTreeCoveragePath(coverage, reservedPath) ||
			containsTargetTreeTransitionPath(transitions, reservedPath) {
			t.Fatalf("prompt-only reservation %s escaped into target authority", reservedPath)
		}
	}
}

func compiledCodeOwnedTargetPaths(
	t *testing.T,
	stack directCodingProjectStack,
	program directCodingProgram,
	target assemblyline.TargetTree,
) []string {
	t.Helper()
	targetPaths := make(map[string]struct{}, len(target.Paths))
	for _, artifactPath := range target.Paths {
		targetPaths[artifactPath] = struct{}{}
	}
	compiledPaths := make([]string, 0, len(program.Source.Documents)+len(program.StaticFiles))
	for _, document := range program.Source.Documents {
		compiledPaths = append(compiledPaths, document.Path)
	}
	for _, file := range program.StaticFiles {
		compiledPaths = append(compiledPaths, file.Path)
	}
	reserved := make(map[string]struct{})
	for _, artifactPath := range compiledPaths {
		if _, workloadPath := targetPaths[artifactPath]; workloadPath {
			continue
		}
		matches := 0
		for _, adapterID := range stack.TargetTreeAdapterIDs {
			adapter := targetTreeClosureAdapter(t, adapterID)
			if _, recognized := adapter.Recognize(artifactPath); recognized {
				matches++
			}
		}
		if matches > 1 {
			t.Fatalf("compiled path %s matches %d target-tree adapters", artifactPath, matches)
		}
		if matches == 1 {
			reserved[artifactPath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(reserved))
	for artifactPath := range reserved {
		paths = append(paths, artifactPath)
	}
	sort.Strings(paths)
	return paths
}

func targetTreeClosureAdapter(t *testing.T, adapterID string) directCodingArtifactAdapter {
	t.Helper()
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		if adapter.ID == adapterID {
			return adapter
		}
	}
	t.Fatalf("target-tree adapter %s is not registered", adapterID)
	return directCodingArtifactAdapter{}
}

func containsTargetTreePath(paths []string, target string) bool {
	for _, artifactPath := range paths {
		if artifactPath == target {
			return true
		}
	}
	return false
}

func containsTargetTreeCoveragePath(
	coverage assemblyline.ApplicationFileCoveragePlan,
	target string,
) bool {
	for _, file := range coverage.Files {
		if file.Path == target {
			return true
		}
	}
	return false
}

func containsTargetTreeTransitionPath(
	transitions []assemblyline.TargetTreeTransition,
	target string,
) bool {
	for _, transition := range transitions {
		if transition.Path == target {
			return true
		}
	}
	return false
}
