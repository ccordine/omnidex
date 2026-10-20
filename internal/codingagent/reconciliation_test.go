package codingagent

import (
	"context"
	"fmt"
	"testing"
)

type interruptedAcceptanceStore struct {
	durableTaskEvidence
	rejected bool
}

func (store *interruptedAcceptanceStore) Append(ctx context.Context, entry Entry) (int64, error) {
	if entry.Phase == "task_accepted" && !store.rejected {
		store.rejected = true
		return 0, fmt.Errorf("synthetic reconciliation interruption")
	}
	return store.durableTaskEvidence.Append(ctx, entry)
}
func TestReconcileParsedReviewWithoutRepeatingProviderCalls(t *testing.T) {
	task := Task{ID: "selected-task", Goal: "Complete the exact retained task."}
	store := &interruptedAcceptanceStore{durableTaskEvidence: durableTaskEvidence{entries: []Entry{
		{Phase: "execution_response", TaskID: task.ID, CallID: 11, Notes: []string{"Which format?"}, ResponseKind: ExecutionQuestion},
		{Phase: "execution_reply", TaskID: task.ID, CallID: 11, Notes: []string{"reply-id", "CSV"}},
	}}}
	provider := &durableTaskProvider{t: t, task: task}
	route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
	controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
	if err := controller.RunTask(t.Context(), task); err == nil {
		t.Fatal("missing reconciliation interruption")
	}
	if provider.calls != 4 {
		t.Fatalf("initial provider calls=%d", provider.calls)
	}
	if err := controller.RunTask(t.Context(), task); err != nil || provider.calls != 4 {
		t.Fatalf("reconciliation repeated completed provider work calls=%d err=%v", provider.calls, err)
	}
	if err := controller.RunTask(t.Context(), task); err != nil || provider.calls != 4 {
		t.Fatalf("duplicate retry repeated accepted work calls=%d err=%v", provider.calls, err)
	}
}
