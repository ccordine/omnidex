package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CognitionEnvironmentState(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
) (cognition.EnvironmentJournalState, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return cognition.EnvironmentJournalState{}, fmt.Errorf("cognition environment journal requires PostgreSQL and context")
	}
	if err := episode.Validate(); err != nil {
		return cognition.EnvironmentJournalState{}, err
	}
	if err := scenario.Validate(); err != nil {
		return cognition.EnvironmentJournalState{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return cognition.EnvironmentJournalState{}, err
	}
	defer tx.Rollback(ctx)
	state, found, err := loadCognitionEnvironmentJournalTx(ctx, tx, episode, false)
	if err != nil {
		return cognition.EnvironmentJournalState{}, err
	}
	if !found || state.Scenario != scenario {
		return cognition.EnvironmentJournalState{}, cognition.ErrEnvironmentJournalNotStarted
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.EnvironmentJournalState{}, err
	}
	return state, nil
}

func loadCognitionEnvironmentJournalTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeRef,
	lock bool,
) (cognition.EnvironmentJournalState, bool, error) {
	query := `SELECT scenario_id,scenario_sha256,start_json,start_sha256,
	                 current_revision,current_revision_sha256,current_receipt_json,current_receipt_sha256,
	                 terminal,terminal_receipt_json,terminal_receipt_sha256
	          FROM cognition_environment_journals WHERE episode_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var scenario cognition.ScenarioRef
	var startRaw, startSHA, currentSHA string
	var currentNumber int64
	var currentRaw, currentReceiptSHA, terminalRaw, terminalSHA *string
	var terminal bool
	err := tx.QueryRow(ctx, query, episode.ID).Scan(
		&scenario.ID, &scenario.SHA256, &startRaw, &startSHA,
		&currentNumber, &currentSHA, &currentRaw, &currentReceiptSHA,
		&terminal, &terminalRaw, &terminalSHA,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognition.EnvironmentJournalState{}, false, nil
	}
	if err != nil {
		return cognition.EnvironmentJournalState{}, false, fmt.Errorf("load cognition environment journal: %w", err)
	}
	start, err := decodeExactEnvironmentTransition(startRaw, startSHA)
	if err != nil {
		return cognition.EnvironmentJournalState{}, false, err
	}
	state := cognition.EnvironmentJournalState{
		Episode: episode, Scenario: scenario, Start: start,
		Current:  cognition.WorldRevision{EpisodeID: episode.ID, Number: uint64(currentNumber), SHA256: currentSHA},
		Terminal: terminal,
	}
	if err := decodeEnvironmentJournalReceipts(episode, &state, currentRaw, currentReceiptSHA, terminalRaw, terminalSHA); err != nil {
		return cognition.EnvironmentJournalState{}, false, err
	}
	if err := state.Validate(); err != nil {
		return cognition.EnvironmentJournalState{}, false, fmt.Errorf("persisted environment journal: %w", err)
	}
	return state, true, nil
}

func decodeEnvironmentJournalReceipts(
	episode cognition.EpisodeRef,
	state *cognition.EnvironmentJournalState,
	currentRaw, currentSHA, terminalRaw, terminalSHA *string,
) error {
	if (currentRaw == nil) != (currentSHA == nil) || (terminalRaw == nil) != (terminalSHA == nil) {
		return fmt.Errorf("%w: persisted environment receipt fields are partial", cognition.ErrEnvironmentJournalConflict)
	}
	if currentRaw != nil {
		receipt, err := decodeExactEnvironmentReceipt(episode, *currentRaw, *currentSHA)
		if err != nil {
			return err
		}
		state.CurrentReceipt = &receipt
	}
	if terminalRaw != nil {
		receipt, err := decodeExactEnvironmentReceipt(episode, *terminalRaw, *terminalSHA)
		if err != nil {
			return err
		}
		state.TerminalReceipt = &receipt
	}
	return nil
}
