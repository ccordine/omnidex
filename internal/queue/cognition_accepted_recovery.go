package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// RecoverAcceptedCognitionDecision authorizes one stranded accepted policy
// decision for its exact active actor or a later replacement. It never
// prepares a new snapshot or consumes another policy call.
func (r *Repository) RecoverAcceptedCognitionDecision(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.AcceptedDecisionRecovery, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("accepted cognition recovery requires PostgreSQL and context")
	}
	authority, err := cognitionRuntimeAuthority(binding)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return nil, err
	} else if status != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "accepted cognition recovery actor is not running", nil)
	}
	callID, candidateFound, err := findStrandedAcceptedCognitionCallTx(
		ctx, tx, authority, binding.Episode.ID,
	)
	if err != nil || !candidateFound {
		return nil, err
	}
	stored, replayFound, err := loadAcceptedCognitionRecoveryTx(ctx, tx, binding, callID, true)
	if err != nil {
		return nil, err
	}
	_, episode, graph, err := loadCognitionSnapshotAuthorityTx(ctx, tx, CognitionRuntimeSnapshotCommand{
		Authority: authority, EpisodeID: binding.Episode.ID,
	})
	if err != nil {
		return nil, err
	}
	recovery, err := buildAcceptedCognitionRecoveryTx(ctx, tx, binding, authority, episode, graph, callID)
	if err != nil {
		return nil, err
	}
	if replayFound {
		if err := validateAcceptedCognitionRecoveryReplay(stored, recovery); err != nil {
			return nil, err
		}
	} else if err := insertAcceptedCognitionRecoveryTx(ctx, tx, recovery); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	copy := cloneAcceptedCognitionRecovery(recovery)
	return &copy, nil
}

func findStrandedAcceptedCognitionCallTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
) (string, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT calls.call_id
		FROM cognition_policy_calls calls
		LEFT JOIN cognition_actions actions ON actions.policy_call_id=calls.call_id
		WHERE calls.episode_id=$1 AND calls.job_id=$2 AND calls.generation=$3 AND calls.step_id=$4
		  AND (calls.step_attempt<$5 OR (calls.step_attempt=$5 AND calls.worker_id=$6))
		  AND calls.status='accepted' AND actions.action_id IS NULL
		ORDER BY calls.step_attempt DESC,calls.created_at DESC,calls.call_id
		LIMIT 2
	`, episodeID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	ids := make([]string, 0, 2)
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
	if len(ids) == 0 {
		return "", false, nil
	}
	if len(ids) != 1 {
		return "", false, fmt.Errorf(
			"%w: replacement attempt found %d stranded accepted decisions", ErrCognitionConflict, len(ids),
		)
	}
	return ids[0], true, nil
}

func loadAcceptedCognitionRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	binding cognitionruntime.Binding,
	callID string,
	lock bool,
) (acceptedCognitionRecoveryRecord, bool, error) {
	query := `
		SELECT recovery_id,recovery_sha256,source_policy_call_id,authority_json,authority_json_sha256
		FROM cognition_accepted_decision_recoveries
		WHERE episode_id=$1 AND job_id=$2 AND generation=$3 AND step_id=$4
		  AND recovery_attempt=$5 AND recovery_worker_id=$6 AND source_policy_call_id=$7`
	if lock {
		query += ` FOR UPDATE`
	}
	var record acceptedCognitionRecoveryRecord
	err := tx.QueryRow(ctx, query, binding.Episode.ID, binding.Attempt.JobID,
		binding.Attempt.Generation, binding.Attempt.StepID, int64(binding.Attempt.Attempt),
		binding.Attempt.WorkerID, callID).Scan(
		&record.ID, &record.SHA256, &record.SourcePolicyCallID,
		&record.AuthorityJSON, &record.AuthorityJSONSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return acceptedCognitionRecoveryRecord{}, false, nil
	}
	return record, err == nil, err
}

type acceptedCognitionRecoveryRecord struct {
	ID, SHA256, SourcePolicyCallID string
	AuthorityJSON                  []byte
	AuthorityJSONSHA256            string
}
