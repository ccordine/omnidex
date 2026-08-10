package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/jackc/pgx/v5"
)

const cognitionAttentionOutcomeSchemaV1 = "omnidex.cognition-attention-outcome.v1"

type cognitionAttentionOutcomePayload struct {
	Schema      string                             `json:"schema"`
	Request     cognition.AttentionRequest         `json:"request"`
	Disposition cognitionstate.AdvisoryDisposition `json:"disposition"`
	Reason      string                             `json:"reason"`
}

func insertCognitionAttentionOutcomesTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	outcomes []cognitionstate.AdvisoryOutcome,
) error {
	for index, outcome := range outcomes {
		payload := cognitionAttentionOutcomePayload{
			Schema: cognitionAttentionOutcomeSchemaV1, Request: outcome.Request,
			Disposition: outcome.Disposition, Reason: outcome.Reason,
		}
		raw, sha, err := cognitionJSON(payload)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO cognition_attention_outcomes (
				outcome_id,outcome_sha256,reconciliation_id,request_index,
				operation,scope,disposition,reason,outcome_json,outcome_json_sha256
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, "cognition_attention_outcome_"+sha, sha, reconciliationID, index,
			outcome.Request.Operation, outcome.Request.Scope, outcome.Disposition,
			outcome.Reason, string(raw), sha)
		if err != nil {
			return fmt.Errorf("insert cognition attention outcome %d: %w", index, err)
		}
	}
	return nil
}

func requireCognitionAttentionOutcomeReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	reconciliationID string,
	requests []cognition.AttentionRequest,
) error {
	rows, err := tx.Query(ctx, `
		SELECT request_index,outcome_json,outcome_json_sha256
		FROM cognition_attention_outcomes
		WHERE reconciliation_id=$1 ORDER BY request_index
	`, reconciliationID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var index int
		var raw []byte
		var sha string
		if err := rows.Scan(&index, &raw, &sha); err != nil {
			return err
		}
		if index != count || index >= len(requests) || cognitionPayloadSHA(raw) != sha {
			return fmt.Errorf("%w: attention outcome replay identity changed", ErrCognitionConflict)
		}
		var payload cognitionAttentionOutcomePayload
		if err := json.Unmarshal(raw, &payload); err != nil ||
			payload.Schema != cognitionAttentionOutcomeSchemaV1 || payload.Request != requests[index] ||
			!validCognitionAttentionDisposition(payload.Disposition) || payload.Reason == "" {
			return fmt.Errorf("%w: attention outcome replay changed", ErrCognitionConflict)
		}
		canonical, _, err := cognitionJSON(payload)
		if err != nil || !bytes.Equal(canonical, raw) {
			return fmt.Errorf("%w: attention outcome replay is not canonical", ErrCognitionConflict)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(requests) {
		return fmt.Errorf("%w: attention outcome replay count changed", ErrCognitionConflict)
	}
	return nil
}

func validCognitionAttentionDisposition(value cognitionstate.AdvisoryDisposition) bool {
	switch value {
	case cognitionstate.AdvisoryAccepted, cognitionstate.AdvisoryRejectedProtected,
		cognitionstate.AdvisoryRejectedCapacity, cognitionstate.AdvisoryRejectedUnavailable:
		return true
	default:
		return false
	}
}
