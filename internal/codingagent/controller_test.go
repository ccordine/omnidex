package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	return append([]Entry(nil), journal.entries...), nil
}

type fixtureEnvironment struct{}

func (fixtureEnvironment) ReadFiles(context.Context) ([]string, error) {
	return []string{"reference.txt"}, nil
}
func (fixtureEnvironment) ReadTaskFile(context.Context, string) (string, error) {
	return "fixture", nil
}
func (fixtureEnvironment) PrepareFiles(context.Context, Task) error     { return nil }
func (fixtureEnvironment) Attest(context.Context) error                 { return nil }
func (fixtureEnvironment) WriteFile(context.Context, FileWrite) error   { return nil }
func (fixtureEnvironment) DeleteFile(context.Context, FileDelete) error { return nil }

type providerFunc func(context.Context, Assignment, FileWriter, func(NativeEvent) error) (Result, error)

func (provider providerFunc) Run(ctx context.Context, call Assignment, files FileWriter, receipt func(NativeEvent) error) (Result, error) {
	return provider(ctx, call, files, receipt)
}

type verifierFunc func(context.Context, Assignment, []Check) ([]CheckResult, error)

func (verifier verifierFunc) ExecuteChecks(ctx context.Context, call Assignment, checks []Check, _ func(model.AgentActivity) error) ([]CheckResult, error) {
	return verifier(ctx, call, checks)
}

type testWorkload struct{ Tasks []Task }

func fixtureWorkload() testWorkload {
	return testWorkload{Tasks: []Task{{
		ID: "task-a", Goal: "Implement A",
		Guidance:   []TaskGuidance{{ID: "requirement-a", Kind: "requirement", Text: "A works"}},
		Acceptance: []string{"A works"}, RequirementIDs: []string{"requirement-a"},
		AcceptanceCriterionIDs: []string{"acceptance-a"}, ReadFiles: []string{"reference.txt"},
		WriteFiles: []string{"a.txt"}, Checks: []Check{{ID: "check-a", Argv: []string{"check-a"}}},
	}}}
}

func fixtureController(t *testing.T, workload testWorkload, behavior func(Assignment) (any, error), verify verifierFunc) (*Controller, *memoryJournal) {
	t.Helper()
	journal := &memoryJournal{}
	sequence := 0
	provider := providerFunc(func(ctx context.Context, call Assignment, files FileWriter, event func(NativeEvent) error) (Result, error) {
		requiresFiles := call.Role == Execute || call.Role == Review || call.Role.IsPlanning() && len(call.ReadFiles) > 0
		if requiresFiles != (files != nil) {
			t.Fatal("agent call did not receive its exact scoped file access")
		}
		sequence++
		if err := event(NativeEvent{Frame: json.RawMessage(`{"native":"receipt"}`)}); err != nil {
			return Result{}, err
		}
		content, err := behavior(call)
		if err != nil {
			return Result{}, err
		}
		encoded, _ := json.Marshal(content)
		return Result{Provider: call.Route.Provider, Model: call.Route.Model,
			ThreadID: fmt.Sprintf("thread-%d", sequence), TurnID: fmt.Sprintf("turn-%d", sequence),
			Completed: true, Content: encoded}, nil
	})
	configuration := Configuration{Mode: Agents,
		Planner:  Route{Provider: "planner", Model: "plan-model"},
		Executor: Route{Provider: "executor", Model: "execute-model"},
		Reviewer: Route{Provider: "reviewer", Model: "review-model"},
	}
	return &Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Configuration: configuration,
		Requirements: []Requirement{{ID: "objective", Outcome: "ship the requested change"}},
		Objective:    &Objective{Instruction: "ship the requested change"},
		Providers:    map[string]Provider{"planner": provider, "executor": provider, "reviewer": provider},
		Verifier:     verify, Environment: fixtureEnvironment{}, Evidence: journal}, journal
}

func passingChecks(_ context.Context, _ Assignment, checks []Check) ([]CheckResult, error) {
	results := make([]CheckResult, len(checks))
	for index, check := range checks {
		results[index] = CheckResult{ID: check.ID}
	}
	return results, nil
}

func cleanReview() ReviewResult {
	return ReviewResult{Findings: []Finding{}, Questions: []string{}, Verification: "Inspected the task-local result against its acceptance criteria."}
}

func TestRunTaskUsesOnlyOneFreshExecutionAndReviewChannel(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	var calls []Assignment
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		calls = append(calls, call)
		if call.Task == nil || !reflect.DeepEqual(*call.Task, task) || call.Objective != nil || len(call.Requirements) != 0 || len(call.ArtifactBindings) != 0 {
			t.Fatalf("task call received anything outside its compiled packet: %#v", call)
		}
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Role != Execute || calls[1].Role != Review {
		t.Fatalf("task call sequence = %#v", calls)
	}
	var threads []string
	for _, entry := range journal.entries {
		if entry.Phase == "call_completed" {
			threads = append(threads, entry.Result.ThreadID)
		}
	}
	if len(threads) != 2 || threads[0] == threads[1] || journal.entries[len(journal.entries)-1].Phase != "task_accepted" {
		t.Fatalf("task did not use isolated accepted calls: %#v", journal.entries)
	}
}

func TestRunTaskRoundBudgetIsLocalToEachLedgerTask(t *testing.T) {
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	for index := 0; index < 4; index++ {
		task := fixtureWorkload().Tasks[0]
		task.ID = fmt.Sprintf("task-%d", index+1)
		if err := controller.RunTask(context.Background(), task); err != nil {
			t.Fatalf("task %d inherited another task's retry budget: %v", index+1, err)
		}
	}
}

func TestRunTaskCorrectionNeverLeavesItsPacket(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	reviews := 0
	executions := 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Task == nil || call.Task.ID != task.ID {
			t.Fatal("correction left its selected task")
		}
		if call.Role == Execute {
			executions++
			return ExecutionResult{Questions: []string{}}, nil
		}
		reviews++
		if reviews == 1 {
			return ReviewResult{Findings: []Finding{{TaskID: task.ID, Requirement: "A works", Observation: "A is incomplete"}}, Questions: []string{}, Verification: "Inspected the task-local result and observed the stated defect."}, nil
		}
		return cleanReview(), nil
	}, passingChecks)
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if executions != 2 || reviews != 2 {
		t.Fatalf("correction calls executions=%d reviews=%d", executions, reviews)
	}
}

func TestRunTaskDoesNotSpendReviewOnKnownFailedChecks(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	var roles []Role
	checkRuns := 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		roles = append(roles, call.Role)
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, func(_ context.Context, _ Assignment, checks []Check) ([]CheckResult, error) {
		checkRuns++
		results := make([]CheckResult, len(checks))
		for index, check := range checks {
			results[index] = CheckResult{ID: check.ID}
			if checkRuns == 1 {
				results[index].ExitCode = 1
				results[index].Output = "deterministic failure"
			}
		}
		return results, nil
	})
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	want := []Role{Execute, Execute, Review}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("agent roles = %v, want %v", roles, want)
	}
}

func TestRunTaskAllowsReviewOnlyVerification(t *testing.T) {
	task := Task{ID: "inspect", Goal: "Inspect delivered state", Acceptance: []string{"state is correct"}}
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			return ExecutionResult{Questions: []string{}}, nil
		}
		return cleanReview(), nil
	}, func(context.Context, Assignment, []Check) ([]CheckResult, error) {
		t.Fatal("review-only task ran a command check")
		return nil, nil
	})
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if last := journal.entries[len(journal.entries)-1]; last.Phase != "task_accepted" || len(last.Checks) != 0 {
		t.Fatalf("review-only acceptance = %#v", last)
	}
}
