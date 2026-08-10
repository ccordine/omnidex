package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const CognitionTraceActionSchemaV1 = "omnidex.cognition-trace-action.v1"

type CognitionTraceAction struct {
	Schema               string                         `json:"schema"`
	EpisodeID            cognition.EpisodeID            `json:"episode_id"`
	ObligationID         cognition.ObligationID         `json:"obligation_id"`
	PolicyCallID         string                         `json:"policy_call_id"`
	ReconciliationID     string                         `json:"reconciliation_id"`
	ReconciliationSHA256 string                         `json:"reconciliation_sha256"`
	ExpectedRevision     cognition.WorldRevision        `json:"expected_revision"`
	SnapshotSHA256       string                         `json:"snapshot_sha256"`
	ContextProjection    cognition.ContextProjectionRef `json:"context_projection"`
	SchemaRef            cognition.ActionSchemaRef      `json:"action_schema"`
	Decision             cognition.CognitionDecision    `json:"decision"`
	RegisteredAction     cognition.RegisteredAction     `json:"registered_action"`
	Status               CognitionActionStatus          `json:"status"`
	Failure              *cognition.ActionFailure       `json:"failure,omitempty"`
	ResultRevision       *cognition.WorldRevision       `json:"result_revision,omitempty"`
	Origin               model.StepAttemptAuthority     `json:"origin"`
}

func newCognitionTraceAction(record CognitionActionRecord) (CognitionTraceAction, error) {
	payload := CognitionTraceAction{
		Schema: CognitionTraceActionSchemaV1, EpisodeID: record.EpisodeID,
		ObligationID: record.ObligationID, PolicyCallID: record.PolicyCallID,
		ReconciliationID:     record.ReconciliationID,
		ReconciliationSHA256: record.ReconciliationSHA256,
		ExpectedRevision:     record.ExpectedRevision, SnapshotSHA256: record.SnapshotSHA256,
		ContextProjection: record.ContextProjection, SchemaRef: record.Schema,
		Decision: record.Decision.Clone(), RegisteredAction: record.Action,
		Status: record.Status, Origin: record.Origin,
	}
	if record.Failure != nil {
		failure := record.Failure.Clone()
		payload.Failure = &failure
	}
	if record.ResultRevision != nil {
		revision := *record.ResultRevision
		payload.ResultRevision = &revision
	}
	if err := payload.Validate(); err != nil {
		return CognitionTraceAction{}, err
	}
	return payload, nil
}

func (payload CognitionTraceAction) Validate() error {
	if payload.Schema != CognitionTraceActionSchemaV1 || payload.EpisodeID == "" ||
		payload.ExpectedRevision.EpisodeID != payload.EpisodeID ||
		payload.ExpectedRevision.Validate() != nil || payload.RegisteredAction.ID == "" ||
		payload.ObligationID != payload.Decision.ObligationID ||
		!reflect.DeepEqual(payload.RegisteredAction.Request, payload.Decision.Action) ||
		!reflect.DeepEqual(payload.RegisteredAction.EvidenceRefs, payload.Decision.EvidenceRefs) ||
		payload.RegisteredAction.Schema != payload.SchemaRef ||
		payload.ContextProjection.Validate() != nil ||
		!cognitionDigestPattern.MatchString(payload.SnapshotSHA256) ||
		!cognitionDigestPattern.MatchString(payload.ReconciliationSHA256) ||
		!taskLedgerExact(payload.PolicyCallID) || !taskLedgerExact(payload.ReconciliationID) {
		return fmt.Errorf("%w: sealed cognition action authority is invalid", ErrCognitionConflict)
	}
	origin := cognition.AttemptRef{
		JobID: payload.Origin.JobID, Generation: payload.Origin.Generation,
		StepID: payload.Origin.StepID, Attempt: uint64(payload.Origin.Attempt),
		WorkerID: payload.Origin.WorkerID,
	}
	if payload.Origin.Attempt <= 0 || origin.Validate() != nil ||
		payload.RegisteredAction.Actor != origin {
		return fmt.Errorf("%w: sealed cognition action origin changed", ErrCognitionConflict)
	}
	switch payload.Status {
	case CognitionActionSucceeded:
		if payload.ResultRevision == nil || payload.Failure != nil ||
			payload.ResultRevision.EpisodeID != payload.EpisodeID ||
			payload.ResultRevision.Number != payload.ExpectedRevision.Number+1 ||
			payload.ResultRevision.Validate() != nil {
			return fmt.Errorf("%w: successful sealed cognition action has invalid result", ErrCognitionConflict)
		}
	case CognitionActionFailed:
		if payload.Failure == nil || payload.ResultRevision != nil {
			return fmt.Errorf("%w: failed sealed cognition action has invalid result", ErrCognitionConflict)
		}
	default:
		return fmt.Errorf("%w: sealed cognition action is unresolved", ErrCognitionConflict)
	}
	return nil
}

func appendCognitionActionTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT actions.action_id,snapshots.call_ordinal
		FROM cognition_actions actions
		JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=actions.snapshot_sha256
		WHERE actions.episode_id=$1 ORDER BY snapshots.call_ordinal,actions.action_id
	`, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type actionIdentity struct {
		id      cognition.ActionID
		ordinal int64
	}
	ids := make([]actionIdentity, 0)
	for rows.Next() {
		var identity actionIdentity
		if err := rows.Scan(&identity.id, &identity.ordinal); err != nil {
			return nil, err
		}
		ids = append(ids, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, identity := range ids {
		record, found, err := loadCognitionActionTx(ctx, tx, identity.id, false)
		if err != nil || !found {
			return nil, fmt.Errorf("load cognition trace action %q: %v", identity.id, err)
		}
		payload, err := newCognitionTraceAction(record)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		records = append(records, cognitionTraceRecord{
			Kind: "action", CallOrdinal: identity.ordinal, Phase: 50,
			ID: string(identity.id), SHA256: cognitionPayloadSHA(raw),
		})
	}
	return records, nil
}

func loadCognitionTraceActionPayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	authority cognitionTraceRecord,
) ([]byte, error) {
	record, found, err := loadCognitionActionTx(ctx, tx, cognition.ActionID(authority.ID), false)
	if err != nil || !found || record.EpisodeID != episode.EpisodeID {
		return nil, fmt.Errorf("%w: sealed cognition action is unavailable: %v", ErrCognitionConflict, err)
	}
	payload, err := newCognitionTraceAction(record)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if cognitionPayloadSHA(raw) != authority.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition action payload changed", ErrCognitionConflict)
	}
	return raw, nil
}
