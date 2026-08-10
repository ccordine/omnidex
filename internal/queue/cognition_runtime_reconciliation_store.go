package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func loadCognitionEvidenceMaterialsTx(
	ctx context.Context,
	tx pgx.Tx,
	refs []cognition.EvidenceRef,
) ([]cognitionstate.EvidenceMaterial, error) {
	materials := make([]cognitionstate.EvidenceMaterial, 0, len(refs))
	for _, ref := range refs {
		observation, err := loadCognitionObservationTx(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		materials = append(materials, cognitionstate.EvidenceMaterial{Ref: ref, Content: observation.Content})
	}
	return materials, nil
}

func loadCognitionReconciliationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionruntime.ReconciliationCommand,
) (cognitionruntime.ReconciliationReceipt, bool, error) {
	var commandJSON, receiptJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT command_json,receipt_json FROM cognition_reconciliations WHERE snapshot_sha256=$1
	`, command.SnapshotSHA256).Scan(&commandJSON, &receiptJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionruntime.ReconciliationReceipt{}, false, nil
	}
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, false, err
	}
	var persisted cognitionruntime.ReconciliationCommand
	var receipt cognitionruntime.ReconciliationReceipt
	if err := json.Unmarshal(commandJSON, &persisted); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, false, err
	}
	if err := json.Unmarshal(receiptJSON, &receipt); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, false, err
	}
	if !reflect.DeepEqual(persisted, command) || receipt.ValidateFor(command) != nil {
		return cognitionruntime.ReconciliationReceipt{}, false,
			fmt.Errorf("%w: reconciliation replay changed content", ErrCognitionConflict)
	}
	return receipt, true, nil
}

func insertCognitionReconciliationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	policyCallID string,
	command cognitionruntime.ReconciliationCommand,
	receipt cognitionruntime.ReconciliationReceipt,
) error {
	commandJSON, commandSHA, err := cognitionJSON(command)
	if err != nil {
		return err
	}
	receiptJSON, receiptJSONSHA, err := cognitionJSON(receipt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_reconciliations (
			reconciliation_id,reconciliation_sha256,episode_id,job_id,generation,step_id,
			actor_attempt,actor_worker_id,snapshot_sha256,policy_call_id,decision_sha256,
			action_schema_id,action_schema_version,action_schema_sha256,ledger_version,
			working_set_version,command_json,command_sha256,receipt_json,receipt_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`, receipt.ID, receipt.SHA256, episodeID, authority.JobID, authority.Generation,
		authority.StepID, authority.Attempt, authority.WorkerID, command.SnapshotSHA256,
		policyCallID, receipt.DecisionSHA256, command.ActionSchema.ID, command.ActionSchema.Version,
		command.ActionSchema.SHA256, int64(receipt.LedgerVersion), int64(receipt.WorkingSetVersion),
		string(commandJSON), commandSHA, string(receiptJSON), receiptJSONSHA)
	if err != nil {
		return fmt.Errorf("insert cognition reconciliation: %w", err)
	}
	return nil
}
