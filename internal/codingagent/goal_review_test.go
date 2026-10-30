package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type goalReviewProvider func(Assignment) (Result, error)

func (provider goalReviewProvider) Run(_ context.Context, call Assignment, _ func(NativeEvent) error) (Result, error) {
	return provider(call)
}

func TestFinalReviewChecksOriginalGoalWithoutCreatingOrReopeningItems(t *testing.T) {
	for _, outcome := range []string{"pass", "finding", "question"} {
		t.Run(outcome, func(t *testing.T) {
			objective := Objective{Instruction: "Deliver the requested application.", Clarifications: []string{"Keep existing user data."}}
			store := &durableTaskEvidence{entries: []Entry{{Phase: "task_accepted", TaskID: "finished-item"}}}
			var roles []Role
			provider := goalReviewProvider(func(call Assignment) (Result, error) {
				roles = append(roles, call.Role)
				if call.Task != nil || !reflect.DeepEqual(call.Objective, &objective) || call.Session != nil {
					t.Fatalf("final review lost original scope or reused an item session: %+v", call)
				}
				if err := call.ValidateTaskContext(); err != nil {
					t.Fatal(err)
				}
				result := Result{Completed: true, ThreadID: "goal-review", TurnID: "goal-turn"}
				switch call.Role {
				case Review:
					result.Content = json.RawMessage(`"Original goal checked in the actual software."`)
				case ReviewResponse:
					parsed := `{"kind":"done","findings":[],"verification":"Goal checked."}`
					if outcome == "finding" {
						parsed = `{"kind":"done","findings":[{"file":"","observation":"The requested workflow is incomplete."}],"verification":"Goal checked."}`
					} else if outcome == "question" {
						parsed = `{"kind":"question","findings":[],"verification":"An essential answer is needed."}`
					}
					var err error
					result.Content, err = MapParserResponse(call, json.RawMessage(parsed))
					if err != nil {
						t.Fatal(err)
					}
				default:
					t.Fatalf("final review launched an extra stage: %s", call.Role)
				}
				return result, nil
			})
			route := Route{Provider: "fixture", Model: "fixture"}
			controller := Controller{JobID: 1, Generation: 1, Evidence: store, Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Reviewer: route, LedgerParser: route}}
			err := controller.ReviewGoal(t.Context(), objective)
			switch outcome {
			case "pass":
				if err != nil {
					t.Fatal(err)
				}
			case "finding":
				var incomplete *IncompleteTaskError
				if !errors.As(err, &incomplete) {
					t.Fatalf("unfulfilled original goal was accepted: %v", err)
				}
			case "question":
				var input *ExecutionInputRequired
				if !errors.As(err, &input) || input.TaskID != "" || input.Role != Review {
					t.Fatalf("goal review lost its owner-input route: %v", err)
				}
			}
			if !reflect.DeepEqual(roles, []Role{Review, ReviewResponse}) {
				t.Fatalf("unexpected stages: %v", roles)
			}
			for _, entry := range store.entries[1:] {
				if entry.TaskID != "" || entry.Phase == "task_accepted" || entry.Phase == "task_executing" || entry.Phase == "task_reviewed" {
					t.Fatalf("final review changed the item queue: %+v", entry)
				}
			}
			// Reconstruct from persisted results without rerunning the reviewer/parser.
			_ = controller.ReviewGoal(t.Context(), objective)
			if len(roles) != 2 {
				t.Fatalf("replayed completed goal review: %v", roles)
			}
		})
	}
}
