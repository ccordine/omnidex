package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

const cognitionPostSealReplayBootstrapAuditSchemaV1 = "omnidex.cognition-postseal-replay-bootstrap-audit.v1"

type cognitionPostSealReplayBootstrapAuditAuthority struct {
	Schema               string `json:"schema"`
	EpisodeID            string `json:"episode_id"`
	JobID                int64  `json:"job_id"`
	Generation           int64  `json:"generation"`
	StepID               int64  `json:"step_id"`
	Attempt              int64  `json:"attempt"`
	WorkerID             string `json:"worker_id"`
	ObservationSHA256    string `json:"observation_sha256"`
	EvidenceID           string `json:"evidence_id"`
	TerminalTraceSHA256  string `json:"terminal_trace_sha256"`
	ProcessObservationID string `json:"process_observation_id"`
	ProcessChainSHA256   string `json:"process_chain_sha256"`
}

func insertCognitionEpisodePostSealReplayBootstrapAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) error {
	bootstrap := command.BrainBootstrap
	if err := bootstrap.Validate(); err != nil {
		return err
	}
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		ctx, tx, bootstrap.BootstrapEvidence,
	); err != nil {
		return err
	}
	processID := command.ProviderProcessActivation.Receipt.ID
	var terminalTraceSHA, processChainSHA string
	if err := tx.QueryRow(ctx, `
		SELECT terminal_trace_sha256,chain_sha256
		FROM cognition_provider_postseal_observations
		WHERE observation_id=$1 AND episode_id=$2
	`, processID, command.EpisodeID).Scan(&terminalTraceSHA, &processChainSHA); err != nil {
		return fmt.Errorf("load post-seal replay process observation: %w", err)
	}
	authority := cognitionPostSealReplayBootstrapAuditAuthority{
		Schema:    cognitionPostSealReplayBootstrapAuditSchemaV1,
		EpisodeID: string(command.EpisodeID), JobID: command.Authority.JobID,
		Generation: command.Authority.Generation, StepID: command.Authority.StepID,
		Attempt: command.Authority.Attempt, WorkerID: command.Authority.WorkerID,
		ObservationSHA256:    bootstrap.AttestedBrain.BootstrapObservation.ObservationSHA256,
		EvidenceID:           bootstrap.BootstrapEvidence.Ref.ID,
		TerminalTraceSHA256:  terminalTraceSHA,
		ProcessObservationID: processID,
		ProcessChainSHA256:   processChainSHA,
	}
	authorityJSON, err := exactjson.Canonical(authority)
	if err != nil {
		return err
	}
	auditID := "cognition_postseal_replay_bootstrap_" + cognitionPayloadSHA(authorityJSON)
	observationJSON, err := exactjson.Canonical(bootstrap.AttestedBrain.BootstrapObservation)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_episode_postseal_replay_bootstrap_audits (
			audit_id,episode_id,evidence_id,job_id,generation,step_id,step_attempt,worker_id,
			provider_observation_json,provider_observation_json_sha256,
			provider_observation_sha256,observed_at,terminal_trace_sha256,
			process_observation_id,process_chain_sha256,authority_json,authority_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (audit_id) DO NOTHING
	`, auditID, command.EpisodeID, bootstrap.BootstrapEvidence.Ref.ID,
		command.Authority.JobID, command.Authority.Generation, command.Authority.StepID,
		command.Authority.Attempt, command.Authority.WorkerID, string(observationJSON),
		cognitionPayloadSHA(observationJSON),
		bootstrap.AttestedBrain.BootstrapObservation.ObservationSHA256,
		bootstrap.AttestedBrain.BootstrapObservation.ObservedAt,
		terminalTraceSHA, processID, processChainSHA,
		string(authorityJSON), cognitionPayloadSHA(authorityJSON)); err != nil {
		return fmt.Errorf("persist post-seal replay bootstrap audit: %w", err)
	}
	var evidenceID, observation, traceSHA, persistedProcess, chainSHA, persistedAuthority string
	if err := tx.QueryRow(ctx, `
		SELECT evidence_id,provider_observation_json,terminal_trace_sha256,
		       process_observation_id,process_chain_sha256,authority_json
		FROM cognition_episode_postseal_replay_bootstrap_audits WHERE audit_id=$1
	`, auditID).Scan(
		&evidenceID, &observation, &traceSHA, &persistedProcess, &chainSHA, &persistedAuthority,
	); err != nil {
		return err
	}
	if evidenceID != bootstrap.BootstrapEvidence.Ref.ID || observation != string(observationJSON) ||
		traceSHA != terminalTraceSHA || persistedProcess != processID || chainSHA != processChainSHA ||
		persistedAuthority != string(authorityJSON) {
		return fmt.Errorf("%w: post-seal replay bootstrap audit changed", ErrCognitionConflict)
	}
	return nil
}
