package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestOrdinaryExecutionResponseIsClassifiedSeparately(t *testing.T) {
	for _, kind := range []ExecutionResponseKind{ExecutionDone, ExecutionQuestion, ExecutionPermission, ExecutionError} {
		t.Run(string(kind), func(t *testing.T) {
			executions, reviews := 0, 0
			controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
				if call.Role == Execute {
					executions++
					return "An ordinary final response.", nil
				}
				reviews++
				return cleanReview(), nil
			}, passingChecks)
			parser := controller.Providers["parser"]
			controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
				if call.Role == ReviewResponse {
					return parser.Run(ctx, call, event)
				}
				if call.Role != ExecutionResponse || call.Response != "An ordinary final response." || call.Task != nil ||
					call.Objective != nil || call.PlanningLedger != nil || len(call.ReadFiles) != 0 {
					t.Fatalf("parser received more than the final response: %#v", call)
				}
				if err := event(NativeEvent{Frame: []byte(`{"parser":"native"}`)}); err != nil {
					return Result{}, err
				}
				content, _ := json.Marshal(struct {
					Kind ExecutionResponseKind `json:"kind"`
				}{kind})
				return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "parser-thread", TurnID: "parser-turn", Content: content}, nil
			})
			err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0])
			var input *ExecutionInputRequired
			if kind == ExecutionDone {
				if err != nil || executions != 1 || reviews != 1 {
					t.Fatalf("completion skipped checks/review: %v, %d, %d", err, executions, reviews)
				}
			} else if kind == ExecutionError {
				if err == nil || errors.As(err, &input) || reviews != 0 {
					t.Fatalf("error was not preserved: %v", err)
				}
			} else if !errors.As(err, &input) || input.Kind != kind || input.Text != "An ordinary final response." || reviews != 0 {
				t.Fatalf("owner request was not preserved: %#v, %v", input, err)
			}
		})
	}
}

func TestExecutionReplyContinuesTheSameTaskWithoutReplanning(t *testing.T) {
	executions := 0
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions++
			if executions == 1 {
				return "Which color should I use?", nil
			}
			if call.Session == nil || call.Session.ThreadID != "thread-1" || call.Session.State != "idle" ||
				call.Reply != "Blue." || call.PlanningLedger != nil {
				t.Fatalf("reply lost its native task thread: %#v", call)
			}
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	parser := controller.Providers["parser"]
	controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
		result, err := parser.Run(ctx, call, event)
		if executions == 1 {
			result.Content = []byte(`{"kind":"question"}`)
		}
		return result, err
	})
	task := fixtureWorkload().Tasks[0]
	var input *ExecutionInputRequired
	if err := controller.RunTask(context.Background(), task); !errors.As(err, &input) {
		t.Fatalf("missing question: %v", err)
	}
	_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "execution_reply", TaskID: task.ID, Notes: []string{"reply-operation", "Blue."}})
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if executions != 2 || journal.entries[len(journal.entries)-1].Phase != "task_accepted" {
		t.Fatal("reply did not finish the same task")
	}
}

func TestParserFailureDoesNotRepeatCompletedNativeExecution(t *testing.T) {
	executions := 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions++
			return "Done.", nil
		}
		return cleanReview(), nil
	}, passingChecks)
	parser := controller.Providers["parser"]
	controller.Providers["parser"] = providerFunc(func(context.Context, Assignment, func(NativeEvent) error) (Result, error) {
		return Result{}, errors.New("parser unavailable")
	})
	task := fixtureWorkload().Tasks[0]
	if err := controller.RunTask(context.Background(), task); err == nil {
		t.Fatal("parser failure was hidden")
	}
	controller.Providers["parser"] = parser
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if executions != 1 {
		t.Fatalf("parser retry repeated native execution %d times", executions)
	}
}
