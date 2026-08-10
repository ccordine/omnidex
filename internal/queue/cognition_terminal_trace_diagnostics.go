package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

const (
	CognitionTracePolicyTimingSchemaV1 = "omnidex.cognition-policy-timing-trace.v1"
	CognitionTraceRecoverySchemaV1     = "omnidex.cognition-recovery-trace.v1"
)

type CognitionTracePolicyTiming struct {
	Schema     string                           `json:"schema"`
	CallID     string                           `json:"call_id"`
	Status     cognitionpolicy.CallResultStatus `json:"status"`
	StartedAt  time.Time                        `json:"started_at"`
	FinishedAt *time.Time                       `json:"finished_at,omitempty"`
}

type CognitionTraceAcceptedDecisionRecovery struct {
	Schema            string                                       `json:"schema"`
	Recovery          cognitionruntime.AcceptedDecisionRecoveryRef `json:"recovery"`
	Binding           cognitionruntime.Binding                     `json:"binding"`
	SourceActor       cognition.AttemptRef                         `json:"source_actor"`
	SnapshotSHA256    string                                       `json:"snapshot_sha256"`
	GraphVersion      uint64                                       `json:"graph_version"`
	GraphSHA256       string                                       `json:"graph_sha256"`
	ContextProjection cognition.ContextProjectionRef               `json:"context_projection"`
	ObligationID      cognition.ObligationID                       `json:"obligation_id"`
	DecisionSHA256    string                                       `json:"decision_sha256"`
	ActionSchema      cognition.ActionSchemaRef                    `json:"action_schema"`
	CreatedAt         time.Time                                    `json:"created_at"`
}

func appendCognitionDiagnosticTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT calls.call_id,calls.status,calls.created_at,calls.finished_at,snapshots.call_ordinal
		FROM cognition_policy_calls calls
		JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
		WHERE calls.episode_id=$1 ORDER BY snapshots.call_ordinal,calls.call_id
	`, episode)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var payload CognitionTracePolicyTiming
		var ordinal int64
		payload.Schema = CognitionTracePolicyTimingSchemaV1
		if err := rows.Scan(
			&payload.CallID, &payload.Status, &payload.StartedAt, &payload.FinishedAt, &ordinal,
		); err != nil {
			rows.Close()
			return nil, err
		}
		if err := payload.Validate(); err != nil {
			rows.Close()
			return nil, err
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("encode cognition policy timing trace: %w", err)
		}
		records = append(records, cognitionTraceRecord{
			Kind: "policy_timing", CallOrdinal: ordinal, Phase: 32,
			ID: payload.CallID + ":timing", SHA256: cognitionPayloadSHA(raw),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return appendCognitionRecoveryTraceRecordsTx(ctx, tx, episode, records)
}

func (payload CognitionTracePolicyTiming) Validate() error {
	terminal := payload.Status == cognitionpolicy.CallResultAccepted ||
		payload.Status == cognitionpolicy.CallResultRejected ||
		payload.Status == cognitionpolicy.CallResultFailed || payload.Status == "abandoned"
	if payload.Schema != CognitionTracePolicyTimingSchemaV1 || !taskLedgerExact(payload.CallID) ||
		payload.StartedAt.IsZero() ||
		(payload.Status == "started" && payload.FinishedAt != nil) ||
		(terminal && (payload.FinishedAt == nil || payload.FinishedAt.Before(payload.StartedAt))) ||
		(!terminal && payload.Status != "started") {
		return fmt.Errorf("%w: cognition policy timing is invalid", ErrCognitionConflict)
	}
	return nil
}

func appendCognitionRecoveryTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT recoveries.recovery_id,snapshots.call_ordinal,
		       CASE WHEN reconciliations.created_at IS NOT NULL
		                  AND reconciliations.created_at<=recoveries.created_at THEN 42 ELSE 35 END
		FROM cognition_accepted_decision_recoveries recoveries
		JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=recoveries.snapshot_sha256
		LEFT JOIN cognition_reconciliations reconciliations
		  ON reconciliations.policy_call_id=recoveries.source_policy_call_id
		WHERE recoveries.episode_id=$1 ORDER BY snapshots.call_ordinal,recoveries.recovery_attempt
	`, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var ordinal, phase int64
		if err := rows.Scan(&id, &ordinal, &phase); err != nil {
			return nil, err
		}
		payload, err := loadCognitionTraceRecoveryValueTx(ctx, tx, episode, id)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode accepted decision recovery trace: %w", err)
		}
		records = append(records, cognitionTraceRecord{
			Kind: "accepted_decision_recovery", CallOrdinal: ordinal, Phase: int(phase),
			Sequence: int64(payload.Binding.Attempt.Attempt), ID: id, SHA256: cognitionPayloadSHA(raw),
		})
	}
	return records, rows.Err()
}

func loadCognitionTraceDiagnosticPayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	record cognitionTraceRecord,
) ([]byte, error) {
	var value any
	if record.Kind == "policy_timing" {
		payload, err := loadCognitionTracePolicyTimingValueTx(ctx, tx, episode, record.ID)
		if err != nil {
			return nil, err
		}
		value = payload
	} else {
		payload, err := loadCognitionTraceRecoveryValueTx(ctx, tx, episode, record.ID)
		if err != nil {
			return nil, err
		}
		value = payload
	}
	raw, err := json.Marshal(value)
	if err != nil || cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: cognition diagnostic trace payload changed", ErrCognitionConflict)
	}
	return raw, nil
}
