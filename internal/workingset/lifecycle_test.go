package workingset

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestSharedItemSurvivesUntilItsLastScopeCloses(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 4, MaxBytes: 128, MaxPinnedItems: 1, MaxPinnedBytes: 32})
	first := Scope{Kind: ScopeTask, ID: "task-1"}
	second := Scope{Kind: ScopeTask, ID: "task-2"}

	result, err := set.Acquire(AcquireRequest{
		ID: "item-1", Ref: testRef("repo://snapshot-1/symbol/alpha", "a"),
		Role: RoleRepositoryEvidence, Retention: RetentionTask, Scope: first,
		Priority: 80, ByteCost: 24, Acquisition: testAcquisition("query-1"),
	})
	if err != nil {
		t.Fatalf("acquire item: %v", err)
	}
	if result.Item.State != ItemResident || len(result.Evicted) != 0 {
		t.Fatalf("unexpected acquisition result: %#v", result)
	}
	if _, err := set.Retain("item-1", second, RetentionTask); err != nil {
		t.Fatalf("retain shared item: %v", err)
	}

	firstClose, err := set.CloseScope(first, "First task completed.")
	if err != nil {
		t.Fatalf("close first scope: %v", err)
	}
	if len(firstClose.Released) != 0 || len(firstClose.Updated) != 1 {
		t.Fatalf("shared item should remain resident: %#v", firstClose)
	}
	resident, ok := set.Item("item-1")
	if !ok || resident.State != ItemResident || len(resident.Memberships) != 1 || resident.Memberships[0].Scope != second {
		t.Fatalf("unexpected shared item after first close: %#v", resident)
	}

	secondClose, err := set.CloseScope(second, "Second task completed.")
	if err != nil {
		t.Fatalf("close second scope: %v", err)
	}
	if len(secondClose.Released) != 1 || secondClose.Released[0].State != ItemReleased {
		t.Fatalf("last membership should release item: %#v", secondClose)
	}
	if usage := set.Usage(); usage.ResidentItems != 0 || usage.ResidentBytes != 0 {
		t.Fatalf("released item still consumes resident budget: %#v", usage)
	}
	all := set.Items()
	if len(all) != 1 || all[0].State != ItemReleased || all[0].DispositionReason != "Second task completed." {
		t.Fatalf("release must preserve a terminal history record: %#v", all)
	}
}

func TestTouchAndExplicitReleaseAreDeterministic(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32, MaxPinnedItems: 1, MaxPinnedBytes: 16})
	scope := Scope{Kind: ScopeCall, ID: "call-1"}
	_, err := set.Acquire(AcquireRequest{
		ID: "item-1", Ref: testRef("evidence://job/7/1", "b"), Role: RoleEvidence,
		Retention: RetentionCall, Scope: scope, Priority: 50, ByteCost: 8,
		Acquisition: testAcquisition("query-2"),
	})
	if err != nil {
		t.Fatalf("acquire item: %v", err)
	}
	before := set.Version()
	touched, err := set.Touch("item-1")
	if err != nil {
		t.Fatalf("touch item: %v", err)
	}
	if touched.UseCount != 1 || touched.LastUsedTick <= touched.CreatedTick || set.Version() != before+1 {
		t.Fatalf("touch did not advance deterministic counters: %#v version=%d", touched, set.Version())
	}
	released, err := set.Release("item-1", scope, "Evidence no longer needed.")
	if err != nil {
		t.Fatalf("release item: %v", err)
	}
	if released.State != ItemReleased || released.ReleasedTick <= touched.LastUsedTick {
		t.Fatalf("release did not produce terminal state: %#v", released)
	}
	if _, err := set.Touch("item-1"); !errors.Is(err, ErrItemNotResident) {
		t.Fatalf("touching released item error = %v, want ErrItemNotResident", err)
	}
}

func TestPinnedMembershipSurvivesScopeClosureUntilExplicitRelease(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32, MaxPinnedItems: 1, MaxPinnedBytes: 16})
	scope := Scope{Kind: ScopeObjective, ID: "objective-1"}
	_, err := set.Acquire(AcquireRequest{
		ID: "pinned-1", Ref: testRef("task://job/7/entry/decision-1", "c"), Role: RoleDecision,
		Retention: RetentionPinned, Scope: scope, Priority: 100, ByteCost: 10,
		Acquisition: testAcquisition("decision-1"),
	})
	if err != nil {
		t.Fatalf("acquire pinned item: %v", err)
	}
	closed, err := set.CloseScope(scope, "Objective completed.")
	if err != nil {
		t.Fatalf("close objective: %v", err)
	}
	if len(closed.Released) != 0 || len(closed.Updated) != 0 {
		t.Fatalf("scope closure must not release pinned membership: %#v", closed)
	}
	item, _ := set.Item("pinned-1")
	if item.State != ItemResident || item.Retention != RetentionPinned {
		t.Fatalf("pinned item did not remain resident: %#v", item)
	}
	if _, err := set.Acquire(AcquireRequest{
		ID: "late", Ref: testRef("task://job/7/entry/late", "d"), Role: RoleEvidence,
		Retention: RetentionObjective, Scope: scope, Priority: 1, ByteCost: 1,
		Acquisition: testAcquisition("late"),
	}); !errors.Is(err, ErrScopeClosed) {
		t.Fatalf("acquire into closed scope error = %v, want ErrScopeClosed", err)
	}
	if _, err := set.Release("pinned-1", scope, "Pin explicitly released."); err != nil {
		t.Fatalf("explicitly release pin: %v", err)
	}
}

func newTestSet(t *testing.T, budget Budget) *Set {
	t.Helper()
	set, err := New(testOwner(t, 7, 1), budget)
	if err != nil {
		t.Fatalf("new working set: %v", err)
	}
	return set
}

func testOwner(t *testing.T, jobID, generation int64) Owner {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: jobID,
		RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	return Owner{LedgerID: ledgerID, JobID: jobID, Generation: generation}
}

func testRef(uri, digestByte string) taskstate.Ref {
	return taskstate.Ref{
		URI: uri, Version: "v1", Hash: repeatDigest(digestByte), Relation: taskstate.RefEvidence,
	}
}

func testAcquisition(operation string) Acquisition {
	return Acquisition{Provider: ProviderRepository, OperationID: operation, Reason: "Required by the active scope."}
}

func repeatDigest(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
