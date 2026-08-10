package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func insertCognitionDecisionAcceptanceTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	reconciliationID string,
	acceptance cognitionDecisionAcceptance,
) error {
	if err := acceptance.validate(); err != nil {
		return err
	}
	raw, rawSHA, err := cognitionJSON(acceptance)
	if err != nil {
		return err
	}
	identityRaw, identitySHA, err := cognitionJSON(acceptance.identity())
	if err != nil {
		return err
	}
	if identitySHA != acceptance.SHA256 {
		return fmt.Errorf("%w: selected decision acceptance identity changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_decision_acceptances (
			acceptance_id,acceptance_sha256,episode_id,reconciliation_id,
			ledger_id,policy_call_id,snapshot_sha256,decision_sha256,candidate_entry_id,
			accepted_entry_id,action_schema_id,action_schema_version,
			action_schema_sha256,acceptance_command_id,acceptance_command_sha256,
			identity_json,identity_json_sha256,descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, acceptance.ID, acceptance.SHA256, episode.EpisodeID, reconciliationID,
		acceptance.LedgerID, acceptance.PolicyCallID, acceptance.SnapshotSHA256, acceptance.DecisionSHA256,
		acceptance.CandidateEntryID, acceptance.AcceptedEntryID, acceptance.ActionSchema.ID,
		acceptance.ActionSchema.Version, acceptance.ActionSchema.SHA256,
		acceptance.AcceptanceCommandID, acceptance.AcceptanceCommandSHA,
		string(identityRaw), identitySHA, string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition decision acceptance: %w", err)
	}
	return nil
}

func requireCognitionDecisionAcceptanceReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	command cognitionruntime.ReconciliationCommand,
) error {
	var raw []byte
	var rawSHA string
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_decision_acceptances WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&raw, &rawSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: reconciliation lost its selected decision acceptance", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	if cognitionPayloadSHA(raw) != rawSHA {
		return fmt.Errorf("%w: selected decision acceptance payload changed", ErrCognitionConflict)
	}
	var acceptance cognitionDecisionAcceptance
	if err := json.Unmarshal(raw, &acceptance); err != nil || acceptance.validate() != nil {
		return fmt.Errorf("%w: selected decision acceptance is invalid", ErrCognitionConflict)
	}
	canonical, _, err := cognitionJSON(acceptance)
	decisionSHA, decisionErr := cognitionruntime.DecisionSHA256(command.Decision)
	if err != nil || decisionErr != nil || !bytes.Equal(canonical, raw) ||
		acceptance.SnapshotSHA256 != command.SnapshotSHA256 ||
		acceptance.DecisionSHA256 != decisionSHA || acceptance.ActionSchema != command.ActionSchema.Ref() {
		return fmt.Errorf("%w: selected decision acceptance replay changed", ErrCognitionConflict)
	}
	return nil
}
