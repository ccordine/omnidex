package worker

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestOneTargetTreePairCarriesSharedCoverageForSeveralTasks(t *testing.T) {
	_, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"counter application", "display a current count", "increment the current count",
	)
	target := assemblyline.TargetTree{
		StackID: genericTypeScriptBrowserAdapter,
		Paths:   []string{"src/counter.tsx", "tests/counter.test.tsx"},
	}
	plan, err := assemblyline.NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"src/counter.tsx":        {workload.Tasks[0].ID, workload.Tasks[1].ID},
			"tests/counter.test.tsx": {workload.Tasks[0].ID, workload.Tasks[1].ID},
		},
		map[string]assemblyline.TargetArtifactKind{
			"src/counter.tsx":        assemblyline.TargetArtifactImplementation,
			"tests/counter.test.tsx": assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		pair, err := directCodingTaskSinglePair(plan, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if pair.ImplementationPath != "src/counter.tsx" || pair.VerificationPath != "tests/counter.test.tsx" {
			t.Fatalf("task %s pair=%+v", task.ID, pair)
		}
	}
	owners, err := plan.TasksForPath("src/counter.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{workload.Tasks[0].ID, workload.Tasks[1].ID}; !reflect.DeepEqual(owners, want) {
		t.Fatalf("shared implementation owners=%v want=%v", owners, want)
	}
}
