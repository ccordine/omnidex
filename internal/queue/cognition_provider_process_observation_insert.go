package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertActiveProviderProcessObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	activation cognitionpolicy.ProviderProcessActivation,
	receiptJSON []byte,
	receiptSHA string,
) error {
	receipt := activation.Receipt
	sequence, err := nextProviderProcessObservationSequenceTx(
		ctx, tx, receipt.EpisodeID, false,
	)
	if err != nil {
		return err
	}
	stableJSON, stableJSONSHA, observationJSON, observationJSONSHA, err :=
		providerProcessObservationPayloads(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_provider_process_observations (
			observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			purpose,sequence,stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,provider_attestation_sha256,challenge_sha256,
			observed_at,receipt_json,receipt_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, receipt.ID, activation.IdentityEvidence.Ref.ID, receipt.EpisodeID, authority.JobID, authority.Generation,
		authority.StepID, authority.Attempt, authority.WorkerID, receipt.Purpose, sequence,
		string(stableJSON), stableJSONSHA, receipt.StableBrain.SHA256,
		string(observationJSON), observationJSONSHA, receipt.Observation.ObservationSHA256,
		receipt.Observation.AttestationSHA256, receipt.Observation.ChallengeSHA256,
		receipt.Observation.ObservedAt, string(receiptJSON), receiptSHA)
	if err != nil {
		return fmt.Errorf("record active provider process observation %q: %w", receipt.ID, err)
	}
	return nil
}

func insertPostSealProviderProcessObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	activation cognitionpolicy.ProviderProcessActivation,
	postSealSource CognitionProviderPostSealSource,
	receiptJSON []byte,
	receiptSHA string,
) error {
	receipt := activation.Receipt
	seal, err := loadCognitionTerminalSealTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return err
	}
	sequence, err := nextProviderProcessObservationSequenceTx(
		ctx, tx, receipt.EpisodeID, true,
	)
	if err != nil {
		return err
	}
	previous := seal.TraceSHA256
	if sequence > 1 {
		if err := tx.QueryRow(ctx, `
			SELECT chain_sha256 FROM cognition_provider_postseal_observations
			WHERE episode_id=$1 AND sequence=$2 FOR UPDATE
		`, receipt.EpisodeID, sequence-1).Scan(&previous); err != nil {
			return err
		}
	}
	chain := providerPostSealChainSHA(
		seal.TraceSHA256, previous, sequence, postSealSource, receiptSHA,
	)
	stableJSON, stableJSONSHA, observationJSON, observationJSONSHA, err :=
		providerProcessObservationPayloads(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_provider_postseal_observations (
			observation_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			purpose,sequence,source_kind,terminal_trace_sha256,previous_chain_sha256,chain_sha256,
			stable_brain_json,stable_brain_json_sha256,stable_brain_sha256,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,provider_attestation_sha256,challenge_sha256,
			observed_at,receipt_json,receipt_sha256
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25
		)
	`, receipt.ID, activation.IdentityEvidence.Ref.ID, receipt.EpisodeID, authority.JobID, authority.Generation,
		authority.StepID, authority.Attempt, authority.WorkerID, receipt.Purpose, sequence,
		postSealSource, seal.TraceSHA256, previous, chain, string(stableJSON), stableJSONSHA,
		receipt.StableBrain.SHA256, string(observationJSON), observationJSONSHA,
		receipt.Observation.ObservationSHA256, receipt.Observation.AttestationSHA256,
		receipt.Observation.ChallengeSHA256, receipt.Observation.ObservedAt,
		string(receiptJSON), receiptSHA)
	if err != nil {
		return fmt.Errorf("record post-seal provider process observation %q: %w", receipt.ID, err)
	}
	return nil
}

func providerProcessObservationPayloads(
	receipt cognitionpolicy.ProviderProcessObservation,
) ([]byte, string, []byte, string, error) {
	stableJSON, err := exactjson.Canonical(receipt.StableBrain)
	if err != nil {
		return nil, "", nil, "", err
	}
	observationJSON, err := exactjson.Canonical(receipt.Observation)
	return stableJSON, cognitionPayloadSHA(stableJSON), observationJSON,
		cognitionPayloadSHA(observationJSON), err
}

func nextProviderProcessObservationSequenceTx(
	ctx context.Context,
	tx pgx.Tx,
	episode any,
	postSeal bool,
) (int64, error) {
	query := "SELECT COALESCE(MAX(sequence),0)+1 FROM cognition_provider_process_observations WHERE episode_id=$1"
	if postSeal {
		query = "SELECT COALESCE(MAX(sequence),0)+1 FROM cognition_provider_postseal_observations WHERE episode_id=$1"
	}
	var sequence int64
	if err := tx.QueryRow(ctx, query, episode).Scan(&sequence); err != nil {
		return 0, err
	}
	return sequence, nil
}

func providerPostSealChainSHA(
	trace, previous string,
	sequence int64,
	source CognitionProviderPostSealSource,
	receipt string,
) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s:%s:%d:%s:%s", trace, previous, sequence, source, receipt,
	)))
	return hex.EncodeToString(digest[:])
}
