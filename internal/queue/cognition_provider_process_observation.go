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
	activation cognitionpolicy.ProviderProcessActivation,
) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("provider process observation requires PostgreSQL and context")
	}
	receipt := activation.Receipt
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
	if err := activation.ValidateFor(episode.AttestedBrain); err != nil {
		return err
	}
	receiptJSON, err := exactjson.Canonical(receipt)
	if err != nil {
		return err
	}
	found, err = exactProviderProcessObservationReplayTx(
		ctx, tx, activation, string(receiptJSON), cognitionPayloadSHA(receiptJSON),
		"", CognitionProviderPostSealDirectAudit,
	)
	if err != nil {
		return err
	}
	if found {
		return tx.Commit(ctx)
	}
	postSealSource := CognitionProviderPostSealSource("")
	if episode.Status != CognitionEpisodeActive {
		postSealSource = CognitionProviderPostSealDirectAudit
	}
	if err := persistCognitionProviderProcessActivationTx(
		ctx, tx, authority, episode, activation, postSealSource,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func persistCognitionProviderProcessActivationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	activation cognitionpolicy.ProviderProcessActivation,
	postSealSource CognitionProviderPostSealSource,
) error {
	receipt := activation.Receipt
	observedAuthority, err := providerProcessObservationAuthority(receipt.Actor)
	if err != nil {
		return err
	}
	if observedAuthority != authority || episode.EpisodeID != receipt.EpisodeID ||
		episode.Authority.JobID != authority.JobID ||
		episode.Authority.Generation != authority.Generation ||
		episode.Authority.StepID != authority.StepID {
		return fmt.Errorf("%w: process observation actor differs from episode", ErrCognitionConflict)
	}
	if err := activation.ValidateFor(episode.AttestedBrain); err != nil {
		return err
	}
	receiptJSON, err := exactjson.Canonical(receipt)
	if err != nil {
		return err
	}
	receiptSHA := cognitionPayloadSHA(receiptJSON)
	found, err := exactProviderProcessObservationReplayTx(
		ctx, tx, activation, string(receiptJSON), receiptSHA, postSealSource,
	)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		ctx, tx, activation.IdentityEvidence,
	); err != nil {
		return err
	}
	switch episode.Status {
	case CognitionEpisodeActive:
		if postSealSource != "" {
			return fmt.Errorf("%w: active process observation has a post-seal source", ErrCognitionConflict)
		}
		err = insertActiveProviderProcessObservationTx(
			ctx, tx, authority, activation, receiptJSON, receiptSHA,
		)
	case CognitionEpisodeCompleted, CognitionEpisodeFailed, CognitionEpisodeCanceled:
		if !validCognitionProviderPostSealSource(postSealSource) {
			return fmt.Errorf("%w: terminal process observation source is not registered", ErrCognitionConflict)
		}
		err = insertPostSealProviderProcessObservationTx(
			ctx, tx, authority, episode, activation, postSealSource, receiptJSON, receiptSHA,
		)
	default:
		err = fmt.Errorf("%w: episode has unknown process-observation status", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	return nil
}

func exactProviderProcessObservationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	activation cognitionpolicy.ProviderProcessActivation,
	receiptJSON, receiptSHA string,
	acceptedSources ...CognitionProviderPostSealSource,
) (bool, error) {
	var existingJSON, existingSHA, evidenceID, existingSource string
	err := tx.QueryRow(ctx, `
		SELECT receipt_json,receipt_sha256,evidence_id,source_kind FROM (
			SELECT receipt_json,receipt_sha256,evidence_id,''::text AS source_kind
			FROM cognition_provider_process_observations
			WHERE observation_id=$1
			UNION ALL
			SELECT receipt_json,receipt_sha256,evidence_id,source_kind
			FROM cognition_provider_postseal_observations
			WHERE observation_id=$1
		) receipts
	`, activation.Receipt.ID).Scan(&existingJSON, &existingSHA, &evidenceID, &existingSource)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingJSON != receiptJSON || existingSHA != receiptSHA ||
		evidenceID != activation.IdentityEvidence.Ref.ID {
		return false, fmt.Errorf("%w: provider process observation replay changed", ErrCognitionConflict)
	}
	sourceAccepted := false
	for _, accepted := range acceptedSources {
		if CognitionProviderPostSealSource(existingSource) == accepted {
			sourceAccepted = true
			break
		}
	}
	if !sourceAccepted {
		return false, fmt.Errorf(
			"%w: provider process observation source changed", ErrCognitionConflict,
		)
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidenceID)
	if err != nil || !reflect.DeepEqual(persisted, activation.IdentityEvidence) {
		return false, fmt.Errorf("%w: provider process raw evidence replay changed: %v",
			ErrCognitionConflict, err)
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
