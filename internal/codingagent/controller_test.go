package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
func (fixtureEnvironment) PrepareFiles(context.Context, Task) error   { return nil }
func (fixtureEnvironment) Attest(context.Context) error               { return nil }
func (fixtureEnvironment) WriteFile(context.Context, FileWrite) error { return nil }

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
	return &Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Configuration: config, Requirements: []Requirement{{"r1", "A works"}, {"r2", "B works"}}, Providers: map[string]Provider{"planner": provider, "executor": provider, "reviewer": provider}, Verifier: verify, Environment: fixtureEnvironment{}, Evidence: journal}, journal
}
func passingChecks(_ context.Context, _ Assignment, checks []Check) ([]CheckResult, error) {
	results := make([]CheckResult, len(checks))
	for index, check := range checks {
		results[index] = CheckResult{ID: check.ID}
	}
	return results, nil
}
func cleanReview() ReviewResult { return ReviewResult{Findings: []Finding{}, Questions: []string{}} }

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
	plans := 0
	for _, entry := range journal.entries {
		if entry.Phase == "plan_frozen" {
			plans++
		}
	}
	if plans != 1 || !executed["a"] || !executed["b"] {
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
		ctx, cancel := context.WithCancel(context.Background())
		planner := controller.Providers["planner"]
		controller.Providers["planner"] = providerFunc(func(ctx context.Context, call Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
			if len(call.ValidationNotes) > 0 {
				cancel()
				return Result{}, ctx.Err()
			}
			return planner.Run(ctx, call, writer, event)
		})
		if err := controller.Run(ctx); err == nil {
			t.Fatal("invalid scope accepted")
		}
		cancel()
		for _, entry := range journal.entries {
			if entry.Phase == "plan_frozen" {
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
