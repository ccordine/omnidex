package queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RecordCognitionProviderProcessObservation(
	ctx context.Context,
	receipt cognitionpolicy.ProviderProcessObservation,
) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("provider process observation requires PostgreSQL and context")
	}
	authority, err := providerProcessObservationAuthority(receipt.Actor)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, receipt.EpisodeID, true)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, receipt.EpisodeID)
	}
	if episode.Authority.JobID != authority.JobID ||
		episode.Authority.Generation != authority.Generation ||
		episode.Authority.StepID != authority.StepID {
		return fmt.Errorf("%w: process observation actor differs from episode", ErrCognitionConflict)
	}
	if err := receipt.ValidateFor(episode.AttestedBrain); err != nil {
		return err
	}
	receiptJSON, err := exactjson.Canonical(receipt)
	if err != nil {
		return err
	}
	receiptSHA := cognitionPayloadSHA(receiptJSON)
	found, err = exactProviderProcessObservationReplayTx(
		ctx, tx, receipt.ID, string(receiptJSON), receiptSHA,
	)
	if err != nil {
		return err
	}
	if found {
		return tx.Commit(ctx)
	}
	switch episode.Status {
	case CognitionEpisodeActive:
		err = insertActiveProviderProcessObservationTx(
			ctx, tx, authority, receipt, receiptJSON, receiptSHA,
		)
	case CognitionEpisodeCompleted, CognitionEpisodeFailed, CognitionEpisodeCanceled:
		err = insertPostSealProviderProcessObservationTx(
			ctx, tx, authority, episode, receipt, receiptJSON, receiptSHA,
		)
	default:
		err = fmt.Errorf("%w: episode has unknown process-observation status", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func exactProviderProcessObservationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	id, receiptJSON, receiptSHA string,
) (bool, error) {
	var existingJSON, existingSHA string
	err := tx.QueryRow(ctx, `
		SELECT receipt_json,receipt_sha256 FROM (
			SELECT receipt_json,receipt_sha256 FROM cognition_provider_process_observations
			WHERE observation_id=$1
			UNION ALL
			SELECT receipt_json,receipt_sha256 FROM cognition_provider_postseal_observations
			WHERE observation_id=$1
		) receipts
	`, id).Scan(&existingJSON, &existingSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingJSON != receiptJSON || existingSHA != receiptSHA {
		return false, fmt.Errorf("%w: provider process observation replay changed", ErrCognitionConflict)
	}
	return true, nil
}

func providerProcessObservationAuthority(
	actor cognition.AttemptRef,
) (model.StepAttemptAuthority, error) {
	if err := actor.Validate(); err != nil {
		return model.StepAttemptAuthority{}, err
	}
	if actor.Attempt > math.MaxInt64 {
		return model.StepAttemptAuthority{}, fmt.Errorf("provider process actor exceeds PostgreSQL BIGINT")
	}
	return model.StepAttemptAuthority{
		JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
		Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
	}, nil
}

func stableAttestedBrainEqual(left, right cognitionpolicy.AttestedBrain) bool {
	leftStable, leftErr := left.StableAuthority()
	rightStable, rightErr := right.StableAuthority()
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftStable, rightStable)
}
