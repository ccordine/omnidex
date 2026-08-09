package workingset

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestTouchManyIsBoundedAtomicAndOrderIndependent(t *testing.T) {
	t.Parallel()
	newSet := func() *Set {
		set := newTestSet(t, Budget{MaxItems: 3, MaxBytes: 30})
		scope := Scope{Kind: ScopeCall, ID: "call-1"}
		for index, id := range []ItemID{"a", "b", "c"} {
			acquireTestItem(t, set, id, "repo://snapshot/symbol/"+string(id), string(rune('a'+index)), scope, RetentionCall, 5)
		}
		return set
	}
	first, second := newSet(), newSet()
	left, err := first.TouchMany([]ItemID{"c", "a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.TouchMany([]ItemID{"b", "c", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 3 || len(right) != 3 || left[0].ID != "a" || right[0].ID != "a" ||
		!reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatalf("touch results are not deterministic: %+v / %+v", left, right)
	}
	before := first.Snapshot()
	for name, ids := range map[string][]ItemID{
		"empty":     {},
		"duplicate": {"a", "a"},
		"missing":   {"a", "missing"},
		"too many":  make([]ItemID, MaxTouchBatchItems+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := first.TouchMany(ids); err == nil {
				t.Fatal("invalid touch batch succeeded")
			}
			if got := first.Snapshot(); got.Version != before.Version || got.Clock != before.Clock {
				t.Fatal("invalid touch batch mutated counters")
			}
		})
	}
}

func TestRootScopeCloseIsTerminalAndReleasesPins(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 3, MaxBytes: 48, MaxPinnedItems: 1, MaxPinnedBytes: 16})
	root := set.Scope()
	objective := Scope{Kind: ScopeObjective, ID: "objective-1"}
	task := Scope{Kind: ScopeTask, ID: "task-1"}
	acquireTestItem(t, set, "pin", "task://job/7/decision/pin", "c", objective, RetentionPinned, 8)
	acquireTestItem(t, set, "task", "repo://snapshot/task", "d", task, RetentionTask, 8)
	result, err := set.CloseScope(root, "Job generation retired.")
	if err != nil {
		t.Fatal(err)
	}
	if set.Status() != StatusClosed || len(result.Released) != 2 || result.Released[0].State != ItemReleased {
		t.Fatalf("terminal close=%s %+v", set.Status(), result)
	}
	snapshot := set.Snapshot()
	if snapshot.ClosedTick == 0 || snapshot.CloseReason != "Job generation retired." || !set.ScopeClosed(root) {
		t.Fatalf("terminal snapshot=%+v", snapshot)
	}
	request := testRequest("late", "repo://snapshot/late", "e", root, RetentionJob, 1)
	if _, err := set.Acquire(request); !errors.Is(err, ErrSetClosed) {
		t.Fatalf("post-close acquire error=%v", err)
	}
	if _, err := set.TouchMany([]ItemID{"pin"}); !errors.Is(err, ErrSetClosed) {
		t.Fatalf("post-close touch error=%v", err)
	}
	if _, err := set.CloseScope(root, "Again."); !errors.Is(err, ErrSetClosed) {
		t.Fatalf("second root close error=%v", err)
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("closed snapshot: %v", err)
	}
	restored, err := Restore(snapshot)
	if err != nil {
		t.Fatalf("restore closed set: %v", err)
	}
	if restored.Status() != StatusClosed {
		t.Fatalf("restored status=%s", restored.Status())
	}
}

func TestDuplicateHistoricalReferenceAndForeignJobScopeAreRejected(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 16})
	root := set.Scope()
	request := testRequest("first", "repo://snapshot/reused", "f", root, RetentionJob, 4)
	if _, err := set.Acquire(request); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Release("first", root, "No longer resident."); err != nil {
		t.Fatal(err)
	}
	request.ID = "second"
	if _, err := set.Acquire(request); !errors.Is(err, ErrDuplicateReference) {
		t.Fatalf("historical duplicate error=%v", err)
	}
	request.ID = "foreign"
	request.Ref = testRef("repo://snapshot/foreign", "a")
	request.Scope = Scope{Kind: ScopeJob, ID: "job-8"}
	if _, err := set.Acquire(request); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("foreign job scope error=%v", err)
	}
}

func TestSnapshotHardCapsFailBeforeRecordValidation(t *testing.T) {
	t.Parallel()
	base := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 1}).Snapshot()
	tests := map[string]func(*Snapshot){
		"items":         func(s *Snapshot) { s.Items = make([]Item, MaxHistoricalItems+1) },
		"closed scopes": func(s *Snapshot) { s.ClosedScopes = make([]Scope, MaxClosedScopes+1) },
		"memberships": func(s *Snapshot) {
			s.Items = []Item{{Memberships: make([]Membership, MaxMemberships+1)}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := base
			mutate(&snapshot)
			if err := ValidateSnapshot(snapshot); !errors.Is(err, ErrCapacityExceeded) {
				t.Fatalf("capacity error=%v", err)
			}
		})
	}
}

func TestExactTextAndBigintLimitsAreEnforced(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 16})
	root := set.Scope()
	for name, value := range map[string]string{
		"nul": "bad\x00value", "invalid utf8": string([]byte{0xff}),
		"oversized": strings.Repeat("x", MaxExactReasonBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			request := testRequest("item", "repo://snapshot/item", "e", root, RetentionJob, 1)
			request.Acquisition.Reason = value
			if _, err := set.Acquire(request); err == nil {
				t.Fatal("invalid persistence text accepted")
			}
		})
	}
	acquireTestItem(t, set, "resident", "repo://snapshot/resident", "f", root, RetentionJob, 1)
	snapshot := set.Snapshot()
	snapshot.Version = uint64(math.MaxInt64)
	snapshot.Clock = snapshot.Version
	snapshot.Items[0].LastUsedTick = snapshot.Clock
	snapshot.Items[0].UseCount = snapshot.Clock - 1
	restored, err := Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest("overflow", "repo://snapshot/overflow", "f", root, RetentionJob, 1)
	if _, err := restored.Acquire(request); !errors.Is(err, ErrClockOverflow) {
		t.Fatalf("overflow mutation error=%v", err)
	}
}
