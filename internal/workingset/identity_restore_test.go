package workingset

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestWorkingSetIdentityIsDeterministicAndOwnerBound(t *testing.T) {
	t.Parallel()
	owner := testOwner(t, 7, 3)
	first, err := NewSetID(owner)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSetID(owner)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(string(first), "working_set_") || len(first) != len("working_set_")+64 {
		t.Fatalf("set identities=%q/%q", first, second)
	}
	for name, invalid := range map[string]Owner{
		"missing ledger":       {JobID: 7, Generation: 3},
		"wrong ledger grammar": {LedgerID: "ledger_bad", JobID: 7, Generation: 3},
		"zero job":             {LedgerID: owner.LedgerID, Generation: 3},
		"zero generation":      {LedgerID: owner.LedgerID, JobID: 7},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSetID(invalid); !errors.Is(err, ErrInvalidSet) {
				t.Fatalf("NewSetID error=%v", err)
			}
		})
	}
	changed := owner
	changed.Generation++
	changedID, err := NewSetID(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedID == first {
		t.Fatal("generation did not affect working-set identity")
	}
}

func TestSnapshotRestoreIsExactAndDoesNotShareMutableState(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 4, MaxBytes: 128, MaxPinnedItems: 1, MaxPinnedBytes: 32})
	scope := Scope{Kind: ScopeTask, ID: "task-1"}
	acquireTestItem(t, set, "item-1", "repo://snapshot/symbol/one", "a", scope, RetentionTask, 16)
	if _, err := set.TouchMany([]ItemID{"item-1"}); err != nil {
		t.Fatal(err)
	}
	snapshot := set.Snapshot()
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("validate snapshot: %v", err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(decoded); err != nil {
		t.Fatalf("restore JSON round trip: %v", err)
	}
	restored, err := Restore(snapshot)
	if err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	snapshot.Items[0].Memberships[0].Scope.ID = "tampered"
	snapshot.ClosedScopes = append(snapshot.ClosedScopes, Scope{Kind: ScopeCall, ID: "tampered"})
	current, ok := restored.Item("item-1")
	if !ok || current.Memberships[0].Scope.ID != "task-1" || len(restored.Snapshot().ClosedScopes) != 0 {
		t.Fatalf("restore retained caller-owned memory: %+v", restored.Snapshot())
	}
	returned := restored.Snapshot()
	returned.Items[0].Memberships = nil
	if current, _ := restored.Item("item-1"); len(current.Memberships) != 1 {
		t.Fatal("snapshot exposed aggregate-owned memberships")
	}
}

func TestRestoreRejectsLifecycleAndPersistenceCorruption(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32, MaxPinnedItems: 1, MaxPinnedBytes: 16})
	scope := Scope{Kind: ScopeTask, ID: "task-1"}
	acquireTestItem(t, set, "item-1", "repo://snapshot/symbol/one", "b", scope, RetentionTask, 8)
	valid := set.Snapshot()
	tests := map[string]func(*Snapshot){
		"wrong identity":         func(s *Snapshot) { s.ID = SetID("working_set_" + strings.Repeat("0", 64)) },
		"version clock mismatch": func(s *Snapshot) { s.Version++ },
		"postgres overflow":      func(s *Snapshot) { s.Clock = uint64(math.MaxInt64) + 1; s.Version = s.Clock },
		"duplicate item":         func(s *Snapshot) { s.Items = append(s.Items, s.Items[0]) },
		"resident disposition":   func(s *Snapshot) { s.Items[0].DispositionReason = "invalid" },
		"released without tick":  func(s *Snapshot) { s.Items[0].State = ItemReleased; s.Items[0].Memberships = nil },
		"wrong root scope":       func(s *Snapshot) { s.Scope.ID = "job-8" },
		"invalid utf8":           func(s *Snapshot) { s.Items[0].Acquisition.Reason = string([]byte{0xff}) },
		"nul":                    func(s *Snapshot) { s.Items[0].Acquisition.Reason = "bad\x00reason" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := cloneSnapshot(valid)
			mutate(&snapshot)
			if _, err := Restore(snapshot); err == nil {
				t.Fatalf("corrupt snapshot restored: %+v", snapshot)
			}
		})
	}
}
