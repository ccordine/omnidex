package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

const cognitionEpisodeReplayBootstrapSchemaV1 = "omnidex.cognition-episode-replay-bootstrap.v1"

type cognitionEpisodeReplayBootstrapAuthority struct {
	Schema               string `json:"schema"`
	EpisodeID            string `json:"episode_id"`
	JobID                int64  `json:"job_id"`
	Generation           int64  `json:"generation"`
	StepID               int64  `json:"step_id"`
	Attempt              int64  `json:"attempt"`
	WorkerID             string `json:"worker_id"`
	ObservationSHA256    string `json:"observation_sha256"`
	EvidenceID           string `json:"evidence_id"`
	ProcessObservationID string `json:"process_observation_id"`
	ProcessReceiptSHA256 string `json:"process_receipt_sha256"`
	ProcessEvidenceID    string `json:"process_evidence_id"`
}

type cognitionEpisodeReplayBootstrapProjection struct {
	ID                    string
	EvidenceID            string
	ObservationJSON       string
	ObservationJSONSHA256 string
	ObservationSHA256     string
	ObservedAt            time.Time
	ProcessObservationID  string
	ProcessReceiptSHA256  string
	ProcessEvidenceID     string
	AuthorityJSON         string
	AuthoritySHA256       string
}

func cognitionEpisodeReplayBootstrapProjectionFor(
	command CognitionEpisodeStart,
) (cognitionEpisodeReplayBootstrapProjection, error) {
	bootstrap := command.BrainBootstrap
	if err := bootstrap.Validate(); err != nil {
		return cognitionEpisodeReplayBootstrapProjection{}, err
	}
	observationJSON, err := exactjson.Canonical(bootstrap.AttestedBrain.BootstrapObservation)
	if err != nil {
		return cognitionEpisodeReplayBootstrapProjection{}, err
	}
	if err := command.ProviderProcessActivation.ValidateFor(bootstrap.AttestedBrain); err != nil {
		return cognitionEpisodeReplayBootstrapProjection{}, err
	}
	processReceiptJSON, err := exactjson.Canonical(command.ProviderProcessActivation.Receipt)
	if err != nil {
		return cognitionEpisodeReplayBootstrapProjection{}, err
	}
	authority := cognitionEpisodeReplayBootstrapAuthority{
		Schema:    cognitionEpisodeReplayBootstrapSchemaV1,
		EpisodeID: string(command.EpisodeID), JobID: command.Authority.JobID,
		Generation: command.Authority.Generation, StepID: command.Authority.StepID,
		Attempt: command.Authority.Attempt, WorkerID: command.Authority.WorkerID,
		ObservationSHA256:    bootstrap.AttestedBrain.BootstrapObservation.ObservationSHA256,
		EvidenceID:           bootstrap.BootstrapEvidence.Ref.ID,
		ProcessObservationID: command.ProviderProcessActivation.Receipt.ID,
		ProcessReceiptSHA256: cognitionPayloadSHA(processReceiptJSON),
		ProcessEvidenceID:    command.ProviderProcessActivation.IdentityEvidence.Ref.ID,
	}
	authorityJSON, err := exactjson.Canonical(authority)
	if err != nil {
		return cognitionEpisodeReplayBootstrapProjection{}, err
	}
	authoritySHA := cognitionPayloadSHA(authorityJSON)
	return cognitionEpisodeReplayBootstrapProjection{
		ID:                    "cognition_replay_bootstrap_" + authoritySHA,
		EvidenceID:            bootstrap.BootstrapEvidence.Ref.ID,
		ObservationJSON:       string(observationJSON),
		ObservationJSONSHA256: cognitionPayloadSHA(observationJSON),
		ObservationSHA256:     bootstrap.AttestedBrain.BootstrapObservation.ObservationSHA256,
		ObservedAt:            bootstrap.AttestedBrain.BootstrapObservation.ObservedAt,
		ProcessObservationID:  authority.ProcessObservationID,
		ProcessReceiptSHA256:  authority.ProcessReceiptSHA256,
		ProcessEvidenceID:     authority.ProcessEvidenceID,
		AuthorityJSON:         string(authorityJSON),
		AuthoritySHA256:       authoritySHA,
	}, nil
}

func insertCognitionEpisodeReplayBootstrapEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) error {
	bootstrap := command.BrainBootstrap
	projection, err := cognitionEpisodeReplayBootstrapProjectionFor(command)
	if err != nil {
		return err
	}
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		ctx, tx, bootstrap.BootstrapEvidence,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_episode_replay_provider_identity_evidence (
			replay_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,observed_at,process_observation_id,
			process_receipt_sha256,process_evidence_id,authority_json,authority_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (replay_id) DO NOTHING
	`, projection.ID, command.EpisodeID, projection.EvidenceID,
		command.Authority.JobID, command.Authority.Generation, command.Authority.StepID,
		command.Authority.Attempt, command.Authority.WorkerID, projection.ObservationJSON,
		projection.ObservationJSONSHA256, projection.ObservationSHA256,
		projection.ObservedAt, projection.ProcessObservationID,
		projection.ProcessReceiptSHA256, projection.ProcessEvidenceID,
		projection.AuthorityJSON, projection.AuthoritySHA256); err != nil {
		return fmt.Errorf("persist cognition replay bootstrap evidence: %w", err)
	}
	var persistedEvidence, persistedObservation, persistedAuthority string
	if err := tx.QueryRow(ctx, `
		SELECT evidence_id,provider_observation_json,authority_json
		FROM cognition_episode_replay_provider_identity_evidence WHERE replay_id=$1
	`, projection.ID).Scan(
		&persistedEvidence, &persistedObservation, &persistedAuthority,
	); err != nil {
		return err
	}
	if persistedEvidence != projection.EvidenceID ||
		persistedObservation != projection.ObservationJSON ||
		persistedAuthority != projection.AuthorityJSON {
		return fmt.Errorf("%w: cognition replay bootstrap authority changed", ErrCognitionConflict)
	}
	return nil
}
