package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func loadCognitionTracePolicyTimingValueTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	id string,
) (CognitionTracePolicyTiming, error) {
	callID := strings.TrimSuffix(id, ":timing")
	if callID == id {
		return CognitionTracePolicyTiming{}, fmt.Errorf("%w: policy timing ID is invalid", ErrCognitionConflict)
	}
	payload := CognitionTracePolicyTiming{Schema: CognitionTracePolicyTimingSchemaV1}
	if err := tx.QueryRow(ctx, `
		SELECT call_id,status,created_at,finished_at FROM cognition_policy_calls
		WHERE episode_id=$1 AND call_id=$2
	`, episode, callID).Scan(
		&payload.CallID, &payload.Status, &payload.StartedAt, &payload.FinishedAt,
	); err != nil {
		return CognitionTracePolicyTiming{}, err
	}
	if err := payload.Validate(); err != nil {
		return CognitionTracePolicyTiming{}, err
	}
	return payload, nil
}

func loadCognitionTraceRecoveryValueTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	id string,
) (CognitionTraceAcceptedDecisionRecovery, error) {
	var payload CognitionTraceAcceptedDecisionRecovery
	var recoveryAttempt, sourceAttempt int64
	payload.Schema = CognitionTraceRecoverySchemaV1
	var recoverySHA, callID string
	err := tx.QueryRow(ctx, `
		SELECT recovery_sha256,source_policy_call_id,job_id,generation,step_id,recovery_attempt,
		       recovery_worker_id,source_attempt,source_worker_id,snapshot_sha256,graph_version,
		       graph_sha256,projection_id,obligation_node_id,decision_sha256,action_schema_id,
		       action_schema_version,action_schema_sha256,created_at
		FROM cognition_accepted_decision_recoveries WHERE episode_id=$1 AND recovery_id=$2
	`, episode, id).Scan(
		&recoverySHA, &callID, &payload.Binding.Attempt.JobID,
		&payload.Binding.Attempt.Generation, &payload.Binding.Attempt.StepID,
		&recoveryAttempt, &payload.Binding.Attempt.WorkerID, &sourceAttempt,
		&payload.SourceActor.WorkerID, &payload.SnapshotSHA256, &payload.GraphVersion,
		&payload.GraphSHA256, &payload.ContextProjection.ID, &payload.ObligationID,
		&payload.DecisionSHA256, &payload.ActionSchema.ID, &payload.ActionSchema.Version,
		&payload.ActionSchema.SHA256, &payload.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return payload, fmt.Errorf("%w: cognition recovery trace is unavailable", ErrCognitionConflict)
	}
	if err != nil {
		return payload, err
	}
	payload.Binding.Episode = cognition.EpisodeRef{ID: episode}
	if recoveryAttempt <= 0 || sourceAttempt <= 0 {
		return payload, fmt.Errorf("%w: cognition recovery attempts are invalid", ErrCognitionConflict)
	}
	payload.Binding.Attempt.Attempt = uint64(recoveryAttempt)
	payload.SourceActor.Attempt = uint64(sourceAttempt)
	payload.SourceActor.JobID, payload.SourceActor.Generation, payload.SourceActor.StepID =
		payload.Binding.Attempt.JobID, payload.Binding.Attempt.Generation, payload.Binding.Attempt.StepID
	payload.Recovery = cognitionruntime.AcceptedDecisionRecoveryRef{
		ID: id, SHA256: recoverySHA, PolicyCallID: callID,
	}
	projection, err := loadContextProjectionTx(ctx, tx, string(payload.ContextProjection.ID))
	if err != nil {
		return payload, err
	}
	payload.ContextProjection = cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.Projection.ID), SHA256: projection.Projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.Projection.WorkingSetID),
		WorkingSetVersion: projection.Projection.WorkingSetVersion,
		RendererVersion:   projection.Projection.RendererVersion,
	}
	if err := payload.Validate(); err != nil {
		return payload, err
	}
	return payload, nil
}

func (payload CognitionTraceAcceptedDecisionRecovery) Validate() error {
	if payload.Schema != CognitionTraceRecoverySchemaV1 || payload.Recovery.Validate() != nil ||
		payload.Binding.Validate() != nil || payload.SourceActor.Validate() != nil ||
		payload.SourceActor.JobID != payload.Binding.Attempt.JobID ||
		payload.SourceActor.Generation != payload.Binding.Attempt.Generation ||
		payload.SourceActor.StepID != payload.Binding.Attempt.StepID ||
		!acceptedDecisionSourceAllowed(payload.SourceActor, payload.Binding.Attempt) ||
		!cognitionDigestPattern.MatchString(payload.SnapshotSHA256) || payload.GraphVersion == 0 ||
		!cognitionDigestPattern.MatchString(payload.GraphSHA256) ||
		payload.ContextProjection.Validate() != nil || payload.ObligationID == "" ||
		!cognitionDigestPattern.MatchString(payload.DecisionSHA256) || payload.ActionSchema.Validate() != nil ||
		payload.CreatedAt.IsZero() {
		return fmt.Errorf("%w: cognition recovery trace payload is invalid", ErrCognitionConflict)
	}
	return nil
}
