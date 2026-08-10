package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// CognitionPolicyCallJournal is the PostgreSQL authority for policy-call
// allowance and immutable before/after inference evidence.
type CognitionPolicyCallJournal struct {
	Repository *Repository
}

var _ cognitionpolicy.CallJournal = CognitionPolicyCallJournal{}

func (journal CognitionPolicyCallJournal) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	if journal.Repository == nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("cognition policy call journal requires a repository")
	}
	return journal.Repository.StartCognitionPolicyCall(ctx, attempt)
}

func (journal CognitionPolicyCallJournal) Finish(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	if journal.Repository == nil {
		return fmt.Errorf("cognition policy call journal requires a repository")
	}
	return journal.Repository.FinishCognitionPolicyCall(ctx, attempt, result, evidence)
}

func (r *Repository) StartCognitionPolicyCall(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("cognition policy call start requires PostgreSQL and context")
	}
	if err := attempt.Validate(); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	defer tx.Rollback(ctx)
	episode, err := lockCognitionPolicyCallAuthority(ctx, tx, authority, attempt)
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	persisted, found, err := loadCognitionPolicyCallTx(ctx, tx, attempt.ID, true)
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	if found {
		reservation, err := exactCognitionCallReservation(attempt, persisted)
		if err != nil {
			return cognitionpolicy.CallReservation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return cognitionpolicy.CallReservation{}, err
		}
		return reservation, nil
	}
	if err := requireExactCognitionPolicySnapshotTx(ctx, tx, authority, episode, attempt); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	var priorCalls int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1`,
		attempt.ExpectedRevision.EpisodeID).Scan(&priorCalls); err != nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("count cognition policy attempts: %w", err)
	}
	if priorCalls < 0 || uint64(priorCalls) >= uint64(episode.Budget.RemainingPolicyCalls) {
		return cognitionpolicy.CallReservation{}, ErrCognitionBudgetExhausted
	}
	expected := episode.Budget
	expected.RemainingPolicyCalls -= uint32(priorCalls)
	if attempt.RuntimeBudget != expected {
		return cognitionpolicy.CallReservation{}, fmt.Errorf(
			"%w: policy call budget differs from exact durable attempt count", ErrCognitionConflict,
		)
	}
	if err := insertCognitionPolicyCallTx(ctx, tx, authority, attempt); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	return cognitionpolicy.CallReservation{Attempt: attempt.Clone(), Created: true}, nil
}

func (r *Repository) FinishCognitionPolicyCall(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.CallEvidence,
) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("cognition policy call finish requires PostgreSQL and context")
	}
	if err := result.Validate(attempt); err != nil {
		return err
	}
	if err := evidence.ValidateFor(attempt, result); err != nil {
		return err
	}
	authority, err := cognitionPolicyCallAuthority(attempt)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := lockCognitionPolicyCallAuthority(ctx, tx, authority, attempt); err != nil {
		return err
	}
	persisted, found, err := loadCognitionPolicyCallTx(ctx, tx, attempt.ID, true)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: policy call result has no started attempt", ErrCognitionConflict)
	}
	if _, err := exactCognitionCallReservation(attempt, persisted); err != nil {
		return err
	}
	if persisted.Result != nil {
		if *persisted.Result != result {
			return fmt.Errorf("%w: policy call result replay changed content", ErrCognitionConflict)
		}
		return tx.Commit(ctx)
	}
	if err := insertCognitionProviderIdentityEvidenceTx(
		ctx, tx, authority, attempt, result, evidence.ProviderIdentity,
	); err != nil {
		return err
	}
	if err := insertCognitionResponseEvidenceTx(
		ctx, tx, authority, attempt, result, evidence.Response,
	); err != nil {
		return err
	}
	if err := insertCognitionProviderResponseCaptureTx(
		ctx, tx, authority, attempt, result, evidence.ProviderResponseCapture,
	); err != nil {
		return err
	}
	if err := insertCognitionProviderGenerationEvidenceTx(
		ctx, tx, authority, attempt, result, evidence.ProviderGeneration,
	); err != nil {
		return err
	}
	resultJSON, err := exactjson.Canonical(result)
	if err != nil {
		return err
	}
	resultSHA := cognitionPayloadSHA(resultJSON)
	tag, err := tx.Exec(ctx, `
		UPDATE cognition_policy_calls SET status=$2,result_json=$3,result_sha256=$4,finished_at=NOW()
		WHERE call_id=$1 AND status='started'
	`, attempt.ID, result.Status, string(resultJSON), resultSHA)
	if err != nil {
		return fmt.Errorf("finish cognition policy call %q: %w", attempt.ID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: policy call was not in started state", ErrCognitionConflict)
	}
	return tx.Commit(ctx)
}

func lockCognitionPolicyCallAuthority(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
) (CognitionEpisode, error) {
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return CognitionEpisode{}, err
	} else if status != model.StepStatusRunning {
		return CognitionEpisode{}, staleStepAttemptError(authority, "cognition policy actor is not running", nil)
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, attempt.ExpectedRevision.EpisodeID, true)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if !found {
		return CognitionEpisode{}, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, attempt.ExpectedRevision.EpisodeID)
	}
	if err := cognitionAuthorityMatches(authority, episode); err != nil {
		return CognitionEpisode{}, err
	}
	if episode.Status != CognitionEpisodeActive {
		return CognitionEpisode{}, ErrCognitionTerminal
	}
	if episode.CurrentRevision != attempt.ExpectedRevision {
		return CognitionEpisode{}, fmt.Errorf("%w: policy call targets another world revision", ErrCognitionConflict)
	}
	return episode, nil
}
