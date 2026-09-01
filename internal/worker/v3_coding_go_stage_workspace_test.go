package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoTaskVerificationProjectsOnlyCurrentTaskTargetPair(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "two independent Go behaviors",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Produce the first observable value."},
			{ID: "requirement_002", SourceQuote: "Produce the second observable value."},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{Paths: []string{
		"feature001.go", "feature001_test.go", "feature002.go", "feature002_test.go",
	}}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target, map[string][]string{
			"task_001": {"feature001.go", "feature001_test.go"},
			"task_002": {"feature002.go", "feature002_test.go"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	context, err := assemblyline.ProjectApplicationTaskContext(workload, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{Workload: workload, TargetTree: target, Coverage: coverage}
	projected, err := projectDirectCodingGoTaskVerificationProgram(program, context)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature002.go", "feature002_test.go"}
	if !sameExactStrings(projected.TargetTree.Paths, want) {
		t.Fatalf("task target paths=%v; want %v", projected.TargetTree.Paths, want)
	}
	if !sameExactStrings(program.TargetTree.Paths, target.Paths) {
		t.Fatalf("task projection mutated complete target authority: %v", program.TargetTree.Paths)
	}
}
