package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func exactPriorCognitionEpisodeStartInvocationTx(
	ctx context.Context,
	tx pgx.Tx,
	existing CognitionEpisode,
	command CognitionEpisodeStart,
) (bool, error) {
	if command.Authority == existing.Authority {
		return exactOriginalCognitionEpisodeStartInvocationTx(ctx, tx, existing, command)
	}
	return exactActiveCognitionEpisodeReplayInvocationTx(ctx, tx, command)
}

func exactOriginalCognitionEpisodeStartInvocationTx(
	ctx context.Context,
	tx pgx.Tx,
	existing CognitionEpisode,
	command CognitionEpisodeStart,
) (bool, error) {
	if !reflect.DeepEqual(command.BrainBootstrap.AttestedBrain, existing.AttestedBrain) {
		return false, fmt.Errorf(
			"%w: cognition episode original invocation Brain changed", ErrCognitionConflict,
		)
	}
	var evidenceID string
	if err := tx.QueryRow(ctx, `SELECT evidence_id
		FROM cognition_episode_provider_identity_evidence WHERE episode_id=$1`,
		command.EpisodeID,
	).Scan(&evidenceID); err != nil {
		return false, fmt.Errorf(
			"%w: cognition episode original invocation lacks its bootstrap: %v",
			ErrCognitionConflict, err,
		)
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidenceID)
	if err != nil || evidenceID != command.BrainBootstrap.BootstrapEvidence.Ref.ID ||
		!reflect.DeepEqual(persisted, command.BrainBootstrap.BootstrapEvidence) {
		return false, fmt.Errorf(
			"%w: cognition episode original invocation bootstrap changed: %v",
			ErrCognitionConflict, err,
		)
	}
	processFound, sequence, err := exactActiveCognitionEpisodeProcessTx(
		ctx, tx, command.ProviderProcessActivation,
	)
	if err != nil || !processFound || sequence != 1 {
		return false, fmt.Errorf(
			"%w: cognition episode original invocation process changed: %v",
			ErrCognitionConflict, err,
		)
	}
	return true, nil
}

func exactActiveCognitionEpisodeReplayInvocationTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) (bool, error) {
	bootstrapFound, err := exactActiveCognitionEpisodeReplayBootstrapTx(ctx, tx, command)
	if err != nil {
		return false, err
	}
	processFound, _, err := exactActiveCognitionEpisodeProcessTx(
		ctx, tx, command.ProviderProcessActivation,
	)
	if err != nil {
		return false, err
	}
	if bootstrapFound != processFound {
		return false, fmt.Errorf(
			"%w: cognition episode replay invocation is only partially durable",
			ErrCognitionConflict,
		)
	}
	return bootstrapFound, nil
}

func exactActiveCognitionEpisodeReplayBootstrapTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) (bool, error) {
	projection, err := cognitionEpisodeReplayBootstrapProjectionFor(command)
	if err != nil {
		return false, err
	}
	var episodeID, evidenceID, workerID string
	var observationJSON, observationJSONSHA, observationSHA string
	var processObservationID, processReceiptSHA, processEvidenceID string
	var authorityJSON, authoritySHA string
	var jobID, generation, stepID, attempt int64
	var observedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
		       provider_observation_json,provider_observation_json_sha256,
		       provider_observation_sha256,observed_at,process_observation_id,
		       process_receipt_sha256,process_evidence_id,authority_json,authority_sha256
		FROM cognition_episode_replay_provider_identity_evidence WHERE replay_id=$1
	`, projection.ID).Scan(
		&episodeID, &evidenceID, &jobID, &generation, &stepID, &attempt, &workerID,
		&observationJSON, &observationJSONSHA, &observationSHA, &observedAt,
		&processObservationID, &processReceiptSHA, &processEvidenceID,
		&authorityJSON, &authoritySHA,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		var actorReplayExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM cognition_episode_replay_provider_identity_evidence
			WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
			  AND step_attempt=$5 AND worker_id=$6
		)`, command.EpisodeID, command.Authority.JobID, command.Authority.Generation,
			command.Authority.StepID, command.Authority.Attempt,
			command.Authority.WorkerID,
		).Scan(&actorReplayExists); err != nil {
			return false, err
		}
		if actorReplayExists {
			return false, fmt.Errorf(
				"%w: cognition episode replay bootstrap authority changed", ErrCognitionConflict,
			)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if episodeID != string(command.EpisodeID) || evidenceID != projection.EvidenceID ||
		jobID != command.Authority.JobID || generation != command.Authority.Generation ||
		stepID != command.Authority.StepID || attempt != command.Authority.Attempt ||
		workerID != command.Authority.WorkerID ||
		observationJSON != projection.ObservationJSON ||
		observationJSONSHA != projection.ObservationJSONSHA256 ||
		observationSHA != projection.ObservationSHA256 || !observedAt.Equal(projection.ObservedAt) ||
		processObservationID != projection.ProcessObservationID ||
		processReceiptSHA != projection.ProcessReceiptSHA256 ||
		processEvidenceID != projection.ProcessEvidenceID ||
		authorityJSON != projection.AuthorityJSON || authoritySHA != projection.AuthoritySHA256 {
		return false, fmt.Errorf(
			"%w: cognition episode replay bootstrap authority changed", ErrCognitionConflict,
		)
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidenceID)
	if err != nil || !reflect.DeepEqual(persisted, command.BrainBootstrap.BootstrapEvidence) {
		return false, fmt.Errorf(
			"%w: cognition episode replay bootstrap raw evidence changed: %v",
			ErrCognitionConflict, err,
		)
	}
	return true, nil
}

func exactActiveCognitionEpisodeProcessTx(
	ctx context.Context,
	tx pgx.Tx,
	activation cognitionpolicy.ProviderProcessActivation,
) (bool, int64, error) {
	receiptJSON, err := exactjson.Canonical(activation.Receipt)
	if err != nil {
		return false, 0, err
	}
	var existingJSON, existingSHA, evidenceID string
	var sequence int64
	err = tx.QueryRow(ctx, `
		SELECT receipt_json,receipt_sha256,evidence_id,sequence
		FROM cognition_provider_process_observations WHERE observation_id=$1
	`, activation.Receipt.ID).Scan(&existingJSON, &existingSHA, &evidenceID, &sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if existingJSON != string(receiptJSON) || existingSHA != cognitionPayloadSHA(receiptJSON) ||
		evidenceID != activation.IdentityEvidence.Ref.ID {
		return false, 0, fmt.Errorf(
			"%w: cognition episode replay process observation changed", ErrCognitionConflict,
		)
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidenceID)
	if err != nil || !reflect.DeepEqual(persisted, activation.IdentityEvidence) {
		return false, 0, fmt.Errorf(
			"%w: cognition episode replay process raw evidence changed: %v",
			ErrCognitionConflict, err,
		)
	}
	return true, sequence, nil
}
