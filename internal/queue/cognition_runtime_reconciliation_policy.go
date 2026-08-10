package queue

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func cognitionRuntimeAuthority(binding cognitionruntime.Binding) (model.StepAttemptAuthority, error) {
	if err := binding.Validate(); err != nil {
		return model.StepAttemptAuthority{}, err
	}
	if binding.Attempt.Attempt > math.MaxInt64 {
		return model.StepAttemptAuthority{}, fmt.Errorf("cognition runtime attempt exceeds PostgreSQL BIGINT")
	}
	return model.StepAttemptAuthority{
		JobID: binding.Attempt.JobID, Generation: binding.Attempt.Generation,
		StepID: binding.Attempt.StepID, Attempt: int64(binding.Attempt.Attempt),
		WorkerID: binding.Attempt.WorkerID,
	}, nil
}

func loadExactCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command cognitionruntime.ReconciliationCommand,
) (string, error) {
	decisionJSON, decisionSHA, err := cognitionJSON(command.Decision)
	if err != nil || len(decisionJSON) == 0 {
		return "", err
	}
	if command.Recovery != nil {
		return loadRecoveredCognitionPolicyCallTx(ctx, tx, command, decisionSHA)
	}
	rows, err := tx.Query(ctx, `
		SELECT call_id FROM cognition_policy_calls
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND step_attempt=$5 AND worker_id=$6 AND snapshot_sha256=$7
		  AND status='accepted' AND result_json::jsonb->>'decision_sha256'=$8
		  AND projection_id=$9
		ORDER BY call_id LIMIT 2
	`, command.Binding.Episode.ID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, command.SnapshotSHA256, decisionSHA, command.Projection.ID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	ids := make([]string, 0, 2)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", fmt.Errorf(
			"%w: reconciliation requires exactly one immutable policy call, found %d",
			ErrCognitionConflict, len(ids),
		)
	}
	call, found, err := loadCognitionPolicyCallTx(ctx, tx, ids[0], false)
	if err != nil || !found || call.Result == nil {
		return "", fmt.Errorf("%w: load accepted policy call: %v", ErrCognitionConflict, err)
	}
	if call.Result.DecisionSHA256 != decisionSHA ||
		call.Result.ActionSchema != command.ActionSchema.Ref() ||
		call.Attempt.SnapshotSHA256 != command.SnapshotSHA256 ||
		call.Attempt.ContextProjection != command.Projection {
		return "", fmt.Errorf("%w: accepted policy call does not bind exact reconciliation", ErrCognitionConflict)
	}
	return ids[0], nil
}

func loadRecoveredCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionruntime.ReconciliationCommand,
	decisionSHA string,
) (string, error) {
	call, found, err := loadCognitionPolicyCallTx(ctx, tx, command.Recovery.PolicyCallID, false)
	if err != nil || !found || call.Result == nil {
		return "", fmt.Errorf("%w: load recovered accepted policy call: %v", ErrCognitionConflict, err)
	}
	if call.Result.Status != cognitionpolicy.CallResultAccepted ||
		call.Result.DecisionSHA256 != decisionSHA || call.Result.ActionSchema != command.ActionSchema.Ref() ||
		call.Attempt.SnapshotSHA256 != command.SnapshotSHA256 ||
		call.Attempt.ContextProjection != command.Projection ||
		!acceptedDecisionSourceAllowed(call.Attempt.Actor, command.Binding.Attempt) {
		return "", fmt.Errorf("%w: recovered policy call does not bind exact reconciliation", ErrCognitionConflict)
	}
	return call.Attempt.ID, nil
}

func sameQueueStepAttempt(left, right cognition.AttemptRef) bool {
	return left.JobID == right.JobID && left.Generation == right.Generation && left.StepID == right.StepID
}
