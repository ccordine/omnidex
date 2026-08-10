package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type cognitionPolicyCallRecord struct {
	Attempt       cognitionpolicy.CallAttempt
	AttemptSHA256 string
	Status        string
	Result        *cognitionpolicy.CallResult
}

func cognitionPolicyCallAuthority(attempt cognitionpolicy.CallAttempt) (model.StepAttemptAuthority, error) {
	if attempt.Actor.Attempt > math.MaxInt64 {
		return model.StepAttemptAuthority{}, fmt.Errorf("cognition policy attempt exceeds PostgreSQL BIGINT")
	}
	return model.StepAttemptAuthority{
		JobID: attempt.Actor.JobID, Generation: attempt.Actor.Generation,
		StepID: attempt.Actor.StepID, Attempt: int64(attempt.Actor.Attempt), WorkerID: attempt.Actor.WorkerID,
	}, nil
}

func loadCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	callID string,
	lock bool,
) (cognitionPolicyCallRecord, bool, error) {
	query := `SELECT attempt_json,attempt_sha256,status,result_json FROM cognition_policy_calls WHERE call_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var attemptJSON []byte
	var resultJSON []byte
	var record cognitionPolicyCallRecord
	err := tx.QueryRow(ctx, query, callID).Scan(
		&attemptJSON, &record.AttemptSHA256, &record.Status, &resultJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionPolicyCallRecord{}, false, nil
	}
	if err != nil {
		return cognitionPolicyCallRecord{}, false, err
	}
	if err := json.Unmarshal(attemptJSON, &record.Attempt); err != nil {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode cognition policy attempt: %w", err)
	}
	_, expectedSHA, err := cognitionJSON(record.Attempt)
	if err != nil || expectedSHA != record.AttemptSHA256 || record.Attempt.Validate() != nil {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf("%w: persisted policy attempt is invalid", ErrCognitionConflict)
	}
	if len(resultJSON) > 0 {
		var result cognitionpolicy.CallResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode cognition policy result: %w", err)
		}
		if err := result.Validate(record.Attempt); err != nil {
			return cognitionPolicyCallRecord{}, false, fmt.Errorf("%w: persisted policy result: %v", ErrCognitionConflict, err)
		}
		record.Result = &result
	}
	if record.Result == nil && record.Status != "started" && record.Status != "abandoned" {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf(
			"%w: persisted policy terminal call has no result", ErrCognitionConflict,
		)
	}
	if record.Result != nil && string(record.Result.Status) != record.Status {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf(
			"%w: persisted policy call status differs from its result", ErrCognitionConflict,
		)
	}
	return record, true, nil
}

func exactCognitionCallReservation(
	attempt cognitionpolicy.CallAttempt,
	persisted cognitionPolicyCallRecord,
) (cognitionpolicy.CallReservation, error) {
	_, attemptSHA, err := cognitionJSON(attempt)
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	if attemptSHA != persisted.AttemptSHA256 || !reflect.DeepEqual(attempt, persisted.Attempt) {
		return cognitionpolicy.CallReservation{}, fmt.Errorf(
			"%w: cognition policy call replay changed the exact attempt", ErrCognitionConflict,
		)
	}
	if persisted.Status == "abandoned" {
		return cognitionpolicy.CallReservation{}, fmt.Errorf(
			"%w: cognition policy call was code-abandoned", ErrCognitionConflict,
		)
	}
	reservation := cognitionpolicy.CallReservation{Attempt: persisted.Attempt.Clone()}
	if persisted.Result != nil {
		result := persisted.Result.Clone()
		reservation.ExistingResult = &result
	}
	if err := reservation.ValidateFor(attempt); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	return reservation, nil
}

func requireExactCognitionPolicySnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
) error {
	var episodeID cognition.EpisodeID
	var jobID, generation, stepID, actorAttempt, ordinal, revision int64
	var workerID, revisionSHA, obligationID, projectionID, workingSetID string
	var budgetJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT episode_id,job_id,generation,step_id,actor_attempt,actor_worker_id,
		       call_ordinal,expected_revision,expected_revision_sha256,obligation_node_id,
		       projection_id,working_set_id,runtime_budget_json
		FROM cognition_runtime_snapshots WHERE snapshot_sha256=$1
	`, attempt.SnapshotSHA256).Scan(
		&episodeID, &jobID, &generation, &stepID, &actorAttempt, &workerID, &ordinal,
		&revision, &revisionSHA, &obligationID, &projectionID, &workingSetID, &budgetJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: policy call has no prepared snapshot", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	var budget cognition.RuntimeBudget
	if err := json.Unmarshal(budgetJSON, &budget); err != nil {
		return fmt.Errorf("decode prepared policy budget: %w", err)
	}
	if episodeID != attempt.ExpectedRevision.EpisodeID || jobID != authority.JobID ||
		generation != authority.Generation || stepID != authority.StepID || actorAttempt != authority.Attempt ||
		workerID != authority.WorkerID || revision <= 0 || uint64(revision) != attempt.ExpectedRevision.Number ||
		revisionSHA != attempt.ExpectedRevision.SHA256 || obligationID != string(attempt.ObligationID) ||
		projectionID != string(attempt.ContextProjection.ID) || workingSetID != string(attempt.ContextProjection.WorkingSetID) ||
		budget != attempt.RuntimeBudget || ordinal <= 0 {
		return fmt.Errorf("%w: policy call differs from exact prepared snapshot", ErrCognitionConflict)
	}
	projection, err := loadContextProjectionTx(ctx, tx, projectionID)
	if err != nil {
		return err
	}
	ref := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.Projection.ID), SHA256: projection.Projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.Projection.WorkingSetID),
		WorkingSetVersion: projection.Projection.WorkingSetVersion,
		RendererVersion:   projection.Projection.RendererVersion,
	}
	if ref != attempt.ContextProjection {
		return fmt.Errorf("%w: policy call projection identity changed", ErrCognitionConflict)
	}
	return nil
}

func insertCognitionPolicyCallTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
) error {
	budgetJSON, _, err := cognitionJSON(attempt.RuntimeBudget)
	if err != nil {
		return err
	}
	brainJSON, _, err := cognitionJSON(attempt.Brain)
	if err != nil {
		return err
	}
	attemptJSON, attemptSHA, err := cognitionJSON(attempt)
	if err != nil {
		return err
	}
	_, budgetSHA, err := cognitionJSON(attempt.RuntimeBudget)
	if err != nil {
		return err
	}
	_, brainSHA, err := cognitionJSON(attempt.Brain)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_policy_calls (
			call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			snapshot_sha256,projection_id,working_set_id,expected_revision,
			expected_revision_sha256,obligation_node_id,runtime_budget_json,runtime_budget_sha256,
			brain_json,brain_sha256,attempt_json,attempt_sha256,
			status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'started'
		)
	`, attempt.ID, attempt.ExpectedRevision.EpisodeID, authority.JobID, authority.Generation,
		authority.StepID, authority.Attempt, authority.WorkerID, attempt.SnapshotSHA256,
		attempt.ContextProjection.ID, attempt.ContextProjection.WorkingSetID,
		int64(attempt.ExpectedRevision.Number), attempt.ExpectedRevision.SHA256, attempt.ObligationID,
		string(budgetJSON), budgetSHA, string(brainJSON), brainSHA, string(attemptJSON), attemptSHA)
	if err != nil {
		return fmt.Errorf("start cognition policy call %q: %w", attempt.ID, err)
	}
	return nil
}
