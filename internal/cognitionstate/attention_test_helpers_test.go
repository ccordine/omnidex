package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func attentionTestRuntime(t *testing.T) (cognition.RuntimeSnapshot, cognition.ObligationGraphSnapshot, cognition.Observation) {
	t.Helper()
	observation := mappingTestObservation(t, "")
	evidence := observation.EvidenceRef()
	predicate, err := cognition.NewPredicate("goal.condition", []string{"target-41"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{ID: "check.goal", Version: "1.0.0", SHA256: mappingTestDigest}
	root := cognition.ObligationSpec{
		ID: "obligation-41", Desired: goal,
		SupportingRefs: []cognition.EvidenceRef{evidence}, CompletionCheck: check,
	}
	readyPredicate, err := cognition.NewPredicate("ready.condition", []string{"target-42"})
	if err != nil {
		t.Fatal(err)
	}
	readyGoal, err := cognition.NewGoalExpression([]cognition.Predicate{readyPredicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ready := cognition.ObligationSpec{
		ID: "obligation-ready", ParentID: root.ID, Desired: readyGoal,
		CompletionCheck: cognition.CompletionCheckRef{ID: "check.ready", Version: "1.0.0", SHA256: mappingTestDigest},
	}
	graph, err := cognition.NewObligationGraph(2, root.ID, []cognition.ObligationSpec{root, ready})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(2); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 2, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	current, ok := graph.Obligation(root.ID)
	if !ok {
		t.Fatal("active obligation missing")
	}
	schema := mappingTestSchema(t)
	catalog, err := cognition.NewActionCatalog("catalog.attention", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		goal, mappingTestRevision(), current, catalog,
		cognition.AttemptRef{JobID: 41, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-41"},
		cognition.ContextProjectionRef{
			ID: "projection-41", SHA256: mappingTestDigest, WorkingSetID: "working-set-41",
			WorkingSetVersion: 1, RendererVersion: "omnidex.context-material-json.v1",
		},
		cognition.RuntimeBudget{
			RemainingPolicyCalls: 1, MaxInputBytes: 64 * 1024, MaxInputTokens: 16 * 1024,
			MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
			MaxEvidenceRefs: 8, MaxActionArguments: 4,
			MaxLedgerProposals: 4, MaxAttentionRequests: 8, MaxExpectedEffectBytes: 512,
		},
		[]cognition.EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, graph.Snapshot(), observation
}

func attentionProjectionState(t *testing.T, snapshot cognition.RuntimeSnapshot) ProjectionState {
	t.Helper()
	state, err := ProjectionStateFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func attentionTestLedger(t *testing.T, observation cognition.Observation) taskstate.MaterializedState {
	t.Helper()
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	constraintID, err := taskstate.NewCommandID("attention-test", "constraint")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(taskstate.AddEntryCommand{
		CommandID: constraintID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
		ID: "constraint-41", Kind: taskstate.EntryConstraint, Content: "Preserve the public authority boundary.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	}); err != nil {
		t.Fatal(err)
	}
	observationMutation, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: ledger.MaterializedState(), Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(observationMutation.Command()); err != nil {
		t.Fatal(err)
	}
	schema := mappingTestSchema(t)
	action := mappingTestAction(t, schema)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action, mappingTestRevision(),
		"The current public precondition is unresolved.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	failureMutation, err := MapActionFailure(ActionFailureInput{
		Ledger: ledger.MaterializedState(), Binding: ActionBinding{Action: action, Schema: schema},
		ExpectedRevision: mappingTestRevision(), Failure: failure,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(failureMutation.Command()); err != nil {
		t.Fatal(err)
	}
	return ledger.MaterializedState()
}

func attentionTestWorkingSet(t *testing.T, ledger taskstate.MaterializedState, budget workingset.Budget) *workingset.Set {
	t.Helper()
	set, err := workingset.New(workingset.Owner{
		LedgerID: ledger.ID, JobID: ledger.Owner.JobID, Generation: 2,
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func attentionSnapshotWithEvidence(
	t *testing.T,
	snapshot cognition.RuntimeSnapshot,
	refs []cognition.EvidenceRef,
) cognition.RuntimeSnapshot {
	t.Helper()
	budget := snapshot.Budget()
	budget.MaxEvidenceRefs = len(refs)
	updated, err := cognition.NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(),
		budget, refs,
	)
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func applyAttentionPlan(t *testing.T, snapshot workingset.Snapshot, plan ReconciliationPlan) *workingset.Set {
	t.Helper()
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for index, mutation := range plan.Commands() {
		if _, err := set.Apply(mutation.Command()); err != nil {
			t.Fatalf("apply attention command %d: %v", index, err)
		}
	}
	return set
}
