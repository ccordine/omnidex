package queue

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestPostgresWorkingSetReacquiresOneExactHistoricalRow(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("working-set-reacquire-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	authority := claimWorkingSetTestJob(t, ctx, repository, job)
	if _, err := repository.CreateCurrentWorkingSet(ctx, authority, workingset.Budget{
		MaxItems: 2, MaxBytes: 32, MaxPinnedItems: 1, MaxPinnedBytes: 16,
	}); err != nil {
		t.Fatal(err)
	}
	oldScope := workingset.Scope{Kind: workingset.ScopeTask, ID: "task-old"}
	newScope := workingset.Scope{Kind: workingset.ScopeStep, ID: "step-new"}
	request := workingSetDatabaseRequest("item-1", oldScope)
	commands := []workingset.Command{
		workingset.AcquireCommand{
			CommandID: workingSetDatabaseCommandID(t, marker, "acquire"), ExpectedVersion: 0,
			Actor: taskstate.AuthorityCode, Request: request,
		},
		workingset.ReleaseCommand{
			CommandID: workingSetDatabaseCommandID(t, marker, "release"), ExpectedVersion: 1,
			Actor: taskstate.AuthorityCode, ItemID: request.ID, Scope: oldScope,
			Reason: "The prior task released this exact evidence.",
		},
		workingset.ReacquireCommand{
			CommandID: workingSetDatabaseCommandID(t, marker, "reacquire"), ExpectedVersion: 2,
			Actor: taskstate.AuthorityCode,
			Request: workingset.ReacquireRequest{
				ItemID: request.ID, Ref: request.Ref, Scope: newScope, Retention: workingset.RetentionStep,
				ExpectedReacquisitionCount: 0,
				Reason:                     "The current step requires this exact historical evidence.",
			},
		},
	}
	events := make([]workingset.Event, 0, len(commands))
	for _, command := range commands {
		event, err := repository.ApplyWorkingSetCommand(ctx, authority, command)
		if err != nil {
			t.Fatalf("apply %T: %v", command, err)
		}
		events = append(events, event)
	}
	final, err := repository.CurrentWorkingSet(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Items) != 1 || final.Items[0].ID != request.ID ||
		final.Items[0].State != workingset.ItemResident || final.Items[0].ReacquisitionCount != 1 ||
		final.Items[0].Acquisition != request.Acquisition || final.Items[0].CreatedTick != 1 {
		t.Fatalf("reacquired materialized item=%#v", final.Items)
	}
	if events[2].Reacquisition == nil || events[2].Reacquisition.ItemID != request.ID ||
		events[2].Reacquisition.Count != 1 || events[2].Reacquisition.OriginalAcquisition != request.Acquisition {
		t.Fatalf("reacquisition event metadata=%#v", events[2])
	}
	var itemRows, eventCount int
	var persistedCount int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(reacquisition_count)
		FROM working_set_items WHERE working_set_id=$1 AND item_id=$2
	`, final.ID, request.ID).Scan(&itemRows, &persistedCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM working_set_events
		WHERE working_set_id=$1 AND command_kind='reacquire' AND event_kind='reacquired'
		  AND reacquired_item_id=$2 AND reacquisition_count=1
	`, final.ID, request.ID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if itemRows != 1 || persistedCount != 1 || eventCount != 1 {
		t.Fatalf("item rows=%d count=%d reacquire events=%d", itemRows, persistedCount, eventCount)
	}
	replayed, err := repository.ApplyWorkingSetCommand(ctx, authority, commands[2])
	if err != nil || !reflect.DeepEqual(replayed, events[2]) {
		t.Fatalf("exact replay=%#v error=%v", replayed, err)
	}
	changed := commands[2].(workingset.ReacquireCommand)
	changed.Request.Ref.Hash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := repository.ApplyWorkingSetCommand(ctx, authority, changed); !errors.Is(err, workingset.ErrCommandIDConflict) {
		t.Fatalf("changed replay error=%v, want ErrCommandIDConflict", err)
	}
	page, err := repository.ListWorkingSetEvents(ctx, job.ID, 1, 0, 10)
	if err != nil || len(page) != 3 {
		t.Fatalf("event page=%#v error=%v", page, err)
	}
	persisted := make([]workingset.Event, len(page))
	for index := range page {
		persisted[index] = page[index].Event
	}
	reconstructed, err := workingset.Reconstruct(final.Owner, final.Budget, persisted)
	if err != nil || !reflect.DeepEqual(reconstructed.Snapshot(), final) {
		t.Fatalf("event reconstruction=%#v error=%v", reconstructed, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE working_set_items SET reacquisition_count=2 WHERE working_set_id=$1 AND item_id=$2
	`, final.ID, request.ID); err == nil {
		t.Fatal("direct reacquisition-count tampering was accepted")
	}
	releaseAfterReplay := workingset.ReleaseCommand{
		CommandID:       workingSetDatabaseCommandID(t, marker, "release-after-reacquire"),
		ExpectedVersion: final.Version, Actor: taskstate.AuthorityCode,
		ItemID: request.ID, Scope: newScope, Reason: "Release before testing eventless reactivation.",
	}
	if _, err := repository.ApplyWorkingSetCommand(ctx, authority, releaseAfterReplay); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE working_set_items
		SET state='resident', retention='step', reacquisition_count=2,
		    last_used_tick=released_tick+1, released_tick=0, disposition_reason=''
		WHERE working_set_id=$1 AND item_id=$2
	`, final.ID, request.ID); err == nil {
		t.Fatal("released item reactivation without an immutable reacquisition event was accepted")
	}
}

func TestPostgresCognitionReconciliationUsesDurableReacquireCommand(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	consumeReacquireCognitionPolicyCall(t, repository, fixture)
	before, err := repository.CurrentWorkingSet(ctx, fixture.Authority.JobID)
	if err != nil {
		t.Fatal(err)
	}
	var target workingset.Item
	for _, item := range before.Items {
		if item.State == workingset.ItemResident && item.Role == workingset.RoleTask {
			target = item
			break
		}
	}
	if target.ID == "" || len(target.Memberships) != 1 {
		t.Fatalf("initial cognition task item=%#v", target)
	}
	release := workingset.ReleaseCommand{
		CommandID:       workingSetDatabaseCommandID(t, string(fixture.EpisodeID), "release-task"),
		ExpectedVersion: before.Version, Actor: taskstate.AuthorityCode,
		ItemID: target.ID, Scope: target.Memberships[0].Scope,
		Reason: "Release the exact task to prove cognition reconciliation reacquires history.",
	}
	if _, err := repository.ApplyWorkingSetCommand(ctx, fixture.Authority, release); err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := repository.CurrentWorkingSet(ctx, fixture.Authority.JobID)
	if err != nil {
		t.Fatal(err)
	}
	reacquired, found := func() (workingset.Item, bool) {
		for _, item := range after.Items {
			if item.ID == target.ID {
				return item, true
			}
		}
		return workingset.Item{}, false
	}()
	if !found || reacquired.State != workingset.ItemResident || reacquired.ReacquisitionCount != 1 ||
		reacquired.Acquisition != target.Acquisition || reacquired.CreatedTick != target.CreatedTick {
		t.Fatalf("cognition reconciliation item=%#v original=%#v", reacquired, target)
	}
	projection := prepared.Prepared.Snapshot.ContextProjection()
	if projection.WorkingSetVersion != after.Version || string(projection.WorkingSetID) != string(after.ID) {
		t.Fatalf("prepared projection=%#v working set=%#v", projection, after)
	}
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM working_set_events
		WHERE working_set_id=$1 AND command_kind='reacquire' AND event_kind='reacquired'
		  AND reacquired_item_id=$2 AND reacquisition_count=1
	`, after.ID, target.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("durable cognition reacquisition events=%d want 1", count)
	}
}

func consumeReacquireCognitionPolicyCall(
	t *testing.T,
	repository *Repository,
	fixture cognitionDatabaseFixture,
) {
	t.Helper()
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	schema := fixture.Catalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{{Name: "target", Value: "current"}})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: request,
		EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence}, ExpectedEffect: "Inspect bounded public state.",
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)},
		cognitionTestBrain(), cognitionGuardProjectionLoader{repository: repository},
		CognitionPolicyCallJournal{Repository: repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(t.Context(), prepared.Prepared.Snapshot); err != nil {
		t.Fatal(err)
	}
}
