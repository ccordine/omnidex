package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestOrdinaryReviewResponseIsParsedSeparately(t *testing.T) {
	for _, kind := range []ExecutionResponseKind{ExecutionDone, ExecutionQuestion, ExecutionPermission, ExecutionError, "unknown"} {
		t.Run(string(kind), func(t *testing.T) {
			reviews, parses := 0, 0
			const response = "An ordinary review response with **formatting** and no JSON contract."
			controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
				if call.Role == Execute {
					return "Done.", nil
				}
				reviews++
				return response, nil
			}, passingChecks)
			parser := controller.Providers["parser"]
			controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
				if call.Role != ReviewResponse {
					return parser.Run(ctx, call, event)
				}
				parses++
				if call.Response != response || call.Task == nil || call.Task.ID != "task-a" ||
					len(call.ReadFiles) != 0 || call.Objective != nil || call.PlanningLedger != nil || call.Session != nil {
					t.Fatalf("review parser has the wrong boundary: %#v", call)
				}
				if err := event(NativeEvent{Frame: []byte(`{"parser":"review"}`)}); err != nil {
					return Result{}, err
				}
				content, _ := json.Marshal(ReviewResponseResult{Kind: kind, ReviewResult: cleanReview()})
				return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true,
					ThreadID: "parser-thread", TurnID: "parser-turn", Content: content}, nil
			})
			err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0])
			var input *ExecutionInputRequired
			switch kind {
			case ExecutionDone:
				if err != nil || journal.entries[len(journal.entries)-1].Phase != "task_accepted" {
					t.Fatalf("review was not accepted: %v", err)
				}
			case ExecutionQuestion, ExecutionPermission:
				if !errors.As(err, &input) || input.Role != Review || input.Kind != kind || input.Text != response {
					t.Fatalf("review input lost: %#v, %v", input, err)
				}
			default:
				if err == nil || errors.As(err, &input) {
					t.Fatalf("review error was not preserved: %v", err)
				}
			}
			if reviews != 1 || parses != 1 {
				t.Fatalf("review/parser calls: %d/%d", reviews, parses)
			}
		})
	}
}

func TestReviewParserFailureDoesNotRepeatNativeExecutionOrReview(t *testing.T) {
	executions, reviews := 0, 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			executions++
			return "Done.", nil
		}
		reviews++
		return cleanReview(), nil
	}, passingChecks)
	parser := controller.Providers["parser"]
	controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
		if call.Role == ReviewResponse {
			return Result{}, errors.New("review parser unavailable")
		}
		return parser.Run(ctx, call, event)
	})
	task := fixtureWorkload().Tasks[0]
	for range 5 {
		var failure *ParserFailure
		if err := controller.RunTask(context.Background(), task); !errors.As(err, &failure) {
			t.Fatalf("parser outage became a terminal task failure: %v", err)
		}
	}
	controller.Providers["parser"] = parser
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if executions != 1 || reviews != 1 {
		t.Fatalf("parser recovery repeated native work: execution=%d review=%d", executions, reviews)
	}
}

func TestCompletedReviewReceiptCanResumeBeforeAcceptance(t *testing.T) {
	reviews := 0
	controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
		if call.Role == Execute {
			return "Done.", nil
		}
		reviews++
		return cleanReview(), nil
	}, passingChecks)
	task := fixtureWorkload().Tasks[0]
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	// Model a restart after the parsed review was saved but before acceptance.
	journal.entries = journal.entries[:len(journal.entries)-1]
	if err := controller.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 || journal.entries[len(journal.entries)-1].Phase != "task_accepted" {
		t.Fatal("acceptance recovery repeated native review")
	}
}

func TestReviewOwnerReplyReturnsToDisposableReviewNotExecutor(t *testing.T) {
	for _, kind := range []ExecutionResponseKind{ExecutionQuestion, ExecutionPermission} {
		t.Run(string(kind), func(t *testing.T) {
			executions, reviews := 0, 0
			const question = "Which remote target may I inspect?"
			controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
				if call.Role == Execute {
					executions++
					return "Done.", nil
				}
				reviews++
				if reviews == 1 {
					return question, nil
				}
				if call.Session != nil || call.Response != question || call.Reply != "Inspect the approved target." || call.Task.ID != "task-a" {
					t.Fatalf("review reply changed its task boundary: %#v", call)
				}
				return cleanReview(), nil
			}, passingChecks)
			parser := controller.Providers["parser"]
			controller.Providers["parser"] = providerFunc(func(ctx context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
				result, err := parser.Run(ctx, call, event)
				if call.Role == ReviewResponse && call.Response == question {
					result.Content, _ = json.Marshal(ReviewResponseResult{Kind: kind, ReviewResult: ReviewResult{Findings: []Finding{}}})
				}
				return result, err
			})
			task := fixtureWorkload().Tasks[0]
			var input *ExecutionInputRequired
			if err := controller.RunTask(context.Background(), task); !errors.As(err, &input) || input.Role != Review {
				t.Fatalf("missing review question: %v", err)
			}
			_, _ = journal.Append(context.Background(), Entry{Generation: 1, Phase: "execution_reply", TaskID: task.ID, Role: Review, Notes: []string{"reply-op", "Inspect the approved target."}})
			if err := controller.RunTask(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if executions != 1 || reviews != 2 {
				t.Fatalf("review reply repeated execution: %d/%d", executions, reviews)
			}
		})
	}
}

func TestInvalidReviewParsingCannotAcceptOrRepeatNativeReview(t *testing.T) {
	for _, review := range []ReviewResult{
		{Findings: nil, Verification: "checked"},
		{Findings: []Finding{}, Verification: ""},
		{Findings: []Finding{{TaskID: "sibling", Requirement: "A works", Observation: "Defect."}}, Verification: "checked"},
		{Findings: []Finding{{TaskID: "task-a", Requirement: "invented requirement", Observation: "Defect."}}, Verification: "checked"},
	} {
		reviews := 0
		controller, journal := fixtureController(t, fixtureWorkload(), func(call Assignment) (any, error) {
			if call.Role == Execute {
				return "Done.", nil
			}
			reviews++
			return review, nil
		}, passingChecks)
		if err := controller.RunTask(context.Background(), fixtureWorkload().Tasks[0]); err == nil {
			t.Fatal("invalid review accepted")
		}
		if reviews != 1 {
			t.Fatal("invalid parser output repeated native review")
		}
		for _, entry := range journal.entries {
			if entry.Phase == "task_accepted" {
				t.Fatal("invalid review reached ledger acceptance")
			}
		}
	}
}
