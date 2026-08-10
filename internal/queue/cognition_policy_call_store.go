package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type cognitionPolicyCallRecord struct {
	Attempt          cognitionpolicy.CallAttempt
	AttemptSHA256    string
	Status           string
	Result           *cognitionpolicy.CallResult
	ResultSHA256     string
	ResponseEvidence *cognitionpolicy.ModelResponseEvidence
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
	query := `SELECT attempt_json,attempt_sha256,status,result_json,COALESCE(result_sha256,'')
		FROM cognition_policy_calls WHERE call_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var attemptJSON []byte
	var resultJSON []byte
	var record cognitionPolicyCallRecord
	err := tx.QueryRow(ctx, query, callID).Scan(
		&attemptJSON, &record.AttemptSHA256, &record.Status, &resultJSON, &record.ResultSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionPolicyCallRecord{}, false, nil
	}
	if err != nil {
		return cognitionPolicyCallRecord{}, false, err
	}
	if err := exactjson.ValidateObject(
		attemptJSON, cognitionpolicy.CallAttempt{}, "cognition policy attempt",
	); err != nil {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode exact cognition policy attempt: %w", err)
	}
	if err := json.Unmarshal(attemptJSON, &record.Attempt); err != nil {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode cognition policy attempt: %w", err)
	}
	canonicalAttempt, err := exactjson.Canonical(record.Attempt)
	if err != nil || !bytes.Equal(canonicalAttempt, attemptJSON) ||
		cognitionPayloadSHA(canonicalAttempt) != record.AttemptSHA256 ||
		record.Attempt.Validate() != nil {
		return cognitionPolicyCallRecord{}, false, fmt.Errorf("%w: persisted policy attempt is invalid", ErrCognitionConflict)
	}
	if len(resultJSON) > 0 {
		var result cognitionpolicy.CallResult
		if err := exactjson.ValidateObject(
			resultJSON, cognitionpolicy.CallResult{}, "cognition policy result",
		); err != nil {
			return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode exact cognition policy result: %w", err)
		}
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return cognitionPolicyCallRecord{}, false, fmt.Errorf("decode cognition policy result: %w", err)
		}
		canonicalResult, err := exactjson.Canonical(result)
		if err != nil || !bytes.Equal(canonicalResult, resultJSON) ||
			cognitionPayloadSHA(canonicalResult) != record.ResultSHA256 ||
			result.Validate(record.Attempt) != nil {
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
	if record.Result != nil {
		if err := validateLoadedCognitionProviderIdentityRefTx(
			ctx, tx, callID, record.Result.ProviderIdentityEvidence,
		); err != nil {
			return cognitionPolicyCallRecord{}, false, err
		}
	}
	if record.Result != nil && record.Result.Status == cognitionpolicy.CallResultAccepted {
		evidence, err := loadCognitionResponseEvidenceTx(ctx, tx, callID, lock)
		if err != nil {
			return cognitionPolicyCallRecord{}, false, err
		}
		record.ResponseEvidence = evidence
		if err := validateLoadedCognitionResponseEvidence(record.Attempt, *record.Result, evidence); err != nil {
			return cognitionPolicyCallRecord{}, false, err
		}
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
		if persisted.ResponseEvidence != nil {
			evidence := persisted.ResponseEvidence.Clone()
			reservation.ExistingResponseEvidence = &evidence
		}
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
	episode CognitionEpisode,
	attempt cognitionpolicy.CallAttempt,
) error {
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: policy call episode has no obligation graph", ErrCognitionConflict)
	}
	prepared, err := loadCognitionPreparedSnapshotBySHATx(
		ctx, tx, authority, episode, graph, attempt.SnapshotSHA256,
	)
	if err != nil {
		return fmt.Errorf("%w: restore exact prepared policy snapshot: %v", ErrCognitionConflict, err)
	}
	projection, err := loadContextProjectionTx(ctx, tx, string(attempt.ContextProjection.ID))
	if err != nil {
		return err
	}
	if err := cognitionpolicy.VerifyCallAttempt(
		prepared.Prepared.Snapshot, projection.Projection, attempt,
	); err != nil {
		return fmt.Errorf("%w: exact policy input verification: %v", ErrCognitionConflict, err)
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
	attemptJSON, err := exactjson.Canonical(attempt)
	if err != nil {
		return err
	}
	attemptSHA := cognitionPayloadSHA(attemptJSON)
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
