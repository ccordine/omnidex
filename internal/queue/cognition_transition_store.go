package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionTransitionTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	transition cognition.Transition,
) error {
	transitionJSON, transitionSHA, err := cognitionJSON(transition)
	if err != nil {
		return err
	}
	transitionID := cognitionTransitionID(episodeID, transitionSHA)
	var previousRevision any
	var previousSHA any
	if transition.Previous != nil {
		previousRevision = int64(transition.Previous.Number)
		previousSHA = transition.Previous.SHA256
	}
	var actionID any
	if transition.ActionID != "" {
		actionID = transition.ActionID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_transitions (
			transition_id,episode_id,job_id,generation,step_id,revision,previous_revision,
			previous_revision_sha256,current_revision_sha256,action_id,actor_attempt,
			actor_worker_id,transition_json,transition_sha256,cost,terminal,public_outcome
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, transitionID, episodeID, authority.JobID, authority.Generation, authority.StepID,
		int64(transition.Current.Number), previousRevision, previousSHA, transition.Current.SHA256,
		actionID, authority.Attempt, authority.WorkerID, string(transitionJSON), transitionSHA,
		transition.Cost, transition.Terminal, transition.PublicOutcome); err != nil {
		return fmt.Errorf("insert cognition transition: %w", err)
	}
	for position, observation := range transition.Observations {
		raw, _, err := cognitionJSON(observation)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_transition_observations (
				transition_id,position,observation_id,content_sha256,observation_json
			) VALUES ($1,$2,$3,$4,$5)
		`, transitionID, position, observation.ID, observation.ContentSHA256, string(raw)); err != nil {
			return fmt.Errorf("insert cognition observation %q: %w", observation.ID, err)
		}
	}
	for position, effect := range transition.Effects {
		raw, _, err := cognitionJSON(effect)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_transition_effects (
				transition_id,position,effect_kind,content_sha256,effect_json
			) VALUES ($1,$2,$3,$4,$5)
		`, transitionID, position, effect.Kind, effect.ContentSHA256, string(raw)); err != nil {
			return fmt.Errorf("insert cognition effect %d: %w", position, err)
		}
	}
	result, err := tx.Exec(ctx, `
		UPDATE cognition_transitions SET normalized_sealed_at=clock_timestamp()
		WHERE transition_id=$1 AND normalized_sealed_at IS NULL
	`, transitionID)
	if err != nil {
		return fmt.Errorf("seal cognition transition: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("cognition transition %q could not be sealed exactly once", transitionID)
	}
	return nil
}
