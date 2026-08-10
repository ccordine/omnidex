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

func (r *Repository) AbandonIndeterminateCognitionPolicyCall(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.PolicyCallAbandonment, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("cognition call abandonment requires PostgreSQL and context")
	}
	authority, err := cognitionRuntimeAuthority(binding)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockCognitionReplacementStepTx(ctx, tx, authority); err != nil {
		return nil, err
	}
	if replay, found, err := loadPendingCognitionAbandonmentTx(ctx, tx, binding, true); err != nil {
		return nil, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &replay, nil
	}
	callID, found, err := findStartedCognitionPolicyCallTx(ctx, tx, authority, binding.Episode.ID)
	if err != nil || !found {
		return nil, err
	}
	source, err := loadCognitionPolicyCallActorTx(ctx, tx, callID)
	if err != nil {
		return nil, err
	}
	if source == authority {
		if err := lockCurrentCognitionReplacementAttemptTx(ctx, tx, authority); err != nil {
			return nil, err
		}
		record, found, err := loadCognitionPolicyCallTx(ctx, tx, callID, true)
		if err != nil || !found || record.Status != "started" || record.Result != nil {
			return nil, fmt.Errorf("%w: indeterminate cognition call changed: %v", ErrCognitionConflict, err)
		}
		return nil, fmt.Errorf("%w: same active attempt owns indeterminate call %s", cognitionpolicy.ErrCallIndeterminate, callID)
	}
	status, err := loadSourceCognitionAttemptStatusTx(ctx, tx, source)
	if err != nil {
		return nil, err
	}
	disposition, err := runtimeSourceAttemptDisposition(status)
	if err != nil {
		return nil, err
	}
	record, found, err := loadCognitionPolicyCallTx(ctx, tx, callID, true)
	if err != nil || !found || record.Status != "started" || record.Result != nil {
		return nil, fmt.Errorf("%w: indeterminate cognition call changed: %v", ErrCognitionConflict, err)
	}
	if record.Attempt.Actor != bindingAttempt(source) {
		return nil, fmt.Errorf("%w: indeterminate cognition call actor changed", ErrCognitionConflict)
	}
	if err := lockCurrentCognitionReplacementAttemptTx(ctx, tx, authority); err != nil {
		return nil, err
	}
	if err := validateCognitionPolicyCallEpisodeTx(ctx, tx, record); err != nil {
		return nil, err
	}
	ordinal, err := cognitionPolicyCallOrdinalTx(ctx, tx, record.Attempt)
	if err != nil {
		return nil, err
	}
	abandonment, err := cognitionruntime.NewPolicyCallAbandonment(
		binding.Episode, callID, ordinal, record.AttemptSHA256, record.Attempt.SnapshotSHA256,
		record.Attempt.Actor, disposition, binding.Attempt,
	)
	if err != nil {
		return nil, err
	}
	if err := persistCognitionPolicyAbandonmentTx(ctx, tx, record, abandonment); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &abandonment, nil
}

func loadCognitionPolicyCallActorTx(
	ctx context.Context,
	tx pgx.Tx,
	callID string,
) (model.StepAttemptAuthority, error) {
	var source model.StepAttemptAuthority
	if err := tx.QueryRow(ctx, `
		SELECT job_id,generation,step_id,step_attempt,worker_id
		FROM cognition_policy_calls WHERE call_id=$1
	`, callID).Scan(
		&source.JobID, &source.Generation, &source.StepID, &source.Attempt, &source.WorkerID,
	); err != nil {
		return model.StepAttemptAuthority{}, err
	}
	return source, nil
}

func bindingAttempt(authority model.StepAttemptAuthority) cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
}

func findStartedCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
) (string, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT call_id FROM cognition_policy_calls
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND step_attempt<=$5 AND status='started'
		ORDER BY step_attempt DESC,created_at DESC,call_id LIMIT 2
	`, episodeID, authority.JobID, authority.Generation, authority.StepID, authority.Attempt)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", false, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(ids) > 1 {
		return "", false, fmt.Errorf("%w: multiple indeterminate cognition calls exist", ErrCognitionConflict)
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[0], true, nil
}

func validateCognitionPolicyCallEpisodeTx(
	ctx context.Context,
	tx pgx.Tx,
	record cognitionPolicyCallRecord,
) error {
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, record.Attempt.ExpectedRevision.EpisodeID, true)
	if err != nil || !found {
		return fmt.Errorf("%w: cognition policy episode is unavailable: %v", ErrCognitionConflict, err)
	}
	if episode.Status != CognitionEpisodeActive || episode.CurrentRevision != record.Attempt.ExpectedRevision ||
		episode.AttestedBrain.Ref != record.Attempt.Brain ||
		episode.AttestedBrain.Attestation != record.Attempt.ProviderAttestation ||
		episode.AttestedBrain.Host != record.Attempt.HostHardwareAttestation {
		return fmt.Errorf("%w: cognition policy call differs from current episode authority", ErrCognitionConflict)
	}
	var linked int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM cognition_reconciliations WHERE policy_call_id=$1)+
		       (SELECT COUNT(*) FROM cognition_actions WHERE policy_call_id=$1)
	`, record.Attempt.ID).Scan(&linked); err != nil {
		return err
	}
	if linked != 0 {
		return fmt.Errorf("%w: policy call already owns reconciliation or action authority", ErrCognitionConflict)
	}
	return nil
}

func cognitionPolicyCallOrdinalTx(
	ctx context.Context,
	tx pgx.Tx,
	attempt cognitionpolicy.CallAttempt,
) (uint64, error) {
	var ordinal int64
	if err := tx.QueryRow(ctx, `
		SELECT call_ordinal FROM cognition_runtime_snapshots WHERE snapshot_sha256=$1
	`, attempt.SnapshotSHA256).Scan(&ordinal); err != nil {
		return 0, err
	}
	if ordinal <= 0 {
		return 0, fmt.Errorf("%w: policy call ordinal is invalid", ErrCognitionConflict)
	}
	return uint64(ordinal), nil
}

func runtimeSourceAttemptDisposition(status model.StepAttemptStatus) (cognitionruntime.SourceAttemptDisposition, error) {
	switch status {
	case model.StepAttemptExpired:
		return cognitionruntime.SourceAttemptExpired, nil
	case model.StepAttemptSuperseded:
		return cognitionruntime.SourceAttemptSuperseded, nil
	default:
		return "", fmt.Errorf("%w: policy source attempt status %q cannot authorize abandonment", ErrCognitionConflict, status)
	}
}

func persistCognitionPolicyAbandonmentTx(
	ctx context.Context,
	tx pgx.Tx,
	record cognitionPolicyCallRecord,
	abandonment cognitionruntime.PolicyCallAbandonment,
) error {
	raw, jsonDigest, err := cognitionJSON(abandonment)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_policy_call_abandonments (
			abandonment_id,abandonment_sha256,source_call_id,episode_id,job_id,generation,step_id,
			source_attempt,source_worker_id,source_attempt_sha256,source_snapshot_sha256,
			source_disposition,recovery_attempt,recovery_worker_id,call_ordinal,
			descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, abandonment.ID, abandonment.SHA256, abandonment.CallID, abandonment.Episode.ID,
		abandonment.SourceActor.JobID, abandonment.SourceActor.Generation, abandonment.SourceActor.StepID,
		int64(abandonment.SourceActor.Attempt), abandonment.SourceActor.WorkerID,
		abandonment.SourceAttemptSHA256, abandonment.SourceSnapshotSHA256, abandonment.SourceDisposition,
		int64(abandonment.RecoveryActor.Attempt), abandonment.RecoveryActor.WorkerID,
		int64(abandonment.CallOrdinal), string(raw), jsonDigest)
	if err != nil {
		return fmt.Errorf("insert cognition policy abandonment: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE cognition_policy_calls SET status='abandoned',finished_at=clock_timestamp()
		WHERE call_id=$1 AND status='started' AND result_json IS NULL
	`, record.Attempt.ID)
	if err != nil {
		return fmt.Errorf("abandon cognition policy call: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: cognition policy call lost abandonment authority", ErrCognitionConflict)
	}
	return nil
}

func decodeCognitionPolicyAbandonment(raw []byte) (cognitionruntime.PolicyCallAbandonment, error) {
	var value cognitionruntime.PolicyCallAbandonment
	if err := json.Unmarshal(raw, &value); err != nil {
		return cognitionruntime.PolicyCallAbandonment{}, err
	}
	if err := value.Validate(); err != nil {
		return cognitionruntime.PolicyCallAbandonment{}, err
	}
	return value, nil
}
