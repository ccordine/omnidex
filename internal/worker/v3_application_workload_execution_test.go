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
	wantPrefix := strings.Join([]string{
		"Delivery surface: " + string(specification.Surface),
		"Product context: " + specification.ProductQuote,
		"Exact user requirement: " + specification.Requirements[0].SourceQuote,
	}, "\n")
	if !strings.HasPrefix(behavior, wantPrefix) {
		t.Fatalf("task behavior authority order differs:\n%s", behavior)
	}
	for _, required := range []string{
		string(specification.Surface), specification.ProductQuote,
		specification.Requirements[0].SourceQuote,
	} {
		if strings.Count(behavior, required) != 1 {
			t.Fatalf("task behavior does not contain exact authority %q once: %s", required, behavior)
		}
	}
	if strings.Count(behavior, "Product context:") != 1 {
		t.Fatalf("task behavior does not identify exact product authority once: %s", behavior)
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

func TestDirectCodingTaskBehaviorIsolatesRequirementsAcrossSurfaces(t *testing.T) {
	t.Parallel()
	for _, surface := range []assemblyline.ApplicationSurface{
		assemblyline.ApplicationSurfaceBrowser,
		assemblyline.ApplicationSurfaceCommandLine,
		assemblyline.ApplicationSurfaceService,
	} {
		surface := surface
		t.Run(string(surface), func(t *testing.T) {
			t.Parallel()
			specification := assemblyline.ApplicationSpecification{
				Surface: surface, ProductQuote: "environmental sensor console",
				Requirements: []assemblyline.Requirement{
					{ID: "requirement_001", SourceQuote: "Display the current reading."},
					{ID: "requirement_002", SourceQuote: "Export the retained history."},
				},
			}
			workload, err := assemblyline.FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatal(err)
			}
			for index, task := range workload.Tasks {
				context, err := assemblyline.ProjectApplicationTaskContext(workload, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				behavior, err := compileDirectCodingApplicationTaskBehavior(context, nil)
				if err != nil {
					t.Fatal(err)
				}
				own := specification.Requirements[index].SourceQuote
				sibling := specification.Requirements[1-index].SourceQuote
				if strings.Count(behavior, own) != 1 ||
					strings.Count(behavior, specification.ProductQuote) != 1 ||
					strings.Contains(behavior, sibling) ||
					strings.Count(behavior, "Product context:") != 1 {
					t.Fatalf("task %s projection is not isolated: %s", task.ID, behavior)
				}
			}
		})
	}
}
