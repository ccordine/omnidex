package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationReleasesSatisfiedObligationFromPriorGeneration(t *testing.T) {
	t.Parallel()
	observation := mappingTestObservation(t, "")
	evidence := observation.EvidenceRef()
	oldGoal := attentionTestGoal(t, "old.condition", "old-target")
	oldCheck := cognition.CompletionCheckRef{ID: "check.old", Version: "1.0.0", SHA256: mappingTestDigest}
	graph, err := cognition.NewObligationGraph(1, "obligation-old", []cognition.ObligationSpec{{
		ID: "obligation-old", Desired: oldGoal,
		SupportingRefs: []cognition.EvidenceRef{evidence}, CompletionCheck: oldCheck,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition("obligation-old", 1, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		"obligation-old", oldCheck, mappingTestRevision(), cognition.CompletionSatisfied,
		[]cognition.EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Satisfy("obligation-old", 1, completion); err != nil {
		t.Fatal(err)
	}
	newGoal := attentionTestGoal(t, "new.condition", "new-target")
	newCheck := cognition.CompletionCheckRef{ID: "check.new", Version: "1.0.0", SHA256: mappingTestDigest}
	if err := graph.Cutover(2, "obligation-new", []cognition.ObligationSpec{{
		ID: "obligation-new", Desired: newGoal,
		SupportingRefs: []cognition.EvidenceRef{evidence}, CompletionCheck: newCheck,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(2); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition("obligation-new", 2, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	current, ok := graph.Obligation("obligation-new")
	if !ok {
		t.Fatal("new obligation missing")
	}
	schema := mappingTestSchema(t)
	catalog, err := cognition.NewActionCatalog("catalog.cutover", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		newGoal, mappingTestRevision(), current, catalog,
		cognition.AttemptRef{JobID: 41, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-41"},
		cognition.ContextProjectionRef{
			ID: "projection-cutover", SHA256: mappingTestDigest, WorkingSetID: "working-set-cutover",
			WorkingSetVersion: 1, RendererVersion: "omnidex.context-material-json.v1",
		},
		cognition.RuntimeBudget{
			RemainingPolicyCalls: 1, MaxInputBytes: 64 * 1024, MaxInputTokens: 16 * 1024,
			MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
			MaxEvidenceRefs: 4, MaxActionArguments: 4,
			MaxLedgerProposals: 4, MaxAttentionRequests: 4, MaxExpectedEffectBytes: 512,
		},
		[]cognition.EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 10, MaxPinnedBytes: 96 * 1024,
	})
	old, ok := graph.Obligation("obligation-old")
	if !ok || old.Status != cognition.ObligationSatisfied {
		t.Fatalf("old obligation = %#v", old)
	}
	oldCandidate, err := obligationCandidate(old, workingset.RoleDependency, 90)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Acquire(workingset.AcquireRequest{
		ID: "satisfied-obligation", Ref: oldCandidate.ref, Role: oldCandidate.role,
		Retention: workingset.RetentionPinned, Scope: set.Scope(), Priority: oldCandidate.priority,
		ByteCost: len(oldCandidate.content), Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: "satisfied-obligation",
			Reason: "Prior satisfied obligation.",
		},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph.Snapshot(), Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: evidence, Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	item, ok := projected.Item("satisfied-obligation")
	if !ok || item.State != workingset.ItemReleased {
		t.Fatalf("satisfied obligation was not released: %#v", item)
	}
}

func attentionTestGoal(t *testing.T, name, arg string) cognition.GoalExpression {
	t.Helper()
	predicate, err := cognition.NewPredicate(cognition.PredicateName(name), []string{arg})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return goal
}
