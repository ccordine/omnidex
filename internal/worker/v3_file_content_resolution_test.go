package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationFileCoveragePlanSupportsPluralSharedAndImplementationOnlyLeaves(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"record command", "create records", "list records",
	)
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID: stack.ID,
		Paths:   []string{"create.go", "list.go", "shared.go"},
	}
	plan, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target,
		map[string][]string{
			workload.Tasks[0].ID: {"create.go", "shared.go"},
			workload.Tasks[1].ID: {"list.go", "shared.go"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.ValidateFor(target, workload); err != nil {
		t.Fatal(err)
	}
	firstFiles, err := plan.FilesForTask(workload.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := coveragePaths(firstFiles); !reflect.DeepEqual(got, []string{"create.go", "shared.go"}) {
		t.Fatalf("first task files=%v", got)
	}
	owners, err := plan.TasksForPath("shared.go")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{workload.Tasks[0].ID, workload.Tasks[1].ID}; !reflect.DeepEqual(owners, want) {
		t.Fatalf("shared owners=%v want=%v", owners, want)
	}
	for _, file := range plan.Files {
		if file.Kind != assemblyline.TargetArtifactImplementation {
			t.Fatalf("implementation-only plan contains %+v", file)
		}
	}
	if specification.Surface != assemblyline.ApplicationSurfaceCommandLine {
		t.Fatalf("fixture surface=%s", specification.Surface)
	}
}

func TestApplicationFileCoveragePlanRejectsMissingOrInvalidTaskProvenance(t *testing.T) {
	_, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"record command", "create records", "list records",
	)
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{StackID: stack.ID, Paths: []string{"first.go", "second.go"}}
	for name, taskPaths := range map[string]map[string][]string{
		"missing task": {
			workload.Tasks[0].ID: {"first.go", "second.go"},
		},
		"non-target path": {
			workload.Tasks[0].ID: {"first.go"},
			workload.Tasks[1].ID: {"outside.go"},
		},
		"duplicate path": {
			workload.Tasks[0].ID: {"first.go", "first.go"},
			workload.Tasks[1].ID: {"second.go"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildDirectCodingApplicationFileCoveragePlan(stack, workload, target, taskPaths)
			if err == nil {
				t.Fatal("accepted invalid focused target-tree provenance")
			}
		})
	}
}

func coveragePaths(files []assemblyline.ApplicationFileCoverage) []string {
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.Path
	}
	return paths
}

func testApplicationFileCoverageAuthority(
	t *testing.T,
	surface assemblyline.ApplicationSurface,
	product string,
	requirementQuotes ...string,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	requirements := make([]assemblyline.Requirement, len(requirementQuotes))
	tasks := make([]assemblyline.ApplicationWorkloadTaskDraft, len(requirementQuotes))
	for index, quote := range requirementQuotes {
		requirementID := fmt.Sprintf("requirement_%03d", index+1)
		requirements[index] = assemblyline.Requirement{ID: requirementID, SourceQuote: quote}
		tasks[index] = assemblyline.ApplicationWorkloadTaskDraft{
			RequirementID: requirementID,
			Objective:     "Deliver " + strings.TrimSuffix(quote, ".") + ".",
			RequiredBehaviors: []string{
				"Implement " + strings.TrimSuffix(quote, ".") + ".",
			},
			AcceptanceCriteria: []string{
				"The application visibly supports " + strings.TrimSuffix(quote, ".") + ".",
			},
		}
	}
	specification := assemblyline.ApplicationSpecification{
		Surface: surface, ProductQuote: product, Requirements: requirements,
	}
	input := applicationWorkloadInput(specification)
	workload, err := assemblyline.FreezeApplicationWorkload(input, assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks:  tasks,
	})
	if err != nil {
		t.Fatal(err)
	}
	return specification, workload
}
