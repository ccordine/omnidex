package cognitionstate

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestDefaultReconciliationKeepsRequiredStateAndBuildsOneBoundedContext(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatalf("build reconciliation: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate reconciliation: %v", err)
	}
	repeated, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil || repeated.Descriptor() != plan.Descriptor() {
		t.Fatalf("reconciliation is not deterministic: descriptor=%#v error=%v", repeated.Descriptor(), err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	roles := make(map[workingset.Role]int)
	for _, item := range projected.ResidentItems() {
		roles[item.Role]++
		if item.Retention != workingset.RetentionPinned {
			t.Fatalf("required item %s is not pinned: %#v", item.ID, item)
		}
	}
	for role, count := range map[workingset.Role]int{
		workingset.RoleGoal: 1, workingset.RoleTask: 1, workingset.RoleInvariant: 1,
		workingset.RoleConstraint: 1, workingset.RoleDependency: 0,
		workingset.RoleFailure: 1, workingset.RoleEvidence: 1,
	} {
		if roles[role] != count {
			t.Fatalf("role %s count = %d, want %d", role, roles[role], count)
		}
	}
	spec := plan.ContextSpec()
	if spec.Name != DefaultContextSpecName || spec.Version != DefaultContextSpecVersion ||
		spec.MaxAcquisitionRounds != 0 || spec.MaxItems > MaxContextItems || spec.MaxBytes > MaxContextBytes {
		t.Fatalf("context spec = %#v", spec)
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "attention-test", Spec: spec, WorkingSet: projected, Materials: plan.Materials(),
	})
	if err != nil {
		t.Fatalf("build projected context: %v", err)
	}
	if len(projection.Selected) != len(projected.ResidentItems()) || len(projection.Omitted) != 0 {
		t.Fatalf("projection selected=%d omitted=%d resident=%d", len(projection.Selected), len(projection.Omitted), len(projected.ResidentItems()))
	}
}

func TestDefaultReconciliationReleasesInactiveAndStaleManagedState(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	for index, request := range []workingset.AcquireRequest{
		{
			ID: "stale-revision", Ref: taskstate.Ref{
				URI: "cognition:episode/episode-41/revision", Version: "2",
				Hash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Relation: taskstate.RefSource,
			}, Role: workingset.RoleInvariant, Retention: workingset.RetentionPinned,
			Scope: set.Scope(), Priority: 100, ByteCost: 16,
			Acquisition: workingset.Acquisition{Provider: workingset.ProviderTaskState, OperationID: "old-revision", Reason: "Prior revision."},
		},
		{
			ID: "completed-obligation", Ref: taskstate.Ref{
				URI: "cognition:obligation/obligation-complete", Version: "1", Hash: mappingTestDigest,
				Relation: taskstate.RefSource,
			}, Role: workingset.RoleDependency, Retention: workingset.RetentionPinned,
			Scope: set.Scope(), Priority: 90, ByteCost: 16,
			Acquisition: workingset.Acquisition{Provider: workingset.ProviderTaskState, OperationID: "completed-obligation", Reason: "Completed state."},
		},
	} {
		if _, err := set.Acquire(request); err != nil {
			t.Fatalf("seed item %d: %v", index, err)
		}
	}
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	for _, id := range []workingset.ItemID{"stale-revision", "completed-obligation"} {
		item, ok := projected.Item(id)
		if !ok || item.State != workingset.ItemReleased {
			t.Fatalf("item %s was not released: %#v", id, item)
		}
	}
}

func TestModelAttentionIsAdvisoryAndCannotReleaseCausalEvidence(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
		Attention: []cognition.AttentionRequest{{
			Operation: cognition.AttentionRelease, TargetRef: observation.EvidenceRef(),
			Scope: cognition.AttentionScopeObligation, Reason: "The policy proposes releasing this evidence.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := plan.AdvisoryOutcomes()
	if len(outcomes) != 1 || outcomes[0].Disposition != AdvisoryRejectedProtected {
		t.Fatalf("advisory outcomes = %#v", outcomes)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	if roles := projected.ResidentItems(); len(roles) == 0 {
		t.Fatal("protected causal evidence was released")
	}
}

func TestAdvisoryRetainsAreBoundedAndNeverBecomePinned(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	materials := []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}}
	refs := []cognition.EvidenceRef{observation.EvidenceRef()}
	requests := make([]cognition.AttentionRequest, 0, MaxAdvisoryRetains+1)
	for index := 0; index < MaxAdvisoryRetains+1; index++ {
		extra, err := cognition.NewObservation(
			cognition.ObservationID("observation-advisory-"+string(rune('a'+index))),
			mappingPriorRevision(), "record", "Bounded advisory evidence "+string(rune('A'+index))+".",
		)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, extra.EvidenceRef())
		materials = append(materials, EvidenceMaterial{Ref: extra.EvidenceRef(), Content: extra.Content})
		requests = append(requests, cognition.AttentionRequest{
			Operation: cognition.AttentionRetain, TargetRef: extra.EvidenceRef(),
			Scope: cognition.AttentionScopeEpisode, Reason: "Retain this non-causal evidence within the advisory cap.",
		})
	}
	snapshot = attentionSnapshotWithEvidence(t, snapshot, refs)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: materials, Attention: requests,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := plan.AdvisoryOutcomes()
	accepted, rejected := 0, 0
	for _, outcome := range outcomes {
		switch outcome.Disposition {
		case AdvisoryAccepted:
			accepted++
		case AdvisoryRejectedCapacity:
			rejected++
		}
	}
	if accepted != MaxAdvisoryRetains || rejected != 1 {
		t.Fatalf("advisory accepted=%d rejected=%d outcomes=%#v", accepted, rejected, outcomes)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	evidenceItems, advisoryPinned := 0, 0
	for _, item := range projected.ResidentItems() {
		if item.Role != workingset.RoleEvidence {
			continue
		}
		evidenceItems++
		if item.Retention == workingset.RetentionPinned && item.Ref != evidenceLedgerRef(observation.EvidenceRef()) {
			advisoryPinned++
		}
	}
	if evidenceItems != MaxAdvisoryRetains+1 || advisoryPinned != 0 {
		t.Fatalf("evidence items=%d advisory pinned=%d", evidenceItems, advisoryPinned)
	}
}

func TestDefaultReconciliationFailsWhenCausalEvidenceMaterialIsMissing(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	_, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
	})
	if !errors.Is(err, ErrMissingMaterial) {
		t.Fatalf("error = %v, want ErrMissingMaterial", err)
	}
}

func TestProjectionStateDoesNotRequireOrInheritAContextProjection(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	state := attentionProjectionState(t, snapshot)
	changedRef := snapshot.ContextProjection()
	changedRef.ID = cognition.ContextProjectionID("context_projection_" + strings.Repeat("7", 64))
	changedRef.SHA256 = strings.Repeat("8", 64)
	changedSnapshot, err := cognition.NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), changedRef, snapshot.Budget(), snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	changedState := attentionProjectionState(t, changedSnapshot)
	if changedState.SHA256() != state.SHA256() {
		t.Fatal("projection-independent state inherited a Context Projection identity")
	}
	ledger := attentionTestLedger(t, observation)
	set := attentionTestWorkingSet(t, ledger, workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 8, MaxPinnedBytes: 96 * 1024,
	})
	input := ReconciliationInput{
		State: state, ObligationGraph: graph, Ledger: ledger, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	}
	first, err := BuildDefaultReconciliation(input)
	if err != nil {
		t.Fatal(err)
	}
	input.State = changedState
	second, err := BuildDefaultReconciliation(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Descriptor() != second.Descriptor() {
		t.Fatal("attention plan changed with an unrelated Context Projection")
	}
}
