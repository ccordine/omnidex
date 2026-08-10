package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/jackc/pgx/v5"
)

func insertCognitionBeliefRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	reconciliationID string,
	materialization cognitionstate.BeliefRevisionMaterialization,
) error {
	if err := materialization.Validate(); err != nil {
		return err
	}
	raw, rawSHA, err := cognitionJSON(materialization)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_belief_revisions (
			revision_id,revision_sha256,episode_id,job_id,generation,step_id,
			reconciliation_id,source_snapshot_sha256,source_decision_sha256,
			ledger_id,expected_ledger_sha256,expected_ledger_version,
			target_uri,target_version,target_sha256,result_ledger_sha256,
			descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, materialization.ID, materialization.SHA256, episode.EpisodeID,
		episode.Authority.JobID, episode.Authority.Generation, episode.Authority.StepID,
		reconciliationID, materialization.SourceSnapshotSHA256,
		materialization.SourceDecisionSHA256, materialization.LedgerID,
		materialization.ExpectedLedgerSHA256, int64(materialization.ExpectedVersion),
		materialization.TargetRef.URI, materialization.TargetRef.Version,
		materialization.TargetRef.SHA256, materialization.ResultLedgerSHA256,
		string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition belief revision: %w", err)
	}
	return nil
}

func requireCognitionBeliefRevisionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	command cognitionruntime.ReconciliationCommand,
) error {
	var raw []byte
	var rawSHA string
	err := tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_belief_revisions WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&raw, &rawSHA)
	wantsRevision := len(command.Decision.Proposals) == 1 &&
		command.Decision.Proposals[0].Kind == cognition.ProposalRevision
	if errors.Is(err, pgx.ErrNoRows) {
		if !wantsRevision {
			return nil
		}
		return fmt.Errorf("%w: reconciliation lost its belief revision", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	if !wantsRevision || cognitionPayloadSHA(raw) != rawSHA {
		return fmt.Errorf("%w: reconciliation gained or changed a belief revision", ErrCognitionConflict)
	}
	var persisted cognitionstate.BeliefRevisionMaterialization
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted.Validate() != nil {
		return fmt.Errorf("%w: persisted belief revision is invalid", ErrCognitionConflict)
	}
	wantRaw, _, err := cognitionJSON(persisted)
	if err != nil || !bytes.Equal(wantRaw, raw) {
		return fmt.Errorf("%w: persisted belief revision is not canonical", ErrCognitionConflict)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(command.Decision)
	if err != nil || persisted.SourceSnapshotSHA256 != command.SnapshotSHA256 ||
		persisted.SourceDecisionSHA256 != decisionSHA {
		return fmt.Errorf("%w: belief revision replay differs from its decision", ErrCognitionConflict)
	}
	return nil
}
