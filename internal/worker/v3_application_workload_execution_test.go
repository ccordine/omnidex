package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingWorkloadExecutesExactRequirementsInAcceptedOrder(t *testing.T) {
	t.Parallel()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceCommandLine,
		ProductQuote: "record utility",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Create one record"},
			{ID: "requirement_002", SourceQuote: "List current records"},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	seen := make([]string, 0, len(workload.Tasks))
	err = executeDirectCodingApplicationWorkload(
		workload,
		func(context assemblyline.ApplicationTaskContext) error {
			seen = append(seen, context.Task.TaskID+":"+context.Task.RequirementQuote)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"task_001:Create one record",
		"task_002:List current records",
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("execution order=%v want %v", seen, want)
	}
}

func TestDirectCodingTaskBehaviorContainsNoPlannerExpansion(t *testing.T) {
	t.Parallel()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "record console",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Group records by status",
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	context, err := assemblyline.ProjectApplicationTaskContext(workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	behavior, err := compileDirectCodingApplicationTaskBehavior(context, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		string(specification.Surface), specification.ProductQuote,
		specification.Requirements[0].SourceQuote,
	} {
		if !strings.Contains(behavior, required) {
			t.Fatalf("task behavior omitted exact authority %q: %s", required, behavior)
		}
	}
	for _, forbidden := range []string{
		"Derived implementation objective", "Derived build decision",
		"Derived verification check", "acceptance criterion", "required behavior",
	} {
		if strings.Contains(behavior, forbidden) {
			t.Fatalf("task behavior contains planner expansion %q: %s", forbidden, behavior)
		}
	}
}
