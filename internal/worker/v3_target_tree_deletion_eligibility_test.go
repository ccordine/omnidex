package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestCodingDriverGrantsDeletionOnlyToAdapterRecognizedManagedFiles(t *testing.T) {
	t.Parallel()
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a current count",
	)
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	input, err := directCodingTargetTreeInput(
		"inventory request",
		specification,
		workload,
		stack,
		[]string{"README.md", "src/App.tsx", "src/obsolete.tsx"},
		[]string{"src", "tests"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input.ExistingPaths, []string{"src/obsolete.tsx"}) {
		t.Fatalf("managed deletion eligibility=%v", input.ExistingPaths)
	}
	target, err := assemblyline.DecodeTargetTreeCandidate(
		input,
		"ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx",
	)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := assemblyline.DiffTargetTree(input, target, input.ExistingPaths)
	if err != nil {
		t.Fatal(err)
	}
	want := []assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeDelete, Path: "src/obsolete.tsx"},
		{Kind: assemblyline.TargetTreeCreate, Path: "src/counter.tsx"},
		{Kind: assemblyline.TargetTreeCreate, Path: "tests/counter.test.tsx"},
	}
	if !reflect.DeepEqual(transitions, want) {
		t.Fatalf("transitions=%+v want=%+v", transitions, want)
	}
}
