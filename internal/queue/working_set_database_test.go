package queue

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestPostgresWorkingSetCommandsMaterializeAndReplayExactly(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("working-set-roundtrip-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	budget := workingset.Budget{MaxItems: 4, MaxBytes: 64, MaxPinnedItems: 1, MaxPinnedBytes: 16}
	if _, err := repository.CurrentWorkingSet(ctx, job.ID); !errors.Is(err, ErrWorkingSetNotFound) {
		t.Fatalf("enqueue unexpectedly created a working set: %v", err)
	}
	created, err := repository.CreateCurrentWorkingSet(ctx, job.ID, 1, budget)
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 0 || created.Status != workingset.StatusActive || len(created.Items) != 0 {
		t.Fatalf("created working set is not exact and empty: %+v", created)
	}
	if _, err := repository.CreateCurrentWorkingSet(ctx, job.ID, 1, budget); !errors.Is(err, ErrWorkingSetExists) {
		t.Fatalf("duplicate creation error=%v, want ErrWorkingSetExists", err)
	}

	taskScope := workingset.Scope{Kind: workingset.ScopeTask, ID: "task-1"}
	objectiveScope := workingset.Scope{Kind: workingset.ScopeObjective, ID: "objective-1"}
	commands := []workingset.Command{
		workingset.AcquireCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "acquire"),
			ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
			Request: workingSetDatabaseRequest("item-1", taskScope),
		},
		workingset.RetainCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "retain"),
			ExpectedVersion: 1, Actor: taskstate.AuthorityCode,
			ItemID: "item-1", Scope: objectiveScope, Retention: workingset.RetentionObjective,
		},
		workingset.TouchCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "touch"),
			ExpectedVersion: 2, Actor: taskstate.AuthorityCode, ItemIDs: []workingset.ItemID{"item-1"},
		},
		workingset.ReleaseCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "release"),
			ExpectedVersion: 3, Actor: taskstate.AuthorityCode, ItemID: "item-1", Scope: taskScope,
			Reason: "The task-local attention scope completed.",
		},
		workingset.InvalidateStaleCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "invalidate"),
			ExpectedVersion: 4, Actor: taskstate.AuthorityCode, ItemID: "item-1",
			CurrentVersion: "snapshot-2", CurrentHash: strings.Repeat("b", 64),
			Reason: "The repository snapshot changed.",
		},
		workingset.CloseScopeCommand{
			CommandID:       workingSetDatabaseCommandID(t, marker, "close"),
			ExpectedVersion: 5, Actor: taskstate.AuthorityCode, Scope: created.Scope,
			Reason: "The generation working set completed.",
		},
	}
	events := make([]workingset.Event, 0, len(commands))
	for _, command := range commands {
		event, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, command)
		if err != nil {
			t.Fatalf("apply %T: %v", command, err)
		}
		events = append(events, event)
		if len(events) == 1 {
			stale := workingset.TouchCommand{
				CommandID:       workingSetDatabaseCommandID(t, marker, "stale-version"),
				ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
				ItemIDs: []workingset.ItemID{"item-1"},
			}
			if _, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, stale); !errors.Is(err, workingset.ErrVersionConflict) {
				t.Fatalf("stale version error=%v, want ErrVersionConflict", err)
			}
		}
	}
	final, err := repository.CurrentWorkingSet(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != workingset.StatusClosed || final.Version != uint64(len(commands)) ||
		len(final.Items) != 1 || final.Items[0].State != workingset.ItemInvalidated {
		t.Fatalf("unexpected final working set: %+v", final)
	}
	reconstructed, err := workingset.Reconstruct(final.Owner, final.Budget, events)
	if err != nil {
		t.Fatalf("reconstruct persisted event stream: %v", err)
	}
	if !reflect.DeepEqual(reconstructed.Snapshot(), final) {
		t.Fatalf("persisted state differs from event replay\n got: %+v\nwant: %+v", final, reconstructed.Snapshot())
	}

	replayed, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, commands[0])
	if err != nil || !reflect.DeepEqual(replayed, events[0]) {
		t.Fatalf("exact replay event=%+v error=%v", replayed, err)
	}
	changed := commands[0].(workingset.AcquireCommand)
	changed.Request.Priority++
	if _, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, changed); !errors.Is(err, workingset.ErrCommandIDConflict) {
		t.Fatalf("changed command replay error=%v, want command conflict", err)
	}
	assertWorkingSetEventPages(t, ctx, repository, job.ID, 1, events)
	assertWorkingSetEventsImmutable(t, ctx, pool, final.ID)
}

func TestPostgresWorkingSetRejectsStaleGenerationAfterJobLock(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("working-set-generation-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	budget := workingset.Budget{MaxItems: 2, MaxBytes: 32}
	if _, err := repository.CreateCurrentWorkingSet(ctx, job.ID, 1, budget); err != nil {
		t.Fatal(err)
	}
	first := workingset.AcquireCommand{
		CommandID:       workingSetDatabaseCommandID(t, marker, "accepted"),
		ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
		Request: workingSetDatabaseRequest("item-1", workingset.Scope{Kind: workingset.ScopeTask, ID: "task-1"}),
	}
	accepted, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, first)
	if err != nil {
		t.Fatal(err)
	}
	advanceWorkingSetTestGeneration(t, ctx, pool, job.ID)

	stale := workingset.TouchCommand{
		CommandID:       workingSetDatabaseCommandID(t, marker, "stale"),
		ExpectedVersion: 1, Actor: taskstate.AuthorityCode, ItemIDs: []workingset.ItemID{"item-1"},
	}
	if _, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, stale); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("stale generation command error=%v, want ErrStaleJobGeneration", err)
	}
	replayed, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, first)
	if err != nil || !reflect.DeepEqual(replayed, accepted) {
		t.Fatalf("retired generation exact replay event=%+v error=%v", replayed, err)
	}
	old, err := repository.WorkingSetForGeneration(ctx, job.ID, 1)
	if err != nil || old.Version != 1 {
		t.Fatalf("historical working set=%+v error=%v", old, err)
	}
	if _, err := repository.CurrentWorkingSet(ctx, job.ID); !errors.Is(err, ErrWorkingSetNotFound) {
		t.Fatalf("missing generation-two set error=%v, want ErrWorkingSetNotFound", err)
	}
	if _, err := repository.CreateCurrentWorkingSet(ctx, job.ID, 2, budget); err != nil {
		t.Fatalf("create generation-two set: %v", err)
	}
	current, err := repository.CurrentWorkingSet(ctx, job.ID)
	if err != nil || current.Owner.Generation != 2 || current.Version != 0 {
		t.Fatalf("current generation working set=%+v error=%v", current, err)
	}
}

func TestPostgresWorkingSetCommandIdentityCannotCrossOwners(t *testing.T) {
	ctx, repository, _ := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("working-set-owner-%d", time.Now().UnixNano())
	firstJob := enqueueWorkingSetTestJob(t, ctx, repository, marker+"-one")
	secondJob := enqueueWorkingSetTestJob(t, ctx, repository, marker+"-two")
	budget := workingset.Budget{MaxItems: 1, MaxBytes: 16}
	for _, jobID := range []int64{firstJob.ID, secondJob.ID} {
		if _, err := repository.CreateCurrentWorkingSet(ctx, jobID, 1, budget); err != nil {
			t.Fatal(err)
		}
	}
	command := workingset.AcquireCommand{
		CommandID:       workingSetDatabaseCommandID(t, marker, "shared-command-id"),
		ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
		Request: workingSetDatabaseRequest("item-1", workingset.Scope{Kind: workingset.ScopeCall, ID: "call-1"}),
	}
	if _, err := repository.ApplyWorkingSetCommand(ctx, firstJob.ID, 1, command); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyWorkingSetCommand(ctx, secondJob.ID, 1, command); !errors.Is(err, workingset.ErrCommandIDConflict) {
		t.Fatalf("cross-owner command error=%v, want command conflict", err)
	}
	second, err := repository.CurrentWorkingSet(ctx, secondJob.ID)
	if err != nil || second.Version != 0 || len(second.Items) != 0 {
		t.Fatalf("cross-owner conflict mutated second set=%+v error=%v", second, err)
	}
}
