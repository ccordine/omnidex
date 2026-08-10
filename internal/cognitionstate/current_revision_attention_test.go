package cognitionstate

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestAcceptedFactWaitsUntilItsSourceRevisionHasPassed(t *testing.T) {
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger, err := taskstate.RestoreLedger(attentionTestLedger(t, observation))
	if err != nil {
		t.Fatal(err)
	}
	applyEpistemicCommand(t, ledger, "accepted-fact-scope", taskstate.AddNodeCommand{
		Actor: taskstate.AuthorityCode, ID: taskstate.NodeID(snapshot.CurrentObligation().ID),
		Kind: taskstate.NodeGoal, Title: "Active cognition obligation", Priority: 100,
		Metadata: taskstate.EmptyJSONObject(),
	})
	policyRef := FactAcceptancePolicyRef{
		ID: "fact-policy.clean-desk", Version: "1.0.0", SHA256: mappingTestDigest,
	}
	policy, err := NewFactAcceptancePolicy(policyRef, func([]FactEvidence) (string, error) {
		return "Compact accepted fact.", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := registry.MapAcceptedFact(FactAcceptanceInput{
		Ledger: ledger.MaterializedState(), ScopeNodeID: taskstate.NodeID(snapshot.CurrentObligation().ID),
		EvidenceRefs: []cognition.EvidenceRef{observation.EvidenceRef()}, PolicyID: policyRef.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(mutation.Command()); err != nil {
		t.Fatal(err)
	}
	entry, exists := ledger.Entry(mutation.Command().ID)
	if !exists {
		t.Fatal("accepted fact entry is missing")
	}
	eligible, err := acceptedFactEligibleAfterSourceRevision(entry, snapshot.CurrentRevision())
	if err != nil || eligible {
		t.Fatalf("current-source fact eligibility=%v error=%v", eligible, err)
	}
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph,
		Ledger: ledger.MaterializedState(), WorkingSet: attentionTestWorkingSet(t, ledger.MaterializedState(), workingset.Budget{
			MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
		}).Snapshot(), Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, compact := false, false
	for _, material := range plan.Materials() {
		raw = raw || material.Content == observation.Content
		compact = compact || material.Content == entry.Content
	}
	if !raw || compact {
		t.Fatalf("current clean desk raw=%v compact=%v", raw, compact)
	}
	next, err := cognition.NewWorldRevision(
		snapshot.CurrentRevision().EpisodeID, snapshot.CurrentRevision().Number+1, strings.Repeat("f", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err = acceptedFactEligibleAfterSourceRevision(entry, next)
	if err != nil || !eligible {
		t.Fatalf("later fact eligibility=%v error=%v", eligible, err)
	}
}

func TestCurrentRevisionEvidenceIsRequiredWithoutPinnedHoarding(t *testing.T) {
	snapshot, graph, causal := attentionTestRuntime(t)
	current, err := cognition.NewObservation(
		"observation-current-extra", snapshot.CurrentRevision(), "public_state",
		"Exact current state that is unrelated to prior history.",
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = attentionSnapshotWithEvidence(t, snapshot, []cognition.EvidenceRef{
		causal.EvidenceRef(), current.EvidenceRef(),
	})
	ledger := attentionTestLedger(t, causal)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph,
		Ledger: ledger, WorkingSet: set.Snapshot(), Evidence: []EvidenceMaterial{
			{Ref: causal.EvidenceRef(), Content: causal.Content},
			{Ref: current.EvidenceRef(), Content: current.Content},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	item, exists := projectedItemByRef(projected, evidenceLedgerRef(current.EvidenceRef()))
	if !exists || item.Retention != workingset.RetentionCall || len(item.Memberships) != 1 ||
		item.Memberships[0].Scope.Kind != workingset.ScopeCall {
		t.Fatalf("current observation lifecycle=%+v", item)
	}
	for _, selector := range plan.ContextSpec().Required {
		if selector.Role == workingset.RoleEvidence && selector.MinItems == 2 {
			return
		}
	}
	t.Fatal("every exact current-revision observation was not required")
}

func TestAdmittedObligationRetentionIsRequiredOnNextCleanDesk(t *testing.T) {
	snapshot, graph, causal := attentionTestRuntime(t)
	retained, err := cognition.NewObservation(
		"observation-retained-prior", mappingPriorRevision(), "record", "Prior evidence explicitly retained.",
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = attentionSnapshotWithEvidence(t, snapshot, []cognition.EvidenceRef{
		causal.EvidenceRef(), retained.EvidenceRef(),
	})
	ledger := attentionTestLedger(t, causal)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	request := cognition.AttentionRequest{
		Operation: cognition.AttentionRetain, TargetRef: retained.EvidenceRef(),
		Scope: cognition.AttentionScopeObligation, Reason: "Keep this exact prerequisite for the obligation.",
	}
	first, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger,
		WorkingSet: set.Snapshot(), Evidence: []EvidenceMaterial{
			{Ref: causal.EvidenceRef(), Content: causal.Content},
			{Ref: retained.EvidenceRef(), Content: retained.Content},
		}, Attention: []cognition.AttentionRequest{request},
	})
	if err != nil {
		t.Fatal(err)
	}
	set = applyAttentionPlan(t, set.Snapshot(), first)
	second, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger,
		WorkingSet: set.Snapshot(), Evidence: []EvidenceMaterial{
			{Ref: causal.EvidenceRef(), Content: causal.Content},
			{Ref: retained.EvidenceRef(), Content: retained.Content},
		}, RequiredAttention: []cognition.AttentionRequest{request},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range second.ContextSpec().Required {
		if selector.Role == workingset.RoleEvidence && selector.MinItems == 2 {
			return
		}
	}
	t.Fatal("accepted obligation retention became optional on the next call")
}
