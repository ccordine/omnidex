package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReviewCognitionEnvironmentAction(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.EnvironmentReceipt, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return cognition.EnvironmentReceipt{}, false, fmt.Errorf("cognition environment journal requires PostgreSQL and context")
	}
	if err := scenario.Validate(); err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	if err := validateEnvironmentActionIdentity(episode, expected, action); err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	defer tx.Rollback(ctx)
	if receipt, found, err := loadCognitionEnvironmentReceiptTx(ctx, tx, episode, expected, action); err != nil || found {
		return receipt, found, err
	}
	state, found, err := loadCognitionEnvironmentJournalTx(ctx, tx, episode, false)
	if err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	if !found || state.Scenario != scenario {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalNotStarted
	}
	if state.Terminal {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalTerminal
	}
	if state.Current != expected {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalStaleRevision
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	return cognition.EnvironmentReceipt{}, false, nil
}

func (r *Repository) CommitCognitionEnvironmentAction(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	candidate cognition.EnvironmentReceipt,
) (cognition.EnvironmentReceipt, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return cognition.EnvironmentReceipt{}, fmt.Errorf("cognition environment journal requires PostgreSQL and context")
	}
	if err := scenario.Validate(); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	if err := validateEnvironmentActionIdentity(episode, candidate.Expected, candidate.Action); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	if err := candidate.Validate(episode); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	defer tx.Rollback(ctx)
	state, found, err := loadCognitionEnvironmentJournalTx(ctx, tx, episode, true)
	if err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	if !found || state.Scenario != scenario {
		return cognition.EnvironmentReceipt{}, cognition.ErrEnvironmentJournalNotStarted
	}
	if receipt, replay, err := loadCognitionEnvironmentReceiptTx(
		ctx, tx, episode, candidate.Expected, candidate.Action,
	); err != nil || replay {
		if err == nil {
			err = tx.Commit(ctx)
		}
		return receipt, err
	}
	if state.Terminal {
		return cognition.EnvironmentReceipt{}, cognition.ErrEnvironmentJournalTerminal
	}
	if state.Current != candidate.Expected {
		return cognition.EnvironmentReceipt{}, cognition.ErrEnvironmentJournalStaleRevision
	}
	authority := environmentActionAuthority(candidate.Action)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	sequence, err := insertCognitionEnvironmentReceiptTx(ctx, tx, episode, candidate)
	if err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	if err := commitCognitionEnvironmentReceiptTx(ctx, tx, episode, candidate, sequence); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	return candidate.Clone(), nil
}

func loadCognitionEnvironmentReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.EnvironmentReceipt, bool, error) {
	var expectedNumber int64
	var expectedSHA, actionRaw, actionSHA, receiptRaw, receiptSHA string
	err := tx.QueryRow(ctx, `
		SELECT expected_revision,expected_revision_sha256,action_json,action_sha256,
		       receipt_json,receipt_sha256
		FROM cognition_environment_receipts WHERE episode_id=$1 AND action_id=$2
	`, episode.ID, action.ID).Scan(
		&expectedNumber, &expectedSHA, &actionRaw, &actionSHA, &receiptRaw, &receiptSHA,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognition.EnvironmentReceipt{}, false, nil
	}
	if err != nil {
		return cognition.EnvironmentReceipt{}, false, fmt.Errorf("load cognition environment receipt: %w", err)
	}
	wantActionRaw, wantActionSHA, err := cognitionJSON(action)
	if err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	if uint64(expectedNumber) != expected.Number || expectedSHA != expected.SHA256 ||
		actionRaw != string(wantActionRaw) || actionSHA != wantActionSHA {
		return cognition.EnvironmentReceipt{}, false, cognition.ErrEnvironmentJournalConflict
	}
	receipt, err := decodeExactEnvironmentReceipt(episode, receiptRaw, receiptSHA)
	if err != nil {
		return cognition.EnvironmentReceipt{}, false, err
	}
	return receipt, true, nil
}

func insertCognitionEnvironmentReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeRef,
	receipt cognition.EnvironmentReceipt,
) (int64, error) {
	actionRaw, actionSHA, err := cognitionJSON(receipt.Action)
	if err != nil {
		return 0, err
	}
	receiptRaw, receiptSHA, err := cognitionJSON(receipt)
	if err != nil {
		return 0, err
	}
	status := "failure"
	if receipt.Transition != nil {
		status = "transition"
	}
	var sequence int64
	err = tx.QueryRow(ctx, `
		INSERT INTO cognition_environment_receipts (
			episode_id,action_id,commit_sequence,expected_revision,expected_revision_sha256,
			action_json,action_sha256,status,receipt_json,receipt_sha256,
			actor_job_id,actor_generation,actor_step_id,actor_attempt,actor_worker_id
		) SELECT $1,$2,commit_sequence+1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
		  FROM cognition_environment_journals WHERE episode_id=$1
		RETURNING commit_sequence
	`, episode.ID, receipt.Action.ID, int64(receipt.Expected.Number), receipt.Expected.SHA256,
		string(actionRaw), actionSHA, status, string(receiptRaw), receiptSHA,
		receipt.Action.Actor.JobID, receipt.Action.Actor.Generation, receipt.Action.Actor.StepID,
		int64(receipt.Action.Actor.Attempt), receipt.Action.Actor.WorkerID).Scan(&sequence)
	if err != nil {
		return 0, fmt.Errorf("persist cognition environment receipt: %w", err)
	}
	return sequence, nil
}

func commitCognitionEnvironmentReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeRef,
	receipt cognition.EnvironmentReceipt,
	sequence int64,
) error {
	raw, sha, err := cognitionJSON(receipt)
	if err != nil {
		return err
	}
	if receipt.Failure != nil {
		result, err := tx.Exec(ctx, `
			UPDATE cognition_environment_journals
			SET commit_sequence=$2,last_receipt_json=$3,last_receipt_sha256=$4,
			    updated_at=clock_timestamp() WHERE episode_id=$1
		`, episode.ID, sequence, string(raw), sha)
		if err != nil {
			return fmt.Errorf("commit cognition environment failure receipt: %w", err)
		}
		if result.RowsAffected() != 1 {
			return cognition.ErrEnvironmentJournalNotStarted
		}
		return nil
	}
	transition := receipt.Transition
	result, err := tx.Exec(ctx, `
		UPDATE cognition_environment_journals
		SET current_revision=$2,current_revision_sha256=$3,
		    current_receipt_json=$4,current_receipt_sha256=$5,terminal=$6,
		    terminal_receipt_json=CASE WHEN $6 THEN $4 ELSE NULL END,
		    terminal_receipt_sha256=CASE WHEN $6 THEN $5 ELSE NULL END,
		    commit_sequence=$7,last_receipt_json=$4,last_receipt_sha256=$5,
		    updated_at=clock_timestamp()
		WHERE episode_id=$1
	`, episode.ID, int64(transition.Current.Number), transition.Current.SHA256,
		string(raw), sha, transition.Terminal, sequence)
	if err != nil {
		return fmt.Errorf("advance cognition environment journal: %w", err)
	}
	if result.RowsAffected() != 1 {
		return cognition.ErrEnvironmentJournalNotStarted
	}
	return nil
}
