package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestAdvisoryRetentionScopesRemainDistinct(t *testing.T) {
	t.Parallel()
	snapshot, graph, causal := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, causal)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	type scopedObservation struct {
		scope cognition.AttentionScope
		item  cognition.Observation
	}
	values := make([]scopedObservation, 0, 3)
	refs := []cognition.EvidenceRef{causal.EvidenceRef()}
	materials := []EvidenceMaterial{{Ref: causal.EvidenceRef(), Content: causal.Content}}
	for _, scope := range []cognition.AttentionScope{
		cognition.AttentionScopeDecision, cognition.AttentionScopeObligation, cognition.AttentionScopeEpisode,
	} {
		observation, err := cognition.NewObservation(
			cognition.ObservationID("observation-"+string(scope)), mappingPriorRevision(), "record",
			"Exact non-causal evidence retained for "+string(scope)+" scope.",
		)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, scopedObservation{scope: scope, item: observation})
		refs = append(refs, observation.EvidenceRef())
		materials = append(materials, EvidenceMaterial{Ref: observation.EvidenceRef(), Content: observation.Content})
	}
	snapshot = attentionSnapshotWithEvidence(t, snapshot, refs)
	requests := make([]cognition.AttentionRequest, 0, len(values))
	for _, value := range values {
		requests = append(requests, cognition.AttentionRequest{
			Operation: cognition.AttentionRetain, TargetRef: value.item.EvidenceRef(), Scope: value.scope,
			Reason: "Retain this evidence only for its explicit bounded scope.",
		})
	}
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph,
		Ledger: ledger, WorkingSet: set.Snapshot(), Evidence: materials, Attention: requests,
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	for _, value := range values {
		item, exists := projectedItemByRef(projected, evidenceLedgerRef(value.item.EvidenceRef()))
		if !exists || len(item.Memberships) != 1 {
			t.Fatalf("scope %s item = %#v", value.scope, item)
		}
		membership := item.Memberships[0]
		switch value.scope {
		case cognition.AttentionScopeDecision:
			if membership.Retention != workingset.RetentionCall || membership.Scope.Kind != workingset.ScopeCall {
				t.Fatalf("decision membership = %#v", membership)
			}
		case cognition.AttentionScopeObligation:
			if membership.Retention != workingset.RetentionTask || membership.Scope.Kind != workingset.ScopeTask ||
				membership.Scope.ID != workingset.ScopeID("obligation-obligation-41") {
				t.Fatalf("obligation membership = %#v", membership)
			}
		case cognition.AttentionScopeEpisode:
			if membership.Retention != workingset.RetentionJob || membership.Scope != projected.Scope() {
				t.Fatalf("episode membership = %#v root=%#v", membership, projected.Scope())
			}
		}
	}
}

func TestAttentionMembershipApplicabilityExcludesPriorDecisionAndSiblingScopes(t *testing.T) {
	t.Parallel()
	root := workingset.Scope{Kind: workingset.ScopeJob, ID: "job-41"}
	decision, err := AttentionMembership(
		cognition.AttentionScopeDecision, root, "obligation-41", mappingTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := AttentionMembership(
		cognition.AttentionScopeObligation, root, "obligation-41", mappingTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := AttentionMembership(
		cognition.AttentionScopeObligation, root, "obligation-ready", mappingTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := AttentionMembership(
		cognition.AttentionScopeEpisode, root, "obligation-41", mappingTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if AttentionMembershipApplies(decision, root, "obligation-41") ||
		!AttentionMembershipApplies(current, root, "obligation-41") ||
		AttentionMembershipApplies(sibling, root, "obligation-41") ||
		!AttentionMembershipApplies(episode, root, "obligation-41") {
		t.Fatalf("unexpected membership applicability: decision=%#v current=%#v sibling=%#v episode=%#v",
			decision, current, sibling, episode)
	}
}

func projectedItemByRef(set *workingset.Set, ref taskstate.Ref) (workingset.Item, bool) {
	for _, item := range set.ResidentItems() {
		if taskstate.RefIdentity(item.Ref) == taskstate.RefIdentity(ref) && item.Ref.Hash == ref.Hash {
			return item, true
		}
	}
	return workingset.Item{}, false
}
