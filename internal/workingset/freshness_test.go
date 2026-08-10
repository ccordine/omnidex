package workingset

import (
	"errors"
	"testing"
)

func TestStaleSourceInvalidatesImmediatelyAndCanBeReacquired(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 20, MaxPinnedItems: 1, MaxPinnedBytes: 10})
	scope := Scope{Kind: ScopeStep, ID: "step-4"}
	acquireTestItem(t, set, "old", "repo://snapshot/file/91", "a", scope, RetentionStep, 10)
	version := set.Version()

	unchanged, stale, err := set.InvalidateStale("old", "v1", repeatDigest("a"), "Repository source changed.")
	if err != nil {
		t.Fatalf("check current source: %v", err)
	}
	if stale || unchanged.State != ItemResident || set.Version() != version {
		t.Fatalf("current source caused mutation: stale=%t item=%#v version=%d", stale, unchanged, set.Version())
	}

	invalidated, stale, err := set.InvalidateStale("old", "v2", repeatDigest("b"), "Repository source changed.")
	if err != nil {
		t.Fatalf("invalidate stale source: %v", err)
	}
	if !stale || invalidated.State != ItemInvalidated || invalidated.DispositionReason != "Repository source changed." || len(invalidated.Memberships) != 0 {
		t.Fatalf("stale item not invalidated: %#v", invalidated)
	}
	if usage := set.Usage(); usage.ResidentItems != 0 || usage.ResidentBytes != 0 {
		t.Fatalf("invalidated item consumes budget: %#v", usage)
	}
	if _, err := set.Retain("old", scope, RetentionStep); !errors.Is(err, ErrItemNotResident) {
		t.Fatalf("retain invalidated item error = %v, want ErrItemNotResident", err)
	}

	request := testRequest("fresh", "repo://snapshot/file/91", "b", scope, RetentionStep, 10)
	request.Ref.Version = "v2"
	if _, err := set.Acquire(request); err != nil {
		t.Fatalf("reacquire fresh identity: %v", err)
	}
	if items := set.Items(); len(items) != 2 {
		t.Fatalf("reacquisition erased invalidation history: %#v", items)
	}
}

func TestInvalidFreshnessIdentityFailsWithoutMutation(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 10, MaxPinnedItems: 0, MaxPinnedBytes: 0})
	scope := Scope{Kind: ScopePhase, ID: "phase-1"}
	acquireTestItem(t, set, "item", "repo://snapshot/module/one", "c", scope, RetentionPhase, 5)
	version := set.Version()
	if _, _, err := set.InvalidateStale("item", "v2", "not-a-digest", "Repository source changed."); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("invalid freshness hash error = %v, want ErrInvalidReference", err)
	}
	if set.Version() != version {
		t.Fatalf("invalid freshness check mutated version: %d != %d", set.Version(), version)
	}
}
