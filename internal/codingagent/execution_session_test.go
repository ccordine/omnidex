package codingagent

import (
	"context"
	"errors"
	"testing"
)

type lostExecutionStream struct{}

func (lostExecutionStream) Error() string   { return "execution stream lost after dispatch" }
func (lostExecutionStream) Retryable() bool { return true }

func TestLostNativeTaskStreamIsNeverBlindlyRetried(t *testing.T) {
	for _, role := range []Role{Execute, Review} {
		t.Run(string(role), func(t *testing.T) {
			calls := 0
			controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) {
				calls++
				return nil, lostExecutionStream{}
			}, passingChecks)
			task := fixtureWorkload().Tasks[0]
			call := controller.assignment(role, &task, controller.Configuration.Executor)
			if _, err := controller.invoke(context.Background(), call, 1); err == nil || calls != 1 {
				t.Fatalf("native task call retried: calls=%d error=%v", calls, err)
			}
			if journal.entries[len(journal.entries)-1].Phase != "call_failed" {
				t.Fatal("lost native call was not recorded")
			}
		})
	}
}

func TestReclaimedTaskRetainsItsOriginalDispatchedNativeThread(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			if call.Session == nil || call.Session.ThreadID != "original-thread" || call.Session.TurnID != "original-turn" || call.Session.State != "running" {
				t.Fatalf("recovery lost original native execution: %#v", call.Session)
			}
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	task := fixtureWorkload().Tasks[0]
	_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "task_executing", TaskID: task.ID, Round: 1})
	_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "execution_session", TaskID: task.ID, Round: 1,
		Session: &ExecutionSession{ThreadID: "original-thread", TurnID: "original-turn", State: "running"}})
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}

func TestNativeInputWithoutOwnerHandlerCannotBeAutoApproved(t *testing.T) {
	controller, _ := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) { return nil, nil }, passingChecks)
	controller.Providers["executor"] = providerFunc(func(_ context.Context, _ Assignment, event func(NativeEvent) error) (Result, error) {
		if err := event(NativeEvent{Frame: []byte(`{"native":"request"}`)}); err != nil {
			return Result{}, err
		}
		input := NativeInput{Request: NativeInputRequest{ThreadID: "thread", Method: "approval"}}
		if err := event(NativeEvent{Input: &input}); err != nil {
			return Result{}, err
		}
		return Result{}, errors.New("unexpected approval")
	})
	if err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0]); err == nil {
		t.Fatal("native permission request bypassed the owner")
	}
}
