package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestCognitionReconciliationPreservesResidentEvidenceTrimmedFromModelProjection(t *testing.T) {
	input := cognitionProjectionFitTestInput(t)
	model := input.CompletionEvidence[:1]
	projection := cognition.ContextProjectionRef{
		ID: "projection-attention-trimmed", SHA256: strings.Repeat("f", 64),
		WorkingSetID: "working-set-attention", WorkingSetVersion: input.Set.Version(), RendererVersion: "v1",
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		input.Episode.Goal, input.Episode.CurrentRevision, input.Current,
		input.Episode.ActionCatalog, input.Attempt, projection, input.Budget, model,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared := cognitionruntime.PreparedSnapshot{
		Snapshot: snapshot, CompletionEvidenceRefs: input.CompletionEvidence,
	}
	state, err := cognitionReconciliationState(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.EvidenceRefs(); len(got) != 2 || got[1] != input.CompletionEvidence[1] {
		t.Fatalf("reconciliation evidence=%#v", got)
	}
}

func TestCognitionEvidenceMembershipDoesNotBleedAcrossSiblingTransition(t *testing.T) {
	root := workingset.Scope{Kind: workingset.ScopeJob, ID: "job-41"}
	first := cognition.ObligationID("obligation-first")
	second := cognition.ObligationID("obligation-second")
	membership := cognitionAttentionMembership(t, cognition.AttentionScopeObligation, root, first)
	item := cognitionAttentionEvidenceItem(testCognitionAttentionEvidence(), membership)

	if !cognitionEvidenceMembershipApplies(item, root, first) {
		t.Fatal("evidence did not apply to its exact obligation")
	}
	if cognitionEvidenceMembershipApplies(item, root, second) {
		t.Fatal("evidence from the completed sibling entered the next sibling call")
	}
}

func TestCognitionDecisionScopedEvidenceDoesNotEnterNextSnapshot(t *testing.T) {
	root := workingset.Scope{Kind: workingset.ScopeJob, ID: "job-42"}
	current := cognition.ObligationID("obligation-current")
	membership, err := cognitionstate.AttentionMembership(
		cognition.AttentionScopeDecision, root, current, strings.Repeat("d", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := testCognitionAttentionEvidence()
	item := cognitionAttentionEvidenceItem(ref, membership)
	if cognitionEvidenceMembershipApplies(item, root, current) {
		t.Fatal("decision-scoped evidence entered a later snapshot")
	}
	requests, err := retainedCognitionAttention(
		workingset.Snapshot{Scope: root, Items: []workingset.Item{item}},
		[]cognitionstate.EvidenceMaterial{{Ref: ref}}, current,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("retained decision requests=%v, want none", requests)
	}
}

func TestCognitionEpisodeScopedEvidenceRemainsApplicable(t *testing.T) {
	root := workingset.Scope{Kind: workingset.ScopeJob, ID: "job-43"}
	current := cognition.ObligationID("obligation-current")
	membership := cognitionAttentionMembership(t, cognition.AttentionScopeEpisode, root, current)
	ref := testCognitionAttentionEvidence()
	item := cognitionAttentionEvidenceItem(ref, membership)
	requests, err := retainedCognitionAttention(
		workingset.Snapshot{Scope: root, Items: []workingset.Item{item}},
		[]cognitionstate.EvidenceMaterial{{Ref: ref}}, current,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].Scope != cognition.AttentionScopeEpisode {
		t.Fatalf("retained episode requests=%v", requests)
	}
}

func cognitionAttentionMembership(
	t *testing.T,
	scope cognition.AttentionScope,
	root workingset.Scope,
	obligation cognition.ObligationID,
) workingset.Membership {
	t.Helper()
	membership, err := cognitionstate.AttentionMembership(scope, root, obligation, strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	return membership
}

func cognitionAttentionEvidenceItem(
	ref cognition.EvidenceRef,
	membership workingset.Membership,
) workingset.Item {
	return workingset.Item{
		ID: "evidence-item", Ref: taskstate.Ref{
			URI: cognitionEvidenceTaskRef(ref), Version: "1", Hash: ref.SHA256,
			Relation: taskstate.RefEvidence,
		}, Role: workingset.RoleEvidence, Retention: membership.Retention,
		State: workingset.ItemResident, Memberships: []workingset.Membership{membership},
	}
}

func testCognitionAttentionEvidence() cognition.EvidenceRef {
	return cognition.EvidenceRef{
		ObservationID: "observation-attention", SHA256: strings.Repeat("a", 64),
		Revision: cognition.WorldRevision{
			EpisodeID: "episode-attention", Number: 1, SHA256: strings.Repeat("b", 64),
		},
	}
}
