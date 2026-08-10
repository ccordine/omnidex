package cognitionstate

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationReacquiresExactReleasedItemWithoutNewAcquisition(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	input := ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger,
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	}
	input.WorkingSet = set.Snapshot()
	initial, err := BuildDefaultReconciliation(input)
	if err != nil {
		t.Fatal(err)
	}
	set = applyAttentionPlan(t, set.Snapshot(), initial)
	var evidence workingset.Item
	for _, item := range set.ResidentItems() {
		if item.Role == workingset.RoleEvidence {
			evidence = item
			break
		}
	}
	if evidence.ID == "" || len(evidence.Memberships) == 0 {
		t.Fatalf("missing exact evidence item: %#v", set.ResidentItems())
	}
	for _, membership := range append([]workingset.Membership(nil), evidence.Memberships...) {
		if _, err := set.Release(evidence.ID, membership.Scope, "The prior projection released this evidence."); err != nil {
			t.Fatal(err)
		}
	}
	input.WorkingSet = set.Snapshot()
	plan, err := BuildDefaultReconciliation(input)
	if err != nil {
		t.Fatal(err)
	}
	reacquires, duplicateAcquires := 0, 0
	for _, mutation := range plan.Commands() {
		switch mutation.Descriptor().Kind {
		case workingset.CommandReacquire:
			reacquires++
		case workingset.CommandAcquire:
			if strings.Contains(string(mutation.Descriptor().Raw), string(evidence.ID)) {
				duplicateAcquires++
			}
		}
	}
	if reacquires != 1 || duplicateAcquires != 0 {
		t.Fatalf("reacquires=%d duplicate acquires=%d commands=%#v", reacquires, duplicateAcquires, plan.Commands())
	}
	reconciled := applyAttentionPlan(t, set.Snapshot(), plan)
	reacquired, ok := reconciled.Item(evidence.ID)
	if !ok || reacquired.State != workingset.ItemResident || reacquired.ReacquisitionCount != 1 ||
		reacquired.Acquisition != evidence.Acquisition || reacquired.CreatedTick != evidence.CreatedTick {
		t.Fatalf("reacquired evidence=%#v original=%#v", reacquired, evidence)
	}
}

func TestAcceptedRetainedExactReferenceFailsWhenInvalidated(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	extra, err := cognition.NewObservation(
		"observation-reacquire-invalidated", mappingPriorRevision(), "record", "Optional exact evidence.",
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = attentionSnapshotWithEvidence(t, snapshot, []cognition.EvidenceRef{
		observation.EvidenceRef(), extra.EvidenceRef(),
	})
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	request := cognition.AttentionRequest{
		Operation: cognition.AttentionRetain, TargetRef: extra.EvidenceRef(),
		Scope: cognition.AttentionScopeEpisode, Reason: "Retain this optional exact evidence.",
	}
	input := ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger,
		WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{
			{Ref: observation.EvidenceRef(), Content: observation.Content},
			{Ref: extra.EvidenceRef(), Content: extra.Content},
		},
		Attention: []cognition.AttentionRequest{request},
	}
	initial, err := BuildDefaultReconciliation(input)
	if err != nil {
		t.Fatal(err)
	}
	set = applyAttentionPlan(t, set.Snapshot(), initial)
	exactRef := evidenceLedgerRef(extra.EvidenceRef())
	var item workingset.Item
	for _, candidate := range set.ResidentItems() {
		if taskstate.RefIdentity(candidate.Ref) == taskstate.RefIdentity(exactRef) {
			item = candidate
			break
		}
	}
	if item.ID == "" {
		t.Fatal("optional item was not acquired")
	}
	if _, _, err := set.InvalidateStale(
		item.ID, item.Ref.Version+"-changed", strings.Repeat("f", 64), "The exact source changed.",
	); err != nil {
		t.Fatal(err)
	}
	input.WorkingSet = set.Snapshot()
	input.RequiredAttention = []cognition.AttentionRequest{request}
	input.Attention = nil
	_, err = BuildDefaultReconciliation(input)
	if !errors.Is(err, ErrInvalidReconciliation) {
		t.Fatalf("error=%v, want loud invalidated required retention", err)
	}
}

func TestCognitionAttentionSourceHasNoReleasedReferenceDuplicateAcquirePath(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("attention_commands.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "residentByRef(") {
		t.Fatal("old resident-only lookup can route a released exact reference through duplicate Acquire")
	}
	for _, required := range []string{"appendReacquire", "ItemReleased", "ItemInvalidated", "isAttentionCapacityError"} {
		if !strings.Contains(source, required) {
			t.Fatalf("exact-reference reacquire cutover omitted %q", required)
		}
	}
}
