package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func TestPostgresCognitionTransitionRejectsActionFromAnotherEpisode(t *testing.T) {
	first := startTaskGenerationRetirementFixture(t, "cross-episode-action-owner")
	action := prepareCognitionGuardAction(t, first, "cross-episode-action-owner")
	second := startTaskGenerationRetirementFixtureIn(
		t, first.Repository, first.Pool, first.Context, "cross-episode-transition-owner",
	)
	tx, err := second.Pool.Begin(second.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	next, err := cognition.NewWorldRevision(second.EpisodeID, 2, cognitionTestDigest("e"))
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(second.Start.Transition.Current),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
	}
	if _, err := tx.Exec(second.Context, `
		UPDATE cognition_episodes
		SET current_revision=2,current_revision_sha256=$2,action_count=action_count+1,
		    version=version+1,updated_at=clock_timestamp()
		WHERE episode_id=$1
	`, second.EpisodeID, next.SHA256); err != nil {
		t.Fatal(err)
	}
	err = insertCognitionTransitionTx(
		second.Context, tx, second.Authority, second.EpisodeID, transition,
	)
	assertCognitionTransactionRejected(t, second.Context, tx, err, "cross-episode action transition")
}

func TestPostgresCognitionActionEventCannotOutrunActionProjection(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "event-reverse-projection")
	action := prepareCognitionGuardAction(t, fixture, "event-reverse-projection")
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action.Action, action.ExpectedRevision,
		"The public precondition was not satisfied.", []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	err = insertCognitionActionEventTx(
		fixture.Context, tx, action.Action.ID, fixture.Authority, CognitionActionFailed, failure,
	)
	assertCognitionTransactionRejected(t, fixture.Context, tx, err, "event ahead of action projection")
}

func TestPostgresCognitionTransitionRejectsMismatchedNormalizedEvidence(t *testing.T) {
	for _, kind := range []string{"observation", "effect"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixture(t, "normalized-"+kind)
			action := prepareCognitionGuardAction(t, fixture, "normalized-"+kind)
			tx, err := fixture.Pool.Begin(fixture.Context)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			err = insertMismatchedCognitionTransition(t, fixture, action, tx, kind)
			assertCognitionTransactionRejected(t, fixture.Context, tx, err, "mismatched normalized "+kind)
		})
	}
}

func TestPostgresCognitionNormalizedEvidenceCannotAppendAfterTransitionCommit(t *testing.T) {
	for _, kind := range []string{"observation", "effect"} {
		t.Run(kind, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixture(t, "post-commit-"+kind)
			var transitionID string
			if err := fixture.Pool.QueryRow(fixture.Context, `
				SELECT transition_id FROM cognition_transitions
				WHERE episode_id=$1 AND revision=1
			`, fixture.EpisodeID).Scan(&transitionID); err != nil {
				t.Fatal(err)
			}
			tx, err := fixture.Pool.Begin(fixture.Context)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if kind == "observation" {
				value, err := cognition.NewObservation(
					"post-commit-observation", fixture.Start.Transition.Current,
					"record", "late immutable evidence",
				)
				if err != nil {
					t.Fatal(err)
				}
				raw, _, err := cognitionJSON(value)
				if err == nil {
					_, err = tx.Exec(fixture.Context, `
						INSERT INTO cognition_transition_observations (
							transition_id,position,observation_id,content_sha256,observation_json
						) VALUES ($1,0,$2,$3,$4)
					`, transitionID, value.ID, value.ContentSHA256, string(raw))
				}
				assertCognitionTransactionRejected(t, fixture.Context, tx, err, "post-commit observation")
				return
			}
			value, err := cognition.NewEffect(
				"post-commit-action", fixture.Start.Transition.Current,
				cognition.EffectNoChange, "late immutable effect",
			)
			if err != nil {
				t.Fatal(err)
			}
			raw, _, err := cognitionJSON(value)
			if err == nil {
				_, err = tx.Exec(fixture.Context, `
					INSERT INTO cognition_transition_effects (
						transition_id,position,effect_kind,content_sha256,effect_json
					) VALUES ($1,0,$2,$3,$4)
				`, transitionID, value.Kind, value.ContentSHA256, string(raw))
			}
			assertCognitionTransactionRejected(t, fixture.Context, tx, err, "post-commit effect")
		})
	}
}

func TestPostgresActiveCognitionEpisodeRequiresRevisionOneTransition(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "missing-start-transition")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	insertCognitionEpisodeWithoutStartTransition(t, fixture, tx)
	assertCognitionTransactionRejected(t, ctx, tx, nil, "active episode without revision-one transition")
}

func insertMismatchedCognitionTransition(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	action CognitionActionRecord,
	tx pgx.Tx,
	kind string,
) error {
	t.Helper()
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("f"))
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
	}
	if kind == "observation" {
		observation, err := cognition.NewActionObservation(
			"observation-expected", action.Action.ID, next, "record", "expected content",
		)
		if err != nil {
			t.Fatal(err)
		}
		transition.Observations = []cognition.Observation{observation}
	} else {
		effect, err := cognition.NewEffect(
			action.Action.ID, next, cognition.EffectStateChanged, "expected effect",
		)
		if err != nil {
			t.Fatal(err)
		}
		transition.Effects = []cognition.Effect{effect}
	}
	transitionJSON, transitionSHA, err := cognitionJSON(transition)
	if err != nil {
		t.Fatal(err)
	}
	transitionID := cognitionTransitionID(fixture.EpisodeID, transitionSHA)
	if _, err := tx.Exec(fixture.Context, `
		INSERT INTO cognition_transitions (
			transition_id,episode_id,job_id,generation,step_id,revision,previous_revision,
			previous_revision_sha256,current_revision_sha256,action_id,actor_attempt,
			actor_worker_id,transition_json,transition_sha256,cost,terminal,public_outcome
		) VALUES ($1,$2,$3,$4,$5,2,1,$6,$7,$8,$9,$10,$11,$12,0,FALSE,'')
	`, transitionID, fixture.EpisodeID, fixture.Authority.JobID, fixture.Authority.Generation,
		fixture.Authority.StepID, action.ExpectedRevision.SHA256, next.SHA256, action.Action.ID,
		fixture.Authority.Attempt, fixture.Authority.WorkerID, string(transitionJSON), transitionSHA); err != nil {
		return err
	}
	if kind == "observation" {
		wrong, err := cognition.NewActionObservation(
			"observation-wrong", action.Action.ID, next, "record", "different content",
		)
		if err != nil {
			t.Fatal(err)
		}
		raw, _, err := cognitionJSON(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(fixture.Context, `
			INSERT INTO cognition_transition_observations (
				transition_id,position,observation_id,content_sha256,observation_json
			) VALUES ($1,0,$2,$3,$4)
		`, transitionID, wrong.ID, wrong.ContentSHA256, string(raw)); err != nil {
			return err
		}
	} else {
		wrong, err := cognition.NewEffect(
			action.Action.ID, next, cognition.EffectNoChange, "different effect",
		)
		if err != nil {
			t.Fatal(err)
		}
		raw, _, err := cognitionJSON(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(fixture.Context, `
			INSERT INTO cognition_transition_effects (
				transition_id,position,effect_kind,content_sha256,effect_json
			) VALUES ($1,0,$2,$3,$4)
		`, transitionID, wrong.Kind, wrong.ContentSHA256, string(raw)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fixture.Context, `
		UPDATE cognition_transitions SET normalized_sealed_at=clock_timestamp()
		WHERE transition_id=$1 AND normalized_sealed_at IS NULL
	`, transitionID); err != nil {
		return err
	}
	if _, err := tx.Exec(fixture.Context, `
		UPDATE cognition_actions SET status='succeeded',result_revision=2,
		       result_revision_sha256=$2,resolved_at=clock_timestamp()
		WHERE action_id=$1
	`, action.Action.ID, next.SHA256); err != nil {
		return err
	}
	if err := insertCognitionActionEventTx(
		fixture.Context, tx, action.Action.ID, fixture.Authority, CognitionActionSucceeded, transition,
	); err != nil {
		return err
	}
	_, err = tx.Exec(fixture.Context, `
		UPDATE cognition_episodes
		SET current_revision=2,current_revision_sha256=$2,action_count=action_count+1,
		    version=version+1,updated_at=clock_timestamp()
		WHERE episode_id=$1
	`, fixture.EpisodeID, next.SHA256)
	return err
}

func assertCognitionTransactionRejected(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	operationErr error,
	label string,
) {
	t.Helper()
	if operationErr == nil {
		operationErr = tx.Commit(ctx)
	}
	if operationErr == nil {
		t.Fatalf("PostgreSQL accepted %s", label)
	}
}

func cognitionRevisionPointer(revision cognition.WorldRevision) *cognition.WorldRevision {
	copy := revision
	return &copy
}
