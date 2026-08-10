package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionTransitionCannotOutrunActionProjection(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "transition-reverse-action")
	action := prepareCognitionGuardAction(t, fixture, "transition-reverse-action")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("d"))
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
	}
	if err := insertCognitionTransitionTx(
		fixture.Context, tx, fixture.Authority, fixture.EpisodeID, transition,
	); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(fixture.Context, `
		UPDATE cognition_episodes
		SET current_revision=2,current_revision_sha256=$2,action_count=action_count+1,
		    version=version+1,updated_at=clock_timestamp()
		WHERE episode_id=$1
	`, fixture.EpisodeID, next.SHA256)
	assertCognitionTransactionRejected(t, fixture.Context, tx, err, "transition ahead of action projection")
}
