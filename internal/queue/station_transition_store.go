package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// OpenStationGapDiscovery persists the typed PortableJob gap and the exact
// provider-discovery opening atomically, before any provider I/O.
func (r *Repository) OpenStationGapDiscovery(
	ctx context.Context,
	record StationGapDiscoveryOpenRecord,
) (StationGapDiscoveryOpening, error) {
	if r == nil || r.pool == nil {
		return StationGapDiscoveryOpening{}, fmt.Errorf("station gap discovery requires PostgreSQL")
	}
	gap, err := validateStationGapOpening(record.Gap)
	if err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireRunningStationAttemptTx(ctx, tx, record.Gap.Authority); err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	if err := insertStationGapOpeningTx(ctx, tx, &gap); err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	discovery, err := validateStationDiscoveryOpening(StationDiscoveryOpenRecord{
		Authority: record.Gap.Authority, Gap: gap, Selection: record.Selection,
	})
	if err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	if err := insertStationDiscoveryOpeningTx(ctx, tx, &discovery); err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationGapDiscoveryOpening{}, err
	}
	return StationGapDiscoveryOpening{Gap: gap, Discovery: discovery}, nil
}

// RecordStationDiscoveryCallOpening atomically persists successful exact
// discovery evidence and the byte-frozen call opening. The caller may contact
// /api/generate only after this method succeeds.
func (r *Repository) RecordStationDiscoveryCallOpening(
	ctx context.Context,
	record StationDiscoveryCallOpenRecord,
) (StationDiscoveryCallOpening, error) {
	if r == nil || r.pool == nil {
		return StationDiscoveryCallOpening{}, fmt.Errorf("station discovery call transition requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	defer tx.Rollback(ctx)
	gap, err := loadStationGapOpeningTx(ctx, tx, record.Gap.ID)
	if err != nil {
		return StationDiscoveryCallOpening{}, fmt.Errorf("load exact station gap opening: %w", err)
	}
	discovery, err := loadStationDiscoveryOpeningTx(ctx, tx, record.Discovery.ID)
	if err != nil {
		return StationDiscoveryCallOpening{}, fmt.Errorf("load exact station discovery opening: %w", err)
	}
	attemptStatus, err := requireStationJournalTransitionAttemptTx(ctx, tx, record.Authority)
	if err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	receiptRecord := StationDiscoveryReceiptRecord{
		Authority: record.Authority, OpeningID: discovery.ID,
		GapID: gap.GapID, Observed: record.Observed,
	}
	receipt, err := insertStationDiscoveryReceiptTx(ctx, tx, receiptRecord, discovery)
	if err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	call, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: record.Authority, Gap: gap, Discovery: receipt, Prepared: record.Prepared,
	})
	if err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	if err := insertStationCallOpeningTx(ctx, tx, &call); err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationDiscoveryCallOpening{}, err
	}
	return StationDiscoveryCallOpening{Discovery: receipt, Call: call, Attempt: attemptStatus}, nil
}

// requireStationJournalTransitionAttemptTx permits only the exact original
// attempt to finish the provider journal transition after cancellation raced
// with successful discovery. It never authorizes a new provider request: the
// caller dispatches with the original canceled context and records an exact
// undispatched receipt.
func requireStationJournalTransitionAttemptTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) (model.StepAttemptStatus, error) {
	var status model.StepAttemptStatus
	var workerID string
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR SHARE
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(
		&status, &workerID,
	); err != nil {
		return "", err
	}
	if workerID != authority.WorkerID {
		return "", fmt.Errorf("station journal worker differs from its exact original attempt")
	}
	switch status {
	case model.StepAttemptActive, model.StepAttemptCanceled,
		model.StepAttemptSuperseded, model.StepAttemptExpired:
		return status, nil
	default:
		return "", fmt.Errorf("station journal transition cannot append for attempt status %q", status)
	}
}

// RecordStationDiscoveryFailure commits the exact raw discovery receipt and
// the failed semantic-gap outcome together. There is no receipt-only state for
// an ordinary provider-discovery failure.
func (r *Repository) RecordStationDiscoveryFailure(
	ctx context.Context,
	record StationDiscoveryFailureRecord,
) (StationDiscoveryFailure, error) {
	if r == nil || r.pool == nil {
		return StationDiscoveryFailure{}, fmt.Errorf("station discovery failure requires PostgreSQL")
	}
	if record.Error == "" {
		return StationDiscoveryFailure{}, fmt.Errorf("station discovery failure requires an exact error")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationDiscoveryFailure{}, err
	}
	defer tx.Rollback(ctx)
	gap, err := loadStationGapOpeningTx(ctx, tx, record.Gap.ID)
	if err != nil {
		return StationDiscoveryFailure{}, err
	}
	discovery, err := loadStationDiscoveryOpeningTx(ctx, tx, record.Discovery.ID)
	if err != nil {
		return StationDiscoveryFailure{}, err
	}
	terminal := StationGapTerminalRecord{
		Authority: record.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: record.Error,
	}
	if err := validateStationGapTerminal(terminal); err != nil {
		return StationDiscoveryFailure{}, err
	}
	if err := requireStationGapClosingAuthorityTx(ctx, tx, terminal); err != nil {
		return StationDiscoveryFailure{}, err
	}
	receipt, err := insertStationDiscoveryReceiptTx(ctx, tx, StationDiscoveryReceiptRecord{
		Authority: record.Authority, OpeningID: discovery.ID, GapID: gap.GapID,
		Observed: record.Observed, FailureReason: record.FailureReason, Error: record.Error,
	}, discovery)
	if err != nil {
		return StationDiscoveryFailure{}, err
	}
	var outcome StationGapOutcome
	if err := insertStationGapOutcomeTx(ctx, tx, terminal, &outcome); err != nil {
		return StationDiscoveryFailure{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationDiscoveryFailure{}, err
	}
	return StationDiscoveryFailure{Discovery: receipt, Outcome: outcome}, nil
}

func requireRunningStationAttemptTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) error {
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return fmt.Errorf("%w: station boundary attempt is not running", ErrStaleStepAttempt)
	}
	return nil
}
