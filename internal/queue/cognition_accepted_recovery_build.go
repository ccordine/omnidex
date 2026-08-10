package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func buildAcceptedCognitionRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	binding cognitionruntime.Binding,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	callID string,
) (cognitionruntime.AcceptedDecisionRecovery, error) {
	call, found, err := loadCognitionPolicyCallTx(ctx, tx, callID, true)
	if err != nil || !found || call.Result == nil || call.Result.Status != cognitionpolicy.CallResultAccepted {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery policy call is unavailable: %v", ErrCognitionConflict, err,
		)
	}
	sourceAuthority, err := cognitionPolicyCallAuthority(call.Attempt)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	if !acceptedDecisionSourceAllowed(call.Attempt.Actor, binding.Attempt) {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted decision source is not the exact actor or a prior attempt",
			ErrCognitionConflict,
		)
	}
	prepared, err := loadCognitionPreparedSnapshotBySHATx(
		ctx, tx, sourceAuthority, episode, graph, call.Attempt.SnapshotSHA256,
	)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	schema, err := acceptedCognitionActionSchema(episode.ActionCatalog, call.Result.ActionSchema)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	if call.ResponseEvidence == nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery lacks exact model response evidence", ErrCognitionConflict,
		)
	}
	decision, err := cognition.DecodeCognitionDecision(call.ResponseEvidence.Content, schema)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: decode accepted recovery decision: %v", ErrCognitionConflict, err,
		)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(decision)
	if err != nil || decisionSHA != call.Result.DecisionSHA256 {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery decision hash changed", ErrCognitionConflict,
		)
	}
	reconciliation, err := loadCognitionReconciliationReplayByCallTx(ctx, tx, callID)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	recovery, err := cognitionruntime.NewAcceptedDecisionRecovery(
		binding, callID, prepared.Prepared, decision, schema, reconciliation,
	)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	return recovery, nil
}

func acceptedDecisionSourceAllowed(source, current cognition.AttemptRef) bool {
	if !sameQueueStepAttempt(source, current) || source.Attempt > current.Attempt {
		return false
	}
	return source.Attempt < current.Attempt || source == current
}

func acceptedCognitionActionSchema(
	catalog cognition.ActionCatalog,
	ref cognition.ActionSchemaRef,
) (cognition.ActionSchema, error) {
	for _, schema := range catalog.Schemas {
		if schema.Ref() == ref {
			return schema.Clone(), nil
		}
	}
	return cognition.ActionSchema{}, fmt.Errorf(
		"%w: accepted recovery action schema is absent from the catalog", ErrCognitionConflict,
	)
}

func loadCognitionReconciliationReplayByCallTx(
	ctx context.Context,
	tx pgx.Tx,
	callID string,
) (*cognitionruntime.ReconciliationReplay, error) {
	var commandJSON, receiptJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT command_json,receipt_json FROM cognition_reconciliations WHERE policy_call_id=$1
	`, callID).Scan(&commandJSON, &receiptJSON)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var replay cognitionruntime.ReconciliationReplay
	if err := cognitionDecodeExact(commandJSON, &replay.Command); err != nil {
		return nil, fmt.Errorf("decode accepted recovery reconciliation command: %w", err)
	}
	if err := cognitionDecodeExact(receiptJSON, &replay.Receipt); err != nil {
		return nil, fmt.Errorf("decode accepted recovery reconciliation receipt: %w", err)
	}
	if err := replay.Receipt.ValidateFor(replay.Command); err != nil {
		return nil, fmt.Errorf("%w: persisted accepted reconciliation: %v", ErrCognitionConflict, err)
	}
	return &replay, nil
}

func cognitionDecodeExact(raw []byte, target any) error {
	return json.Unmarshal(raw, target)
}
