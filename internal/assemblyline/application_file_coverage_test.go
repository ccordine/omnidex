package assemblyline

import (
	"reflect"
	"testing"
)

func TestApplicationFileCoveragePlanSupportsPluralSharedAndImplementationOnlyFiles(t *testing.T) {
	workload := applicationFileCoverageWorkload(t)
	target := TargetTree{Paths: []string{"command.go", "shared.go", "storage.go"}}
	plan, err := NewApplicationFileCoveragePlan(
		workload, target,
		map[string][]string{
			"command.go": {"task_001"},
			"shared.go":  {"task_001", "task_002"},
			"storage.go": {"task_002"},
		},
		map[string]TargetArtifactKind{
			"command.go": TargetArtifactImplementation,
			"shared.go":  TargetArtifactImplementation,
			"storage.go": TargetArtifactImplementation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	files, err := plan.FilesForTask("task_001")
	if err != nil || len(files) != 2 {
		t.Fatalf("task files=%+v error=%v", files, err)
	}
	owners, err := plan.TasksForPath("shared.go")
	if err != nil || !reflect.DeepEqual(owners, []string{"task_001", "task_002"}) {
		t.Fatalf("shared owners=%v error=%v", owners, err)
	}
}

func TestApplicationFileCoveragePlanRejectsUnknownDuplicateOrUncoveredAuthority(t *testing.T) {
	workload := applicationFileCoverageWorkload(t)
	target := TargetTree{Paths: []string{"first.go", "second.go"}}
	for name, provenance := range map[string]map[string][]string{
		"unknown": {
			"first.go": {"task_999"}, "second.go": {"task_002"},
		},
		"duplicate": {
			"first.go": {"task_001", "task_001"}, "second.go": {"task_002"},
		},
		"uncovered": {
			"first.go": {"task_001"}, "second.go": {"task_001"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewApplicationFileCoveragePlan(
				workload, target, provenance,
				map[string]TargetArtifactKind{
					"first.go": TargetArtifactImplementation, "second.go": TargetArtifactVerification,
				},
			)
			if err == nil {
				t.Fatal("accepted invalid file coverage authority")
			}
		})
	}
}

func applicationFileCoverageWorkload(t *testing.T) FrozenApplicationWorkload {
	t.Helper()
	input := ApplicationWorkloadDraftInput{
		Surface: ApplicationSurfaceCommandLine, ProductQuote: "record command",
		Requirements: []Requirement{
			{ID: "requirement_001", SourceQuote: "create records"},
			{ID: "requirement_002", SourceQuote: "list records"},
		},
	}
	workload, err := FreezeApplicationWorkload(input, ApplicationWorkloadDraft{
		Schema: ApplicationWorkloadDraftSchemaV1,
		Tasks: []ApplicationWorkloadTaskDraft{
			{RequirementID: "requirement_001", Objective: "Create records.", RequiredBehaviors: []string{"Accept record input."}, AcceptanceCriteria: []string{"A record is accepted."}},
			{RequirementID: "requirement_002", Objective: "List records.", RequiredBehaviors: []string{"Render records."}, AcceptanceCriteria: []string{"Records are visible."}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return workload
}
