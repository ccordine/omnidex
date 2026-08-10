package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionActionTakeoverPreservesSemanticIdentityAndFencesOrigin(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "action-takeover")
	original := prepareCognitionGuardAction(t, fixture, "action-takeover")
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	if _, err := fixture.Repository.UnresolvedCognitionAction(
		fixture.Context, fixture.Authority, fixture.EpisodeID,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale unresolved read error=%v, want ErrStaleStepAttempt", err)
	}
	unresolved, err := fixture.Repository.UnresolvedCognitionAction(fixture.Context, replacement, fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved == nil || unresolved.Action.ID != original.Action.ID ||
		unresolved.Status != CognitionActionDispatched {
		t.Fatalf("unresolved cognition action=%+v", unresolved)
	}
	reauthorized, err := unresolved.ActionFor(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if reauthorized.ID != original.Action.ID || !reflect.DeepEqual(reauthorized.Request, original.Action.Request) ||
		reauthorized.Actor.Attempt != uint64(replacement.Attempt) ||
		reauthorized.Actor.WorkerID != replacement.WorkerID {
		t.Fatalf("reauthorized action=%+v original=%+v", reauthorized, original.Action)
	}
	if _, err := fixture.Repository.DispatchCognitionAction(
		fixture.Context, replacement, original.Action.ID,
	); err != nil {
		t.Fatalf("replacement dispatch replay: %v", err)
	}
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("8"))
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: original.Action.ID, Previous: cognitionRevisionPointer(original.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{}, Cost: 1,
	}
	result, err := fixture.Repository.IngestCognitionTransition(
		fixture.Context, replacement, original.Action.ID, transition, cognitionTestFactAuthority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != CognitionActionSucceeded || result.Origin != fixture.Authority {
		t.Fatalf("takeover result=%+v", result)
	}
	if _, err := fixture.Repository.IngestCognitionTransition(
		fixture.Context, fixture.Authority, original.Action.ID, transition, cognitionTestFactAuthority(),
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale origin ingestion error=%v, want ErrStaleStepAttempt", err)
	}
	if unresolved, err := fixture.Repository.UnresolvedCognitionAction(
		fixture.Context, replacement, fixture.EpisodeID,
	); err != nil || unresolved != nil {
		t.Fatalf("resolved action remains unresolved=%+v error=%v", unresolved, err)
	}
	var preparedActor, dispatchedActor, succeededActor string
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT MAX(actor_worker_id) FILTER (WHERE sequence=1),
		       MAX(actor_worker_id) FILTER (WHERE sequence=2),
		       MAX(actor_worker_id) FILTER (WHERE sequence=3)
		FROM cognition_action_events WHERE action_id=$1
	`, original.Action.ID).Scan(&preparedActor, &dispatchedActor, &succeededActor); err != nil {
		t.Fatal(err)
	}
	if preparedActor != fixture.Authority.WorkerID || dispatchedActor != fixture.Authority.WorkerID ||
		succeededActor != replacement.WorkerID {
		t.Fatalf("action event actors=%q/%q/%q", preparedActor, dispatchedActor, succeededActor)
	}
}
