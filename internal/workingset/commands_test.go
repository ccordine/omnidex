package workingset

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestWorkingSetCommandsAreVersionedIdempotentAndReplayable(t *testing.T) {
	t.Parallel()
	owner := testOwner(t, 41, 3)
	budget := Budget{MaxItems: 3, MaxBytes: 64, MaxPinnedItems: 1, MaxPinnedBytes: 16}
	set, err := New(owner, budget)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{Kind: ScopeTask, ID: "task-1"}
	commands := []Command{
		AcquireCommand{
			CommandID: workingCommandID(t, "acquire"), ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
			Request: testRequest("item-1", "repo://snapshot/symbol/one", "a", scope, RetentionTask, 8),
		},
		TouchCommand{
			CommandID: workingCommandID(t, "touch"), ExpectedVersion: 1,
			Actor: taskstate.AuthorityCode, ItemIDs: []ItemID{"item-1"},
		},
		RetainCommand{
			CommandID: workingCommandID(t, "retain"), ExpectedVersion: 2, Actor: taskstate.AuthorityCode,
			ItemID: "item-1", Scope: Scope{Kind: ScopeObjective, ID: "objective-1"}, Retention: RetentionObjective,
		},
		ReleaseCommand{
			CommandID: workingCommandID(t, "release"), ExpectedVersion: 3, Actor: taskstate.AuthorityCode,
			ItemID: "item-1", Scope: scope, Reason: "Task-local attention is complete.",
		},
		InvalidateStaleCommand{
			CommandID: workingCommandID(t, "invalidate"), ExpectedVersion: 4, Actor: taskstate.AuthorityCode,
			ItemID: "item-1", CurrentVersion: "v2", CurrentHash: repeatDigest("b"),
			Reason: "The repository source identity changed.",
		},
		CloseScopeCommand{
			CommandID: workingCommandID(t, "close"), ExpectedVersion: 5, Actor: taskstate.AuthorityCode,
			Scope: set.Scope(), Reason: "The job generation completed.",
		},
	}
	events := make([]Event, 0, len(commands))
	for _, command := range commands {
		event, err := set.Apply(command)
		if err != nil {
			t.Fatalf("apply %T: %v", command, err)
		}
		if err := ValidateEvent(event); err != nil {
			t.Fatalf("validate %T event: %v", command, err)
		}
		events = append(events, event)
	}
	if set.Status() != StatusClosed || set.Version() != uint64(len(commands)) {
		t.Fatalf("terminal set status=%q version=%d", set.Status(), set.Version())
	}
	replayed, err := Reconstruct(owner, budget, events)
	if err != nil {
		t.Fatalf("reconstruct events: %v", err)
	}
	if !reflect.DeepEqual(replayed.Snapshot(), set.Snapshot()) {
		t.Fatalf("replayed snapshot differs\n got: %#v\nwant: %#v", replayed.Snapshot(), set.Snapshot())
	}
}

func TestWorkingSetCommandReplayAndConflictsAreExplicit(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32})
	command := AcquireCommand{
		CommandID: workingCommandID(t, "replay"), ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
		Request: testRequest(
			"item-1", "repo://snapshot/symbol/one", "a",
			Scope{Kind: ScopeCall, ID: "call-1"}, RetentionCall, 8,
		),
	}
	first, err := set.Apply(command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := set.Apply(command)
	if err != nil || !reflect.DeepEqual(first, replayed) || set.Version() != 1 {
		t.Fatalf("exact replay event=%#v error=%v version=%d", replayed, err, set.Version())
	}
	changed := command
	changed.Request.Priority++
	if _, err := set.Apply(changed); !errors.Is(err, ErrCommandIDConflict) {
		t.Fatalf("changed replay error=%v, want ErrCommandIDConflict", err)
	}
	stale := TouchCommand{
		CommandID: workingCommandID(t, "stale"), ExpectedVersion: 0,
		Actor: taskstate.AuthorityCode, ItemIDs: []ItemID{"item-1"},
	}
	if _, err := set.Apply(stale); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale command error=%v, want ErrVersionConflict", err)
	}
}

func TestWorkingSetCommandsRejectNonCodeAuthorityAndNoOpInvalidation(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32})
	request := testRequest(
		"item-1", "repo://snapshot/symbol/one", "a",
		Scope{Kind: ScopeCall, ID: "call-1"}, RetentionCall, 8,
	)
	invalid := AcquireCommand{
		CommandID: workingCommandID(t, "model"), ExpectedVersion: 0,
		Actor: taskstate.AuthorityModelProposal, Request: request,
	}
	if _, err := set.Apply(invalid); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("model mutation error=%v, want ErrInvalidCommand", err)
	}
	valid := invalid
	valid.CommandID = workingCommandID(t, "code")
	valid.Actor = taskstate.AuthorityCode
	if _, err := set.Apply(valid); err != nil {
		t.Fatal(err)
	}
	noChange := InvalidateStaleCommand{
		CommandID: workingCommandID(t, "fresh"), ExpectedVersion: 1, Actor: taskstate.AuthorityCode,
		ItemID: request.ID, CurrentVersion: request.Ref.Version, CurrentHash: request.Ref.Hash,
		Reason: "Check the bound source identity.",
	}
	if _, err := set.Apply(noChange); !errors.Is(err, ErrNoStateChange) {
		t.Fatalf("fresh invalidation error=%v, want ErrNoStateChange", err)
	}
	if set.Version() != 1 {
		t.Fatalf("no-op invalidation changed version to %d", set.Version())
	}
}

func TestWorkingSetEventRejectsCommandPayloadTampering(t *testing.T) {
	t.Parallel()
	set := newTestSet(t, Budget{MaxItems: 2, MaxBytes: 32})
	event, err := set.Apply(AcquireCommand{
		CommandID: workingCommandID(t, "tamper"), ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
		Request: testRequest(
			"item-1", "repo://snapshot/symbol/one", "a",
			Scope{Kind: ScopeCall, ID: "call-1"}, RetentionCall, 8,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(event.Command, &fields); err != nil {
		t.Fatal(err)
	}
	fields["expected_version"] = float64(9)
	event.Command, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("tampered event command was accepted")
	}
}

func workingCommandID(t *testing.T, label string) CommandID {
	t.Helper()
	id, err := NewCommandID(t.Name(), label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
