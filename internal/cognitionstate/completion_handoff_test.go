package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestCompletionHandoffRetainsOnlyProofForDependentAndClosesCompletedScope(t *testing.T) {
	snapshot, proof := observationRetentionFixture(t)
	unrelated, err := cognition.NewObservation(
		"observation-unrelated", proof.Revision, "public_state", "Unrelated child-only evidence.",
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range []cognition.Observation{proof, unrelated} {
		mutations, buildErr := BuildObservationRetention(set.Snapshot(), "child", observation)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		for _, mutation := range mutations {
			if _, applyErr := set.Apply(mutation.Command()); applyErr != nil {
				t.Fatal(applyErr)
			}
		}
	}
	mutations, err := BuildCompletionEvidenceHandoff(
		set.Snapshot(), "child", []cognition.ObligationID{"parent"},
		[]cognition.EvidenceRef{proof.EvidenceRef()},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range mutations {
		if _, err := set.Apply(mutation.Command()); err != nil {
			t.Fatal(err)
		}
	}
	parent, err := AttentionMembership(
		cognition.AttentionScopeObligation, set.Scope(), "parent", mappingZeroDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	child, err := AttentionMembership(
		cognition.AttentionScopeObligation, set.Scope(), "child", mappingZeroDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range set.Items() {
		if item.Ref.Hash == proof.ContentSHA256 {
			if item.State != workingset.ItemResident || !itemHasExactMembership(item, parent) ||
				itemHasExactMembership(item, child) {
				t.Fatalf("completion proof was not handed off exactly: %+v", item)
			}
		}
		if item.Ref.Hash == unrelated.ContentSHA256 && item.State != workingset.ItemReleased {
			t.Fatalf("unrelated child evidence bled into parent: %+v", item)
		}
	}
	if !set.ScopeClosed(child.Scope) {
		t.Fatal("satisfied child scope remained open")
	}
}

func TestCompletionHandoffWithoutDependentsClosesCompletedAndDecisionScopes(t *testing.T) {
	snapshot, observation := observationRetentionFixture(t)
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	mutations, err := BuildObservationRetention(set.Snapshot(), "independent", observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range mutations {
		if _, err := set.Apply(mutation.Command()); err != nil {
			t.Fatal(err)
		}
	}
	item := set.Items()[0]
	completed, err := AttentionMembership(
		cognition.AttentionScopeObligation, set.Scope(), "independent", mappingZeroDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Retain(item.ID, completed.Scope, completed.Retention); err != nil {
		t.Fatal(err)
	}
	mutations, err = BuildCompletionEvidenceHandoff(
		set.Snapshot(), "independent", []cognition.ObligationID{}, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range mutations {
		if _, err := set.Apply(mutation.Command()); err != nil {
			t.Fatal(err)
		}
	}
	item, _ = set.Item(item.ID)
	if item.State != workingset.ItemReleased || len(item.Memberships) != 0 ||
		!set.ScopeClosed(completed.Scope) {
		t.Fatalf("independent completion left stale attention=%+v closed=%v", item, set.ScopeClosed(completed.Scope))
	}
}
