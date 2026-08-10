package workingset

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestReacquireReactivatesExactReleasedItemAndReplays(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 24, MaxPinnedItems: 1, MaxPinnedBytes: 12})
	firstScope := Scope{Kind: ScopeTask, ID: "task-old"}
	request := testRequest("item-1", "repo://snapshot/symbol/one", "a", firstScope, RetentionTask, 8)
	acquire := AcquireCommand{
		CommandID: workingCommandID(t, "acquire"), ExpectedVersion: 0,
		Actor: taskstate.AuthorityCode, Request: request,
	}
	acquiredEvent, err := set.Apply(acquire)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := set.Item(request.ID)
	release := ReleaseCommand{
		CommandID: workingCommandID(t, "release"), ExpectedVersion: 1,
		Actor: taskstate.AuthorityCode, ItemID: request.ID, Scope: firstScope,
		Reason: "The first task no longer requires this exact evidence.",
	}
	releasedEvent, err := set.Apply(release)
	if err != nil {
		t.Fatal(err)
	}
	newScope := Scope{Kind: ScopeStep, ID: "step-new"}
	reacquire := ReacquireCommand{
		CommandID: workingCommandID(t, "reacquire"), ExpectedVersion: 2,
		Actor: taskstate.AuthorityCode,
		Request: ReacquireRequest{
			ItemID: request.ID, Ref: request.Ref, Scope: newScope, Retention: RetentionStep,
			ExpectedReacquisitionCount: 0,
			Reason:                     "The current step requires this exact historical evidence again.",
		},
	}
	event, err := set.Apply(reacquire)
	if err != nil {
		t.Fatal(err)
	}
	item, _ := set.Item(request.ID)
	if item.ID != original.ID || item.Ref != original.Ref || item.Role != original.Role ||
		item.Priority != original.Priority || item.ByteCost != original.ByteCost ||
		item.Acquisition != original.Acquisition || item.CreatedTick != original.CreatedTick {
		t.Fatalf("reacquisition changed immutable acquisition identity\n got: %#v\nwant: %#v", item, original)
	}
	if item.State != ItemResident || item.Retention != RetentionStep ||
		!reflect.DeepEqual(item.Memberships, []Membership{{Scope: newScope, Retention: RetentionStep}}) ||
		item.ReleasedTick != 0 || item.DispositionReason != "" || item.ReacquisitionCount != 1 {
		t.Fatalf("reacquired item lifecycle = %#v", item)
	}
	wantMetadata := &ReacquisitionMetadata{
		ItemID: request.ID, Count: 1, OriginalAcquisition: original.Acquisition,
	}
	if event.Kind != EventReacquired || !reflect.DeepEqual(event.Reacquisition, wantMetadata) {
		t.Fatalf("reacquisition event = %#v", event)
	}
	tamperedEvent := cloneEvent(event)
	tamperedEvent.Reacquisition.Count++
	if err := ValidateEvent(tamperedEvent); err == nil {
		t.Fatal("tampered immutable reacquisition count was accepted")
	}
	replayed, err := set.Apply(reacquire)
	if err != nil || !reflect.DeepEqual(replayed, event) || set.Version() != 3 {
		t.Fatalf("exact replay event=%#v error=%v version=%d", replayed, err, set.Version())
	}
	changed := reacquire
	changed.Request.Reason = "Different reason under the same command identity."
	if _, err := set.Apply(changed); !errors.Is(err, ErrCommandIDConflict) {
		t.Fatalf("changed replay error=%v, want ErrCommandIDConflict", err)
	}
	reconstructed, err := Reconstruct(set.Owner(), set.Budget(), []Event{acquiredEvent, releasedEvent, event})
	if err != nil || !reflect.DeepEqual(reconstructed.Snapshot(), set.Snapshot()) {
		t.Fatalf("reconstruct=%#v error=%v want=%#v", reconstructed, err, set.Snapshot())
	}
	tamperedHistory := []Event{acquiredEvent, releasedEvent, cloneEvent(event)}
	tamperedHistory[2].Reacquisition.OriginalAcquisition.OperationID = "tampered-operation"
	if _, err := Reconstruct(set.Owner(), set.Budget(), tamperedHistory); err == nil {
		t.Fatal("tampered immutable original acquisition metadata replayed")
	}
}

func TestReacquireRejectsResidentInvalidatedChangedAndStaleRequests(t *testing.T) {
	t.Parallel()
	scope := Scope{Kind: ScopeTask, ID: "task-1"}
	newScope := Scope{Kind: ScopeStep, ID: "step-2"}
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 20})
	request := testRequest("item", "repo://snapshot/file/one", "b", scope, RetentionTask, 8)
	acquireTestItem(t, set, request.ID, request.Ref.URI, "b", scope, RetentionTask, request.ByteCost)
	command := ReacquireCommand{
		CommandID: workingCommandID(t, "resident"), ExpectedVersion: set.Version(),
		Actor: taskstate.AuthorityCode,
		Request: ReacquireRequest{ItemID: request.ID, Ref: request.Ref, Scope: newScope, Retention: RetentionStep,
			ExpectedReacquisitionCount: 0, Reason: "The new step needs exact evidence."},
	}
	if _, err := set.Apply(command); !errors.Is(err, ErrItemNotReleased) {
		t.Fatalf("resident reacquire error=%v, want ErrItemNotReleased", err)
	}
	if _, err := set.Release(request.ID, scope, "The task released the evidence."); err != nil {
		t.Fatal(err)
	}
	version := set.Version()
	for name, mutate := range map[string]func(*ReacquireRequest){
		"uri":      func(r *ReacquireRequest) { r.Ref.URI += "/changed" },
		"version":  func(r *ReacquireRequest) { r.Ref.Version = "v2" },
		"hash":     func(r *ReacquireRequest) { r.Ref.Hash = repeatDigest("c") },
		"relation": func(r *ReacquireRequest) { r.Ref.Relation = taskstate.RefSupports },
		"count":    func(r *ReacquireRequest) { r.ExpectedReacquisitionCount = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := command
			candidate.CommandID = workingCommandID(t, "changed-"+name)
			candidate.ExpectedVersion = version
			mutate(&candidate.Request)
			if _, err := set.Apply(candidate); !errors.Is(err, ErrReacquisitionConflict) {
				t.Fatalf("changed request error=%v, want ErrReacquisitionConflict", err)
			}
			if set.Version() != version {
				t.Fatalf("changed request mutated version to %d", set.Version())
			}
		})
	}

	invalidated := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 10})
	acquireTestItem(t, invalidated, request.ID, request.Ref.URI, "b", scope, RetentionTask, request.ByteCost)
	if _, _, err := invalidated.InvalidateStale(request.ID, "v2", repeatDigest("d"), "The source changed."); err != nil {
		t.Fatal(err)
	}
	command.CommandID = workingCommandID(t, "invalidated")
	command.ExpectedVersion = invalidated.Version()
	if _, err := invalidated.Apply(command); !errors.Is(err, ErrItemInvalidated) {
		t.Fatalf("invalidated reacquire error=%v, want ErrItemInvalidated", err)
	}

	closed := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 10})
	acquireTestItem(t, closed, request.ID, request.Ref.URI, "b", scope, RetentionTask, request.ByteCost)
	if _, err := closed.Release(request.ID, scope, "The task released the evidence."); err != nil {
		t.Fatal(err)
	}
	if _, err := closed.CloseScope(newScope, "The destination step completed."); err != nil {
		t.Fatal(err)
	}
	command.CommandID = workingCommandID(t, "closed-scope")
	command.ExpectedVersion = closed.Version()
	if _, err := closed.Apply(command); !errors.Is(err, ErrScopeClosed) {
		t.Fatalf("closed-scope reacquire error=%v, want ErrScopeClosed", err)
	}
}

func TestReacquireUsesAcquireBudgetEvictionWithoutDuplicateHistory(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 1, MaxBytes: 6})
	oldScope := Scope{Kind: ScopeCall, ID: "call-old"}
	newScope := Scope{Kind: ScopeCall, ID: "call-new"}
	old := testRequest("old", "repo://snapshot/file/old", "d", oldScope, RetentionCall, 6)
	if _, err := set.Acquire(old); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Release(old.ID, oldScope, "Release the historical evidence."); err != nil {
		t.Fatal(err)
	}
	resident := testRequest("resident", "repo://snapshot/file/resident", "e", newScope, RetentionCall, 6)
	if _, err := set.Acquire(resident); err != nil {
		t.Fatal(err)
	}
	result, err := set.Reacquire(ReacquireRequest{
		ItemID: old.ID, Ref: old.Ref, Scope: newScope, Retention: RetentionCall,
		ExpectedReacquisitionCount: 0, Reason: "Use the exact historical evidence again.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evicted) != 1 || result.Evicted[0].ID != resident.ID ||
		result.Item.ID != old.ID || result.Item.ReacquisitionCount != 1 || len(set.Items()) != 2 {
		t.Fatalf("reacquisition result=%#v items=%#v", result, set.Items())
	}
	duplicate := old
	duplicate.ID = "duplicate-row"
	if _, err := set.Acquire(duplicate); !errors.Is(err, ErrDuplicateReference) {
		t.Fatalf("duplicate Acquire error=%v, want ErrDuplicateReference", err)
	}
}
