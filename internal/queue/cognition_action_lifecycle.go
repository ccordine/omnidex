package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) DispatchCognitionAction(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	actionID cognition.ActionID,
) (CognitionActionRecord, error) {
	return r.advanceCognitionAction(ctx, authority, actionID, CognitionActionDispatched, nil)
}

func (r *Repository) IngestCognitionFailure(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	actionID cognition.ActionID,
	failure cognition.ActionFailure,
) (CognitionActionRecord, error) {
	return r.advanceCognitionAction(ctx, authority, actionID, CognitionActionFailed, failure)
}

func (r *Repository) advanceCognitionAction(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	actionID cognition.ActionID,
	target CognitionActionStatus,
	detail any,
) (CognitionActionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || actionID == "" {
		return CognitionActionRecord{}, fmt.Errorf("cognition action transition requires PostgreSQL, context, and action ID")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return CognitionActionRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionActionRecord{}, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return CognitionActionRecord{}, err
	} else if status != model.StepStatusRunning {
		return CognitionActionRecord{}, staleStepAttemptError(authority, "cognition action driver is not running", nil)
	}
	var episodeID cognition.EpisodeID
	if err := tx.QueryRow(ctx, `SELECT episode_id FROM cognition_actions WHERE action_id=$1`, actionID).Scan(&episodeID); err != nil {
		if err == pgx.ErrNoRows {
			return CognitionActionRecord{}, fmt.Errorf("%w: %s", ErrCognitionActionNotFound, actionID)
		}
		return CognitionActionRecord{}, err
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, true)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, true)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("load cognition action episode: %w", err)
	}
	record, found, err := loadCognitionActionTx(ctx, tx, actionID, true)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if !found {
		return CognitionActionRecord{}, fmt.Errorf("%w: %s", ErrCognitionActionNotFound, actionID)
	}
	if authority.JobID != record.Origin.JobID || authority.Generation != record.Origin.Generation ||
		authority.StepID != record.Origin.StepID {
		return CognitionActionRecord{}, staleStepAttemptError(authority, "cognition action belongs to another step", nil)
	}
	if target == record.Status {
		if target == CognitionActionFailed {
			failure, ok := detail.(cognition.ActionFailure)
			if !ok || record.Failure == nil {
				return CognitionActionRecord{}, fmt.Errorf("%w: failed cognition replay has no exact failure", ErrCognitionConflict)
			}
			left, leftSHA, leftErr := cognitionJSON(failure)
			right, rightSHA, rightErr := cognitionJSON(*record.Failure)
			if leftErr != nil || rightErr != nil || leftSHA != rightSHA || string(left) != string(right) {
				return CognitionActionRecord{}, fmt.Errorf("%w: failed cognition replay changed content", ErrCognitionConflict)
			}
			proof, err := cognitionActionFailureProof(failure)
			if err != nil {
				return CognitionActionRecord{}, err
			}
			if err := requireCognitionProposalDispositionReplayTx(
				ctx, tx, record, cognitionProposalRejectedFailure, proof,
			); err != nil {
				return CognitionActionRecord{}, err
			}
		} else if detail != nil {
			return CognitionActionRecord{}, fmt.Errorf("%w: cognition action replay added content", ErrCognitionConflict)
		}
		return record, nil
	}
	if target == CognitionActionDispatched {
		if record.Status != CognitionActionPrepared || detail != nil {
			return CognitionActionRecord{}, fmt.Errorf("%w: action %q cannot dispatch from %q", ErrCognitionConflict, actionID, record.Status)
		}
		result, err := tx.Exec(ctx, `
			UPDATE cognition_actions SET status='dispatched',dispatched_at=clock_timestamp()
			WHERE action_id=$1 AND status='prepared'
		`, actionID)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		if result.RowsAffected() != 1 {
			return CognitionActionRecord{}, fmt.Errorf("%w: cognition action changed before dispatch", ErrCognitionConflict)
		}
	} else if target == CognitionActionFailed {
		failure, ok := detail.(cognition.ActionFailure)
		if !ok || record.Status != CognitionActionDispatched {
			return CognitionActionRecord{}, fmt.Errorf("%w: action %q cannot fail from %q", ErrCognitionConflict, actionID, record.Status)
		}
		if err := failure.Validate(record.Action, record.ExpectedRevision); err != nil {
			return CognitionActionRecord{}, err
		}
		if err := persistCognitionActionFailureTx(ctx, tx, header, episode, record, failure); err != nil {
			return CognitionActionRecord{}, err
		}
		header, err = loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, true)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		link, found, err := loadCognitionActionGraphMaterializationTx(ctx, tx, record)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		if found {
			proof, err := cognitionActionFailureProof(failure)
			if err != nil {
				return CognitionActionRecord{}, err
			}
			header, err = persistCognitionProposalDispositionTx(
				ctx, tx, header, episode, record, link,
				cognitionProposalRejectedFailure, proof, authority,
			)
			if err != nil {
				return CognitionActionRecord{}, err
			}
		}
		failureJSON, failureSHA, err := cognitionJSON(failure)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		result, err := tx.Exec(ctx, `
			UPDATE cognition_actions
			SET status='failed',failure_json=$2,failure_sha256=$3,resolved_at=clock_timestamp()
			WHERE action_id=$1 AND status='dispatched'
		`, actionID, string(failureJSON), failureSHA)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		if result.RowsAffected() != 1 {
			return CognitionActionRecord{}, fmt.Errorf("%w: cognition action changed before failure", ErrCognitionConflict)
		}
	} else {
		return CognitionActionRecord{}, fmt.Errorf("unregistered cognition action target %q", target)
	}
	if err := insertCognitionActionEventTx(ctx, tx, actionID, authority, target, detail); err != nil {
		return CognitionActionRecord{}, err
	}
	record, found, err = loadCognitionActionTx(ctx, tx, actionID, false)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("reload cognition action %q: %w", actionID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionActionRecord{}, err
	}
	return record, nil
}
