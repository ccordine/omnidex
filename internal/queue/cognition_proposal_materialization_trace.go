package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func loadCognitionProposalMaterializationTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	record cognitionTraceRecord,
) ([]byte, error) {
	var raw []byte
	var payloadSHA, reconciliationID string
	if err := tx.QueryRow(ctx, `
		SELECT payload_json,payload_json_sha256,reconciliation_id
		FROM cognition_proposal_materializations
		WHERE episode_id=$1 AND materialization_id=$2 AND proposal_index=$3 AND call_ordinal=$4
	`, episode.EpisodeID, record.ID, record.Sequence, record.CallOrdinal).Scan(
		&raw, &payloadSHA, &reconciliationID,
	); err != nil {
		return nil, fmt.Errorf("load sealed cognition proposal materialization: %w", err)
	}
	value, err := DecodeCognitionProposalMaterialization(raw, payloadSHA)
	if err != nil || payloadSHA != record.SHA256 || value.ReconciliationID != reconciliationID ||
		int64(value.ProposalIndex) != record.Sequence || int64(value.CallOrdinal) != record.CallOrdinal {
		return nil, fmt.Errorf("%w: sealed proposal materialization tuple changed: %v", ErrCognitionConflict, err)
	}
	var commandRaw, commandSHA, receiptRaw, receiptSHA string
	if err := tx.QueryRow(ctx, `
		SELECT command_json,command_sha256,receipt_json,receipt_sha256
		FROM cognition_reconciliations WHERE episode_id=$1 AND reconciliation_id=$2
	`, episode.EpisodeID, reconciliationID).Scan(
		&commandRaw, &commandSHA, &receiptRaw, &receiptSHA,
	); err != nil {
		return nil, err
	}
	var command cognitionruntime.ReconciliationCommand
	var receipt cognitionruntime.ReconciliationReceipt
	if json.Unmarshal([]byte(commandRaw), &command) != nil || json.Unmarshal([]byte(receiptRaw), &receipt) != nil ||
		cognitionPayloadSHA([]byte(commandRaw)) != commandSHA || cognitionPayloadSHA([]byte(receiptRaw)) != receiptSHA ||
		receipt.ValidateFor(command) != nil || receipt.ID != reconciliationID {
		return nil, fmt.Errorf("%w: proposal materialization reconciliation changed", ErrCognitionConflict)
	}
	wantCommandRaw, _, err := cognitionJSON(command)
	if err != nil || !bytes.Equal([]byte(commandRaw), wantCommandRaw) {
		return nil, fmt.Errorf("%w: proposal materialization reconciliation is not exact", ErrCognitionConflict)
	}
	wantReceiptRaw, _, err := cognitionJSON(receipt)
	if err != nil || !bytes.Equal([]byte(receiptRaw), wantReceiptRaw) {
		return nil, fmt.Errorf("%w: proposal materialization receipt is not exact", ErrCognitionConflict)
	}
	snapshot, callOrdinal, err := loadCognitionProposalMaterializationSnapshotTx(
		ctx, tx, episode, value.SnapshotSHA256,
	)
	if err != nil || callOrdinal != value.CallOrdinal {
		return nil, fmt.Errorf("%w: rederive proposal materialization snapshot: %v", ErrCognitionConflict, err)
	}
	prepared := CognitionRuntimeSnapshotRecord{
		Prepared: cognitionruntime.PreparedSnapshot{Snapshot: snapshot}, CallOrdinal: callOrdinal,
	}
	if err := requireCognitionProposalMaterializationReplayTx(
		ctx, tx, episode, prepared, command, receipt,
	); err != nil {
		return nil, err
	}
	return append([]byte(nil), raw...), nil
}
