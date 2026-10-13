package codingagent

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestControllerCallActivityFollowsDurableLifecycleForEveryRole(t *testing.T) {
	for _, role := range []Role{Plan, Execute, Review, OverallReview} {
		t.Run(string(role), func(t *testing.T) {
			controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
				if call.Role == Execute {
					return ExecutionResult{Questions: []string{}}, nil
				}
				return cleanReview(), nil
			}, passingChecks)
			var task *Task
			if role == Execute || role == Review {
				task = &fixtureWorkload().Tasks[0]
			}
			call := controller.assignment(role, task, controller.Configuration.Executor)
			var observed []model.AgentCallActivity
			controller.CallActivity = func(activity model.AgentCallActivity) {
				last := journal.entries[len(journal.entries)-1]
				wantPhase := "call_opened"
				if activity.Status == model.AgentCallActivityCompleted {
					wantPhase = "call_completed"
				}
				if last.Phase != wantPhase || activity.Role != string(role) || activity.CallID != journal.entries[0].ID || task == nil && activity.TaskID != "" || task != nil && activity.TaskID != task.ID {
					t.Fatalf("call activity preceded or changed its journal binding: activity=%+v receipt=%+v", activity, last)
				}
				observed = append(observed, activity)
			}
			if _, err := controller.invoke(context.Background(), call, 1); err != nil {
				t.Fatal(err)
			}
			if len(observed) != 2 || observed[0].Status != model.AgentCallActivityStarted || observed[1].Status != model.AgentCallActivityCompleted {
				t.Fatalf("durable native handoff invisible: %+v", observed)
			}
		})
	}
}

func TestControllerCallActivityPublishesOnlyPersistedFailures(t *testing.T) {
	providerFailure := errors.New("provider stopped")
	for _, rejectedPhase := range []string{"", "call_opened", "call_failed", "call_completed"} {
		t.Run(rejectedPhase, func(t *testing.T) {
			controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) {
				if rejectedPhase != "call_completed" {
					return nil, providerFailure
				}
				return ExecutionResult{Questions: []string{}}, nil
			}, passingChecks)
			store := &callActivityRejectingJournal{memoryJournal: journal, rejectedPhase: rejectedPhase}
			controller.Evidence = store
			var observed []model.AgentCallActivity
			controller.CallActivity = func(activity model.AgentCallActivity) {
				last := journal.entries[len(journal.entries)-1]
				if activity.Status == model.AgentCallActivityFailed && last.Phase != "call_failed" {
					t.Fatal("failure published before persistence")
				}
				observed = append(observed, activity)
			}
			call := controller.assignment(Execute, &fixtureWorkload().Tasks[0], controller.Configuration.Executor)
			if _, err := controller.invoke(context.Background(), call, 1); err == nil {
				t.Fatal("provider or journal failure was hidden")
			}
			want := 1
			if rejectedPhase == "" {
				want = 2
			} else if rejectedPhase == "call_opened" {
				want = 0
			}
			if len(observed) != want || want == 2 && observed[1].Status != model.AgentCallActivityFailed {
				t.Fatalf("missing or unpersisted handoff: observations=%+v rejectedPhase=%s", observed, rejectedPhase)
			}
		})
	}
}

func TestControllerCallActivityRecordsRejectedNativeCompletionAsFailed(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) {
		return ExecutionResult{Questions: []string{}}, nil
	}, passingChecks)
	call := controller.assignment(Execute, &fixtureWorkload().Tasks[0], controller.Configuration.Executor)
	provider := controller.Providers[call.Route.Provider]
	controller.Providers[call.Route.Provider] = providerFunc(func(ctx context.Context, assignment Assignment, writer FileWriter, event func(NativeEvent) error) (Result, error) {
		result, err := provider.Run(ctx, assignment, writer, event)
		result.Completed = false
		return result, err
	})
	var observed []model.AgentCallActivity
	controller.CallActivity = func(activity model.AgentCallActivity) { observed = append(observed, activity) }
	if _, err := controller.invoke(context.Background(), call, 1); err == nil {
		t.Fatal("incomplete native evidence was accepted")
	}
	if len(observed) != 2 || observed[1].Status != model.AgentCallActivityFailed || journal.entries[len(journal.entries)-1].Phase != "call_failed" {
		t.Fatalf("rejected completion left an apparently running call: %+v", observed)
	}
}

type callActivityRejectingJournal struct {
	*memoryJournal
	rejectedPhase string
}

func (journal *callActivityRejectingJournal) Append(ctx context.Context, entry Entry) (int64, error) {
	if entry.Phase == journal.rejectedPhase {
		return 0, errors.New("journal write refused")
	}
	return journal.memoryJournal.Append(ctx, entry)
}
