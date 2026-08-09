package workingset

import (
	"errors"
	"testing"
)

func TestAcquireEvictsOnlyLeastRecentlyUsedItemInSameRetentionClass(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 12, MaxPinnedItems: 1, MaxPinnedBytes: 6})
	scope := Scope{Kind: ScopeCall, ID: "call-1"}
	acquireTestItem(t, set, "old", "repo://snapshot/file/old", "1", scope, RetentionCall, 6)
	acquireTestItem(t, set, "recent", "repo://snapshot/file/recent", "2", scope, RetentionCall, 6)
	if _, err := set.Touch("old"); err != nil {
		t.Fatalf("touch old item: %v", err)
	}

	result, err := set.Acquire(testRequest("new", "repo://snapshot/file/new", "3", scope, RetentionCall, 6))
	if err != nil {
		t.Fatalf("acquire with eviction: %v", err)
	}
	if len(result.Evicted) != 1 || result.Evicted[0].ID != "recent" || result.Evicted[0].State != ItemReleased {
		t.Fatalf("wrong deterministic LRU victim: %#v", result.Evicted)
	}
	if item, _ := set.Item("old"); item.State != ItemResident {
		t.Fatalf("recently touched item was evicted: %#v", item)
	}
	if usage := set.Usage(); usage.ResidentItems != 2 || usage.ResidentBytes != 12 {
		t.Fatalf("hard budget not respected: %#v", usage)
	}
}

func TestBudgetFailureDoesNotEvictAcrossRetentionClassesOrPartiallyMutate(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 12, MaxPinnedItems: 1, MaxPinnedBytes: 6})
	call := Scope{Kind: ScopeCall, ID: "call-1"}
	task := Scope{Kind: ScopeTask, ID: "task-1"}
	acquireTestItem(t, set, "call", "repo://snapshot/file/call", "4", call, RetentionCall, 6)
	acquireTestItem(t, set, "task", "repo://snapshot/file/task", "5", task, RetentionTask, 6)
	version := set.Version()

	_, err := set.Acquire(testRequest("large-call", "repo://snapshot/file/large", "6", call, RetentionCall, 10))
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("cross-class overflow error = %v, want ErrBudgetExceeded", err)
	}
	if set.Version() != version {
		t.Fatalf("failed acquisition changed version: got %d want %d", set.Version(), version)
	}
	for _, id := range []ItemID{"call", "task"} {
		item, ok := set.Item(id)
		if !ok || item.State != ItemResident {
			t.Fatalf("failed acquisition partially evicted %s: %#v", id, item)
		}
	}
	if _, exists := set.Item("large-call"); exists {
		t.Fatal("failed acquisition retained its candidate item")
	}
}

func TestPinnedBudgetsAreHardAndPromotionIsAtomic(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 3, MaxBytes: 30, MaxPinnedItems: 1, MaxPinnedBytes: 10})
	objective := Scope{Kind: ScopeObjective, ID: "objective-1"}
	task := Scope{Kind: ScopeTask, ID: "task-1"}
	acquireTestItem(t, set, "pin", "task://job/7/decision/pin", "7", objective, RetentionPinned, 10)

	_, err := set.Acquire(testRequest("pin-2", "task://job/7/decision/pin-2", "8", objective, RetentionPinned, 1))
	if !errors.Is(err, ErrPinnedBudgetExceeded) {
		t.Fatalf("second pin error = %v, want ErrPinnedBudgetExceeded", err)
	}
	acquireTestItem(t, set, "normal", "repo://snapshot/symbol/normal", "9", task, RetentionTask, 5)
	version := set.Version()
	if _, err := set.Retain("normal", task, RetentionPinned); !errors.Is(err, ErrPinnedBudgetExceeded) {
		t.Fatalf("pin promotion error = %v, want ErrPinnedBudgetExceeded", err)
	}
	item, _ := set.Item("normal")
	if item.Retention != RetentionTask || len(item.Memberships) != 1 || set.Version() != version {
		t.Fatalf("failed pin promotion mutated item: %#v version=%d", item, set.Version())
	}
}

func acquireTestItem(t *testing.T, set *Set, id ItemID, uri, digest string, scope Scope, retention Retention, bytes int) {
	t.Helper()
	if _, err := set.Acquire(testRequest(id, uri, digest, scope, retention, bytes)); err != nil {
		t.Fatalf("acquire %s: %v", id, err)
	}
}

func testRequest(id ItemID, uri, digest string, scope Scope, retention Retention, bytes int) AcquireRequest {
	return AcquireRequest{
		ID: id, Ref: testRef(uri, digest), Role: RoleRepositoryEvidence,
		Retention: retention, Scope: scope, Priority: 50, ByteCost: bytes,
		Acquisition: testAcquisition("acquire-" + string(id)),
	}
}
