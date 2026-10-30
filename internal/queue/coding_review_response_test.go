package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func appendCodingReviewParserFixture(t *testing.T, store *CodingAgentEvidenceStore, authority model.StepAttemptAuthority, task *codingagent.Task, route codingagent.Route, reviewCall int64) {
	t.Helper()
	ctx := context.Background()
	call := &codingagent.Assignment{JobID: authority.JobID, Generation: authority.Generation, Role: codingagent.ReviewResponse,
		Task: task, Route: route, Response: "Checked the exact task; no defects found."}
	callID, err := store.Append(ctx, codingagent.Entry{Generation: authority.Generation, Phase: "call_opened", TaskID: task.ID, Role: call.Role, Assignment: call})
	if err != nil {
		t.Fatal(err)
	}
	parsed := codingagent.ReviewResponseResult{Kind: codingagent.ExecutionDone, ReviewResult: codingagent.ReviewResult{Findings: []codingagent.Finding{}, Verification: "checked exact task"}}
	content, _ := json.Marshal(parsed)
	for _, entry := range []codingagent.Entry{
		{Phase: "native_event", Frame: []byte(`{"parser":"review"}`)},
		{Phase: "call_completed", Result: &codingagent.Result{CallID: callID, Provider: route.Provider, Model: route.Model,
			ThreadID: "review-parser-" + task.ID, TurnID: "parser-turn-" + task.ID, Completed: true, Content: content}},
	} {
		entry.Generation, entry.CallID, entry.TaskID, entry.Role = authority.Generation, callID, task.ID, call.Role
		if _, err := store.Append(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Append(ctx, codingagent.Entry{Generation: authority.Generation, Phase: "review_response", TaskID: task.ID,
		CallID: reviewCall, ParserCallID: callID, Role: codingagent.Review, ResponseKind: codingagent.ExecutionDone, ReviewResponse: &parsed,
		Notes: []string{call.Response}}); err != nil {
		t.Fatal(err)
	}
}
