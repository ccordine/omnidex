package codingagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTaskReviewCorrectionsContinueUntilFulfilledOrCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executions, reviews := 0, 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions++
			return "Done.", nil
		}
		reviews++
		if reviews == 7 {
			return cleanReview(), nil
		}
		return ReviewResult{Findings: []Finding{{TaskID: "task-a", Requirement: "A works", Observation: "Still incomplete."}}, Verification: "Checked."}, nil
	}, passingChecks)
	if err := controller.RunTask(ctx, fixtureWorkload().Tasks[0]); err != nil || executions != 7 || reviews != 7 {
		t.Fatalf("approved task hit an arbitrary cutoff: %d/%d %v", executions, reviews, err)
	}
	controller, _ = fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			cancel()
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	if err := controller.RunTask(ctx, fixtureWorkload().Tasks[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("removed attempt cutoff also removed cancellation: %v", err)
	}
}

func TestNativeAssignmentsConsumeMinificationWithoutChangingTaskAuthority(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	var roles []Role
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Context != "Only this task's compact required context." || !reflect.DeepEqual(*call.Task, task) {
			t.Fatalf("context replaced frozen authority: %#v", call)
		}
		if call.Role == Execute {
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	controller.MinifyContext = func(_ context.Context, call Assignment) (string, error) {
		roles = append(roles, call.Role)
		if call.Objective != nil || call.PlanningLedger != nil {
			t.Fatal("task minification received parent scope")
		}
		return "Only this task's compact required context.", nil
	}
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roles, []Role{Execute, Review}) {
		t.Fatalf("native context or parser source crossed the wrong processing boundary: %v", roles)
	}
	for _, entry := range journal.entries {
		if entry.Phase == "call_opened" && entry.Role.IsNative() && entry.Assignment.Context == "" {
			t.Fatal("compiled prompt was not journaled")
		}
		if entry.Phase == "call_opened" && !entry.Role.IsNative() && entry.Assignment.Context != "" {
			t.Fatal("a response parser received a lossy shared-ledger projection")
		}
	}
}

func TestMultipleReviewAnswersReachBothExecutionAndReviewProgrammatically(t *testing.T) {
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if len(call.Clarifications) != 2 || call.Clarifications[0].Answer != "server-a" || call.Clarifications[1].Answer != "version-b" {
			t.Fatalf("prior owner answers were dropped: %#v", call.Clarifications)
		}
		if call.Role == Execute {
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	for index, text := range []string{"server-a", "version-b"} {
		_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "review_response", Role: Review, TaskID: "task-a", CallID: int64(index + 10), ResponseKind: ExecutionQuestion, Notes: []string{"Which target?"}})
		_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "execution_reply", Role: Review, TaskID: "task-a", CallID: int64(index + 10), Notes: []string{"owner-op", text}})
	}
	if err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0]); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedTaskOnlyProposesContinuationAndWaitsForOwner(t *testing.T) {
	task := fixtureWorkload().Tasks[0]
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role != Plan || call.Task != nil || call.Continuation == nil || call.Continuation.Task.ID != task.ID || call.PlanningLedger == nil {
			t.Fatalf("incomplete task was not returned to planning: %#v", call)
		}
		return "The remaining work needs smaller tasks. May I propose that ledger change?", nil
	}, passingChecks)
	if err := controller.PlanContinuation(context.Background(), task, &IncompleteTaskError{Reason: "context exhausted", ThreadID: "original"}, EngineeringLedger{ID: "ledger"}); err != nil {
		t.Fatal(err)
	}
	last := journal.entries[len(journal.entries)-1]
	if last.Phase != "task_incomplete" || last.TaskID != task.ID || last.Continuation.ThreadID != "original" {
		t.Fatalf("handoff lost durable authority: %#v", last)
	}
	var input *ExecutionInputRequired
	if err := controller.RunTask(context.Background(), task); !errors.As(err, &input) || input.Role != Plan {
		t.Fatalf("handoff automatically executed unapproved continuation: %v", err)
	}
}
