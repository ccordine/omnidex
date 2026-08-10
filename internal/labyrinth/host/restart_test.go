package host

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
)

func TestPostgresRecreationReconstructsCommittedWorldAndSuppressesDuplicate(t *testing.T) {
	fixture := newDurableFixture(t)
	authorized := func(actor cognition.AttemptRef) bool {
		return actor.JobID == fixture.Actor.JobID && actor.Generation == fixture.Actor.Generation &&
			actor.StepID == fixture.Actor.StepID && (actor.Attempt == 1 || actor.Attempt == 2)
	}
	firstHost := fixture.environment(t, authorized)
	started, err := firstHost.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	firstAction := fixture.action(t, 0, "durable-first", fixture.Actor)
	first, err := firstHost.Apply(context.Background(), fixture.Episode, started.Current, firstAction)
	if err != nil {
		t.Fatal(err)
	}

	secondStore, err := NewStore(fixture.Pool)
	if err != nil {
		t.Fatal(err)
	}
	secondHost, err := NewEnvironment(
		secondStore, fixture.Episode, fixture.resolver(),
		func(_ context.Context, actor cognition.AttemptRef) error {
			if !authorized(actor) {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
		func(_ context.Context, _ pgx.Tx, actor cognition.AttemptRef) error {
			if !authorized(actor) {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := secondHost.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, restarted) {
		t.Fatalf("durable start replay changed:\nfirst=%#v\nreplay=%#v", started, restarted)
	}
	replacement := firstAction.Clone()
	replacement.Actor.Attempt = 2
	replacement.Actor.WorkerID = "replacement-worker"
	replayed, err := secondHost.Apply(context.Background(), fixture.Episode, started.Current, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatalf("durable action replay changed:\nfirst=%#v\nreplay=%#v", first, replayed)
	}
	secondAction := fixture.action(t, 1, "durable-second", replacement.Actor)
	second, err := secondHost.Apply(context.Background(), fixture.Episode, first.Current, secondAction)
	if err != nil {
		t.Fatal(err)
	}
	if second.Current.Number != first.Current.Number+1 {
		t.Fatalf("reconstructed transition number = %d", second.Current.Number)
	}
	episodeReceipt, err := secondStore.Episode(context.Background(), fixture.Episode)
	if err != nil {
		t.Fatal(err)
	}
	if episodeReceipt.Current != second.Current {
		t.Fatalf("database head = %#v, want %#v", episodeReceipt.Current, second.Current)
	}
	stored, err := secondStore.Action(context.Background(), fixture.Episode, firstAction.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Transition == nil || !reflect.DeepEqual(*stored.Transition, first) {
		t.Fatalf("stored transition receipt changed: %#v", stored)
	}
}

func TestPostgresActionIDConflictAndStaleRevisionAreTypedAndDurable(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	firstAction := fixture.action(t, 0, "bound-action-id", fixture.Actor)
	first, err := environment.Apply(context.Background(), fixture.Episode, started.Current, firstAction)
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture.action(t, 1, firstAction.ID, fixture.Actor)
	_, err = environment.Apply(context.Background(), fixture.Episode, started.Current, changed)
	if !errors.Is(err, labyrinth.ErrReplayConflict) {
		t.Fatalf("changed action identity error = %v", err)
	}
	var conflict cognition.ActionFailure
	if !errors.As(err, &conflict) || conflict.Code != cognition.ActionFailureIdempotencyConflict {
		t.Fatalf("changed action identity lacks typed conflict: %v", err)
	}
	stale := fixture.action(t, 1, "stale-revision-action", fixture.Actor)
	_, err = environment.Apply(context.Background(), fixture.Episode, started.Current, stale)
	if !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	var staleFailure cognition.ActionFailure
	if !errors.As(err, &staleFailure) || staleFailure.Code != cognition.ActionFailureStaleRevision {
		t.Fatalf("stale action lacks typed failure: %v", err)
	}
	stored, err := fixture.Store.Action(context.Background(), fixture.Episode, stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Failure == nil || !reflect.DeepEqual(*stored.Failure, staleFailure) {
		t.Fatalf("stale failure receipt changed: %#v", stored)
	}
	head, err := fixture.Store.Episode(context.Background(), fixture.Episode)
	if err != nil {
		t.Fatal(err)
	}
	if head.Current != first.Current {
		t.Fatalf("stale failure advanced durable state: %#v", head.Current)
	}
}
