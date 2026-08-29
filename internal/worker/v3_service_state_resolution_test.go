package worker

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceStateResolutionClassifiesEveryTaskWithOneAttemptEach(t *testing.T) {
	t.Parallel()
	_, workload := serviceStateWorkloadFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 4, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
			calls++
			if model != "state-model" {
				t.Fatalf("model=%q", model)
			}
			for _, task := range workload.Tasks {
				if strings.Contains(prompt, task.ID) {
					t.Fatalf("state prompt exposed task identity %s: %s", task.ID, prompt)
				}
			}
			for _, forbidden := range []string{
				".php", "workspace", "filename", "tool", "command", "endpoint",
				"database", "postgresql", "redis", "cache", "filesystem",
			} {
				if strings.Contains(strings.ToLower(prompt), forbidden) {
					t.Fatalf("state prompt exposed %q authority: %s", forbidden, prompt)
				}
			}
			lifetime := assemblyline.ApplicationServiceStateRequestLocalOnly
			if calls == 2 {
				lifetime = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
			}
			return string(lifetime), nil
		}),
	}
	plan, err := resolveDirectCodingServiceStatePlan(
		runtime, "state-model", workload, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != len(workload.Tasks) || len(plan.ByTask) != len(workload.Tasks) ||
		plan.WorkloadSHA256 != workload.SHA256 {
		t.Fatalf("calls=%d plan=%+v", calls, plan)
	}
	if plan.ByTask[workload.Tasks[0].ID] != assemblyline.ApplicationServiceStateRequestLocalOnly ||
		plan.ByTask[workload.Tasks[1].ID] != assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
		t.Fatalf("state decisions=%+v", plan.ByTask)
	}
}

func TestServiceStateResolutionDoesNotRetryInvalidLeaf(t *testing.T) {
	t.Parallel()
	_, workload := serviceStateWorkloadFixture(t)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 9, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, _ string, _ string) (string, error) {
			calls++
			return "unknown", nil
		}),
	}
	_, err := resolveDirectCodingServiceStatePlan(
		runtime, "state-model", workload, nil,
	)
	if err == nil || calls != 1 {
		t.Fatalf("invalid lifetime error=%v calls=%d", err, calls)
	}
}

func TestServiceStatePlanIsCompleteAndProjectsOnlyCurrentTask(t *testing.T) {
	t.Parallel()
	_, workload := serviceStateWorkloadFixture(t)
	plan := testRequestLocalServiceStatePlan(workload)
	if err := plan.ValidateFor(workload); err != nil {
		t.Fatal(err)
	}
	projected, err := plan.projectTask(workload.Tasks[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if projected.WorkloadSHA256 != workload.SHA256 || len(projected.ByTask) != 1 ||
		projected.ByTask[workload.Tasks[1].ID] != assemblyline.ApplicationServiceStateRequestLocalOnly {
		t.Fatalf("projected state=%+v", projected)
	}
	delete(plan.ByTask, workload.Tasks[0].ID)
	plan.ByTask["task_unknown"] = assemblyline.ApplicationServiceStateRequestLocalOnly
	if err := plan.ValidateFor(workload); err == nil {
		t.Fatal("accepted missing and unknown state authority")
	}
}

func TestPHPServiceStateHookCompilesCrossRequestAuthorityToPostgreSQL(t *testing.T) {
	t.Parallel()
	specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
	requestLocal := testRequestLocalServiceStatePlan(workload)
	if err := validatePHPServiceStateLifetime(workload, requestLocal); err != nil {
		t.Fatal(err)
	}
	crossRequest := testRequestLocalServiceStatePlan(workload)
	crossRequest.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	capabilities := directCodingCapabilityGraph{"requirement_001": nil}
	crossRequest = bindTestServiceStateInterfaces(
		t, workload, capabilities, crossRequest,
		testStringServiceStateField("durable feature state"),
	)
	if err := validatePHPServiceStateLifetime(workload, crossRequest); err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingServiceProgram(
		"php-service", specification, nil, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage,
		crossRequest, endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	hasState, err := phpServiceProgramRequiresPostgreSQL(program)
	if err != nil || !hasState {
		t.Fatalf("central service compile durable state=%t error=%v", hasState, err)
	}
	_, staticFiles, err := compileGenericPHPServiceBlueprint(
		"php-service", specification, map[string]directCodingSkillBinding{}, workload,
		capabilities, target, coverage,
		crossRequest, endpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(phpServiceFileContent(t, staticFiles, phpServiceStateMigrationPath)) == "" {
		t.Fatal("direct PHP compiler omitted its code-owned PostgreSQL migration")
	}
}

func TestServiceStateBoundaryRunsBeforeTargetTreeResolution(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_driver_plan.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "v3_coding_driver_plan.go", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := directCodingAssembleBody(t, file)
	calls := make([]string, 0)
	collectCalls(body, &calls)
	state := uniqueWorkloadCallIndex(t, calls, "resolveServiceStateBeforeTargetTree")
	tree := uniqueWorkloadCallIndex(t, calls, "resolveDirectCodingTargetTree")
	if state >= tree {
		t.Fatalf("service state boundary does not precede target tree: %v", calls)
	}
}

func TestServiceStateHookFollowsHTTPCompilerCapability(t *testing.T) {
	t.Parallel()
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.CompileServiceSource != nil {
			if stack.ValidateServiceState == nil {
				t.Fatalf("HTTP stack %s has no state-lifetime hook", stack.ID)
			}
			continue
		}
		if stack.ValidateServiceState != nil {
			t.Fatalf("non-HTTP stack %s registered service state authority", stack.ID)
		}
	}
	_, workload, _ := applicationTaskLifecycleFixture(t)
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := (&directCodingSession{}).resolveServiceStateBeforeTargetTree(
		typedWorkerRuntime{}, stack, workload, nil,
	)
	if err != nil || plan.WorkloadSHA256 != "" || len(plan.ByTask) != 0 {
		t.Fatalf("non-service state resolution plan=%+v error=%v", plan, err)
	}
}

func collectCalls(node ast.Node, calls *[]string) {
	ast.Inspect(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallExpr)
		if ok {
			*calls = append(*calls, workloadCallName(call.Fun))
		}
		return true
	})
}

func serviceStateWorkloadFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: "inventory service",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Accept one inventory record."},
			{ID: "requirement_002", SourceQuote: "Return a requested inventory record."},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	return specification, workload
}
