package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

type memoryJournal struct{ entries []Entry }

func (journal *memoryJournal) Append(_ context.Context, entry Entry) (int64, error) {
	entry.ID = int64(len(journal.entries) + 1)
	encoded, _ := json.Marshal(entry)
	var owned Entry
	_ = json.Unmarshal(encoded, &owned)
	journal.entries = append(journal.entries, owned)
	return entry.ID, nil
}
func (journal *memoryJournal) Read(context.Context) ([]Entry, error) {
	return append([]Entry{}, journal.entries...), nil
}

type fixtureEnvironment struct{}

func (fixtureEnvironment) ReadFiles(context.Context) ([]string, error) {
	return []string{"reference.txt"}, nil
}
func (fixtureEnvironment) PrepareFiles(context.Context, Task) error     { return nil }
func (fixtureEnvironment) Attest(context.Context) error                 { return nil }
func (fixtureEnvironment) WriteFile(context.Context, FileWrite) error   { return nil }
func (fixtureEnvironment) DeleteFile(context.Context, FileDelete) error { return nil }

type providerFunc func(context.Context, Assignment, FileWriter, func(NativeEvent) error) (Result, error)

func (provider providerFunc) Run(ctx context.Context, call Assignment, writer FileWriter, receipt func(NativeEvent) error) (Result, error) {
	return provider(ctx, call, writer, receipt)
}

type verifierFunc func(context.Context, Assignment, []Check) ([]CheckResult, error)

func (verifier verifierFunc) ExecuteChecks(ctx context.Context, call Assignment, checks []Check, activity func(model.AgentActivity) error) ([]CheckResult, error) {
	return verifier(ctx, call, checks)
}

func fixtureWorkload() Workload {
	return Workload{Tasks: []Task{
		{ID: "a", Goal: "A", Acceptance: []string{"A works"}, ReadFiles: []string{"reference.txt"}, WriteFiles: []string{"a.txt"}, RequirementIDs: []string{"r1"}, Checks: []Check{{ID: "a-check", Argv: []string{"check-a"}}}},
		{ID: "b", Goal: "B", Acceptance: []string{"B works"}, WriteFiles: []string{"b.txt"}, RequirementIDs: []string{"r2"}, Checks: []Check{{ID: "b-check", Argv: []string{"check-b"}}}},
	}, Checks: []Check{{ID: "together", Argv: []string{"check-all"}}}}
}
func fixtureController(t *testing.T, workload Workload, behavior func(Assignment) (any, error), verify verifierFunc) (*Controller, *memoryJournal) {
	t.Helper()
	journal := &memoryJournal{}
	sequence := 0
	provider := providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
		if (call.Role == Execute) != (writer != nil) {
			t.Fatal("review/execution effect boundary was not enforced")
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		sequence++
		if err := event(NativeEvent{Frame: json.RawMessage(`{"native":"receipt"}`)}); err != nil {
			return Result{}, err
		}
		var content any
		var err error
		if call.Role == Plan {
			content = workload
		} else {
			content, err = behavior(call)
		}
		if err != nil {
			return Result{}, err
		}
		encoded, _ := json.Marshal(content)
		return Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: fmt.Sprint("thread-", sequence), TurnID: fmt.Sprint("turn-", sequence), Completed: true, Content: encoded}, nil
	})
	config := Configuration{Mode: Agents, Planner: Route{Provider: "planner", Model: "plan-model"}, Executor: Route{Provider: "executor", Model: "execute-model"}, Reviewer: Route{Provider: "reviewer", Model: "review-model"}}
	approved := workload
	return &Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Configuration: config, Requirements: []Requirement{{"r1", "A works"}, {"r2", "B works"}}, ApprovedWorkload: &approved, Providers: map[string]Provider{"planner": provider, "executor": provider, "reviewer": provider}, Verifier: verify, Environment: fixtureEnvironment{}, Evidence: journal}, journal
}
func passingChecks(_ context.Context, _ Assignment, checks []Check) ([]CheckResult, error) {
	results := make([]CheckResult, len(checks))
	for index, check := range checks {
		results[index] = CheckResult{ID: check.ID}
	}
	return results, nil
}
func cleanReview() ReviewResult { return ReviewResult{Findings: []Finding{}, Questions: []string{}} }

type retryableFixtureError struct{}

func (retryableFixtureError) Error() string   { return "provider capacity unavailable" }
func (retryableFixtureError) Retryable() bool { return true }

func TestExecutionRequiresApprovedWorkloadAndNeverPlans(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) {
		return nil, errors.New("execution must not reach a provider without approval")
	}, passingChecks)
	controller.ApprovedWorkload = nil
	if err := controller.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "explicitly approved workload") {
		t.Fatalf("execution without approval error = %v", err)
	}
	if len(journal.entries) != 0 {
		t.Fatal("execution opened a native session before approval")
	}
}

func TestPlanningRetriesTransientProviderCapacityInFreshCalls(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) {
		return cleanReview(), nil
	}, passingChecks)
	provider := controller.Providers["planner"]
	attempts := 0
	controller.Providers["planner"] = providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
		attempts++
		if attempts < 3 {
			return Result{}, retryableFixtureError{}
		}
		return provider.Run(ctx, call, writer, event)
	})
	workload, err := controller.Propose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || len(workload.Tasks) != len(fixtureWorkload().Tasks) {
		t.Fatalf("transient retry attempts=%d workload=%#v", attempts, workload)
	}
	opened, failed, proposed := 0, 0, 0
	for _, entry := range journal.entries {
		switch entry.Phase {
		case "call_opened":
			opened++
		case "call_failed":
			failed++
		case "plan_proposed":
			proposed++
		}
	}
	if opened != 3 || failed != 2 || proposed != 1 {
		t.Fatalf("retry journal opened=%d failed=%d proposed=%d", opened, failed, proposed)
	}
}

func TestControllerConvergesAndOnlyReopensAffectedWork(t *testing.T) {
	executions := map[string]int{}
	globalFound := false
	behavior := func(call Assignment) (any, error) {
		if call.Role == Execute {
			if call.Route.Model != "execute-model" || len(call.WriteFiles) != 1 {
				t.Fatal("executor lost route or narrow write grant")
			}
			executions[call.Task.ID]++
			if len(call.QuestionContext) > 0 {
				t.Fatal("unexpected raw conversation context")
			}
			return ExecutionResult{Questions: []string{}}, nil
		}
		if len(call.WriteFiles) != 0 || call.Route.Model != "review-model" {
			t.Fatal("review received write access or wrong model")
		}
		review := cleanReview()
		if call.Role == OverallReview && !globalFound {
			globalFound = true
			review.Findings = []Finding{{TaskID: "a", File: "a.txt", Requirement: "A works", Observation: "A fails when integrated with B"}}
		} else if call.Role == Review && call.Task.ID == "a" && (executions["a"] == 1 || (globalFound && executions["a"] == 2)) {
			review.Findings = []Finding{{TaskID: "a", Requirement: "A works", Observation: "A has an observed defect"}}
		}
		return review, nil
	}
	verify := func(ctx context.Context, call Assignment, checks []Check) ([]CheckResult, error) {
		results, _ := passingChecks(ctx, call, checks)
		if call.Task != nil && call.Task.ID == "a" && executions["a"] == 1 {
			results[0].ExitCode = 1
			results[0].Output = "actual A failure"
		}
		return results, nil
	}
	controller, journal := fixtureController(t, fixtureWorkload(), behavior, verify)
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executions["a"] != 3 || executions["b"] != 1 {
		t.Fatalf("unresolved work did not converge narrowly: %v", executions)
	}
	last := journal.entries[len(journal.entries)-1]
	if last.Phase != "overall_accepted" || last.CallID == 0 {
		t.Fatal("completion lacks independent review binding")
	}
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executions["a"] != 3 || executions["b"] != 1 {
		t.Fatal("restart regenerated accepted tasks")
	}
}

func TestOverallFindingExecutesCorrectionBeforeReview(t *testing.T) {
	executions := map[string]int{}
	overallFound := false
	behavior := func(call Assignment) (any, error) {
		switch call.Role {
		case Execute:
			executions[call.Task.ID]++
			return ExecutionResult{Questions: []string{}}, nil
		case Review:
			if overallFound && call.Task.ID == "a" && executions["a"] < 2 {
				t.Fatal("overall finding reached review before corrective execution")
			}
			return cleanReview(), nil
		case OverallReview:
			if !overallFound {
				overallFound = true
				return ReviewResult{Findings: []Finding{{TaskID: "a", File: "a.txt", Requirement: "A works", Observation: "A fails after integration"}}, Questions: []string{}}, nil
			}
			return cleanReview(), nil
		default:
			t.Fatalf("unexpected role %q", call.Role)
			return nil, nil
		}
	}
	controller, journal := fixtureController(t, fixtureWorkload(), behavior, passingChecks)
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executions["a"] != 2 || executions["b"] != 1 {
		t.Fatalf("overall correction executions=%v", executions)
	}
	entries := journal.entries
	for i, entry := range entries {
		if entry.Phase != "task_reopened" || entry.TaskID != "a" || len(entry.Findings) == 0 {
			continue
		}
		if i+1 >= len(entries) || entries[i+1].Phase != "task_executing" || entries[i+1].TaskID != "a" {
			t.Fatalf("overall reopening was not followed by corrective execution: %#v", entries[i:])
		}
		return
	}
	t.Fatal("overall finding did not reopen task a")
}

func TestRestartedCatalogFixtureExecutesOnlyReopenedRenderer(t *testing.T) {
	workload := Workload{Tasks: []Task{
		{ID: "catalog", Goal: "Load catalog", Acceptance: []string{"catalog loads"}, WriteFiles: []string{"catalog.go"}, RequirementIDs: []string{"catalog"}, Checks: []Check{{ID: "catalog-check", Argv: []string{"check-catalog"}}}},
		{ID: "renderer", Goal: "Render catalog", Acceptance: []string{"catalog renders"}, WriteFiles: []string{"render.go"}, Dependencies: []string{"catalog"}, RequirementIDs: []string{"renderer"}, Checks: []Check{{ID: "renderer-check", Argv: []string{"check-renderer"}}}},
		{ID: "manual", Goal: "Document catalog", Acceptance: []string{"manual is complete"}, WriteFiles: []string{"MANUAL.md"}, RequirementIDs: []string{"manual"}, Checks: []Check{{ID: "manual-check", Argv: []string{"check-manual"}}}},
	}, Checks: []Check{{ID: "catalog-suite", Argv: []string{"check-catalog-suite"}}}}
	executions := map[string]int{}
	controller, journal := fixtureController(t, workload, func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions[call.Task.ID]++
			if call.Task.ID != "renderer" || len(call.Findings) != 1 {
				t.Fatalf("restart executed wrong scope: task=%s findings=%#v", call.Task.ID, call.Findings)
			}
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	controller.Requirements = []Requirement{{"catalog", "catalog loads"}, {"renderer", "catalog renders"}, {"manual", "manual is complete"}}
	journal.entries = []Entry{
		{ID: 1, Generation: 1, Phase: "task_executing", TaskID: "catalog", Round: 1},
		{ID: 2, Generation: 1, Phase: "task_accepted", TaskID: "catalog", Round: 2},
		{ID: 3, Generation: 1, Phase: "task_executing", TaskID: "renderer", Round: 3},
		{ID: 4, Generation: 1, Phase: "task_accepted", TaskID: "renderer", Round: 4},
		{ID: 5, Generation: 1, Phase: "task_executing", TaskID: "manual", Round: 5},
		{ID: 6, Generation: 1, Phase: "task_accepted", TaskID: "manual", Round: 6},
		{ID: 7, Generation: 1, Phase: "task_reopened", TaskID: "renderer", Round: 7, Findings: []Finding{{TaskID: "renderer", File: "render.go", Requirement: "catalog renders", Observation: "integrated rendering is stale"}}},
	}
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executions["renderer"] != 1 || executions["catalog"] != 0 || executions["manual"] != 0 {
		t.Fatalf("restart correction executions=%v", executions)
	}
	if journal.entries[len(journal.entries)-1].Phase != "overall_accepted" {
		t.Fatalf("restart did not converge: %#v", journal.entries[len(journal.entries)-1])
	}
}

func TestQuestionsPersistAndAnswersResumeWithoutReplanning(t *testing.T) {
	questioned := false
	executed := map[string]bool{}
	behavior := func(call Assignment) (any, error) {
		if call.Role == Execute {
			if call.Task.ID == "a" && !questioned {
				questioned = true
				return ExecutionResult{Questions: []string{"Which supplied format should A use?"}}, nil
			}
			executed[call.Task.ID] = true
			return ExecutionResult{Questions: []string{}}, nil
		}
		review := cleanReview()
		if call.Task != nil && !executed[call.Task.ID] {
			if len(call.QuestionContext) != 1 || call.QuestionContext[0].Questions[0] != "Which supplied format should A use?" {
				t.Fatal("resumed task lost exact answer")
			}
			review.Findings = []Finding{{TaskID: call.Task.ID, Requirement: call.Task.Acceptance[0], Observation: "requested file is still missing behavior"}}
		}
		return review, nil
	}
	controller, journal := fixtureController(t, fixtureWorkload(), behavior, passingChecks)
	var pending *QuestionsPending
	if err := controller.Run(context.Background()); !errors.As(err, &pending) {
		t.Fatalf("expected concrete question, got %v", err)
	}
	count := len(journal.entries)
	if err := controller.Run(context.Background()); !errors.As(err, &pending) || len(journal.entries) != count {
		t.Fatal("unanswered question dispatched more agents")
	}
	_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "answers", QuestionID: journal.entries[len(journal.entries)-1].ID, Answers: []string{"Use format A."}})
	if err := controller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, entry := range journal.entries {
		if entry.Phase == "plan_proposed" {
			t.Fatal("execution replanned instead of resuming its approved workload")
		}
	}
	if !executed["a"] || !executed["b"] {
		t.Fatal("answer replanned instead of resuming frozen task")
	}
}

func TestControllerRejectsInventedScopeAndMissingRequirements(t *testing.T) {
	for _, mutation := range []func(*Workload){
		func(plan *Workload) { plan.Tasks[0].ReadFiles = []string{"ungranted.txt"} },
		func(plan *Workload) { plan.Tasks[0].WriteFiles = []string{"../escape"} },
		func(plan *Workload) { plan.Tasks[0].WriteFiles = []string{".env"} },
		func(plan *Workload) { plan.Tasks[0].RequirementIDs = []string{"invented"} },
		func(plan *Workload) { plan.Tasks = plan.Tasks[:1] },
	} {
		plan := fixtureWorkload()
		mutation(&plan)
		controller, journal := fixtureController(t, plan, func(Assignment) (any, error) { t.Fatal("invalid plan reached execution"); return nil, nil }, passingChecks)
		controller.ApprovedWorkload = nil
		ctx, cancel := context.WithCancel(context.Background())
		planner := controller.Providers["planner"]
		controller.Providers["planner"] = providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
			if len(call.ValidationNotes) > 0 {
				cancel()
				return Result{}, ctx.Err()
			}
			return planner.Run(ctx, call, writer, event)
		})
		if _, err := controller.Propose(ctx); err == nil {
			t.Fatal("invalid scope accepted")
		}
		cancel()
		for _, entry := range journal.entries {
			if entry.Phase == "plan_proposed" {
				t.Fatal("invalid plan frozen")
			}
		}
	}
}

func TestFailedOverallChecksCannotAcceptCleanModelReview(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, func(ctx context.Context, call Assignment, checks []Check) ([]CheckResult, error) {
		results, _ := passingChecks(ctx, call, checks)
		if call.Role == OverallReview {
			results[0].ExitCode = 3
		}
		return results, nil
	})
	var pending *QuestionsPending
	if err := controller.Run(context.Background()); !errors.As(err, &pending) {
		t.Fatalf("failed verification escaped governance: %v", err)
	}
	for _, entry := range journal.entries {
		if entry.Phase == "overall_accepted" {
			t.Fatal("failed checks accepted")
		}
	}
}

func TestVerificationMustCoverEveryFrozenCheck(t *testing.T) {
	controller, _ := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) { return ExecutionResult{Questions: []string{}}, nil }, func(context.Context, Assignment, []Check) ([]CheckResult, error) {
		return []CheckResult{{ID: "substitute"}}, nil
	})
	if err := controller.Run(context.Background()); err == nil {
		t.Fatal("substituted verification accepted")
	}
}
