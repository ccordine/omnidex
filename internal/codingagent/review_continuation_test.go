package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestPersistedIncompleteReviewContinuesExecutionWithoutReplayingReview(t *testing.T) {
	for _, fixture := range []struct{ goal, response, finding string }{
		{"Write the requested text report.", "The report is incomplete: the output lacks the requested units and its runtime checks have not been performed.", "The output lacks the requested units and its runtime checks have not been performed."},
		{"Export the requested records.", "Task remains incomplete. The export omitted the final record and the required round-trip verification is still pending.", ""},
	} {
		t.Run(fixture.goal, func(t *testing.T) {
			task := Task{ID: "retained-task", Goal: fixture.goal, WriteFiles: []string{"output.txt"}}
			content, _ := json.Marshal(fixture.response)
			retained := Result{CallID: 3, Provider: "fixture", Model: "fixture", Completed: true,
				ThreadID: "old-review", TurnID: "old-turn", Content: content}
			findings := []Finding{}
			expected := Finding{TaskID: task.ID, Observation: fixture.response}
			if fixture.finding != "" {
				expected.File, expected.Observation = "output.txt", fixture.finding
				findings = append(findings, expected)
			}
			store := &durableTaskEvidence{entries: []Entry{
				{ID: 1, Phase: "task_accepted", TaskID: "already-complete", Round: 1},
				{ID: 2, Phase: "execution_response", TaskID: task.ID, CallID: 2, Round: 1, ResponseKind: ExecutionDone, Notes: []string{"Implementation ready for review."}},
				{ID: 3, Phase: "call_completed", TaskID: task.ID, CallID: 3, Role: Review, Round: 1, Result: &retained},
				{ID: 4, Phase: "review_response", TaskID: task.ID, CallID: 3, Role: Review, Round: 1, ResponseKind: ExecutionError,
					ReviewResponse: &ReviewResponseResult{Kind: ExecutionError, ReviewResult: ReviewResult{Findings: findings, Verification: fixture.response}}, Notes: []string{fixture.response}},
				{ID: 5, Phase: "task_interrupted", TaskID: task.ID, Continuation: &TaskContinuation{Task: task, Reason: "Review is incomplete: " + fixture.response, ThreadID: retained.ThreadID, TurnID: retained.TurnID}},
			}}
			provider := &reviewScopeRoutingProvider{executionScopeProvider: executionScopeProvider{t: t, task: task, response: "Corrected and verified.", kind: "done"}, reviews: 1}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{},
				Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			if err := controller.RunTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprint(provider.roles); got != "[execute execution_response review review_response]" {
				t.Fatalf("retained review was replayed instead of corrected: %s", got)
			}
			var corrected bool
			for _, entry := range store.entries[5:] {
				if entry.TaskID != task.ID {
					t.Fatal("continuation changed another task")
				}
				if entry.Phase == "call_opened" && entry.Role == Execute {
					findings := entry.Assignment.Findings
					corrected = len(findings) == 1 && findings[0] == expected
				}
			}
			if !corrected {
				t.Fatal("executor did not receive the exact retained review")
			}
			if err := controller.RunTask(t.Context(), task); err != nil || len(provider.roles) != 4 {
				t.Fatalf("accepted work ran again: %v; roles=%v", err, provider.roles)
			}
		})
	}
}

type failedReviewProvider struct{ failure error }

func (provider failedReviewProvider) Run(context.Context, Assignment, func(NativeEvent) error) (Result, error) {
	return Result{}, provider.failure
}

func TestReviewProviderFailureIsNotConvertedToCorrectionFeedback(t *testing.T) {
	task := Task{ID: "task", Goal: "Verify the requested result."}
	failure := errors.New("review provider unavailable")
	route := Route{Provider: "fixture", Model: "fixture", Effort: "high", ServiceTier: "default"}
	store := &durableTaskEvidence{}
	controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{},
		Providers: map[string]Provider{"fixture": failedReviewProvider{failure}}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
	_, _, err := controller.review(t.Context(), controller.assignment(Review, &task, route), map[string]Task{task.ID: task}, 1, nil)
	if !errors.Is(err, failure) {
		t.Fatalf("provider failure was hidden: %v", err)
	}
	for _, entry := range store.entries {
		if entry.Phase == "task_reviewed" || entry.Phase == "task_accepted" {
			t.Fatal("provider failure became task feedback or acceptance")
		}
	}
}
