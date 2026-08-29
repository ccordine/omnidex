package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) OpenStationDiscovery(
	ctx context.Context,
	record StationDiscoveryOpenRecord,
) (StationDiscoveryOpening, error) {
	if r == nil || r.pool == nil {
		return StationDiscoveryOpening{}, fmt.Errorf("station discovery opening requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	defer tx.Rollback(ctx)
	gap, err := loadStationGapOpeningTx(ctx, tx, record.Gap.ID)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	record.Gap = gap
	opening, err := validateStationDiscoveryOpening(record)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	if err := requireRunningStationAttemptTx(ctx, tx, record.Authority); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if err := insertStationDiscoveryOpeningTx(ctx, tx, &opening); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationDiscoveryOpening{}, err
	}
	return opening, nil
}

func insertStationDiscoveryOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	opening *StationDiscoveryOpening,
) error {
	err := scanStationDiscoveryOpening(tx.QueryRow(ctx, `
		INSERT INTO station_provider_discoveries (
			gap_opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			selection,selection_sha256,challenge
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id,gap_opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,selection,selection_sha256,challenge,created_at
	`, opening.GapOpeningID, opening.JobID, opening.Generation, opening.StepID,
		opening.StepAttempt, opening.WorkerID, opening.GapID, string(opening.Selection),
		opening.SelectionSHA256, opening.Challenge), opening)
	if err != nil {
		return fmt.Errorf("persist station discovery opening: %w", err)
	}
	return nil
}

func (r *Repository) RecordStationDiscoveryReceipt(
	ctx context.Context,
	record StationDiscoveryReceiptRecord,
) (StationDiscoveryReceipt, error) {
	if r == nil || r.pool == nil {
		return StationDiscoveryReceipt{}, fmt.Errorf("station discovery receipt requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationDiscoveryReceipt{}, err
	}
	defer tx.Rollback(ctx)
	opening, err := loadStationDiscoveryOpeningTx(ctx, tx, record.OpeningID)
	if err != nil {
		return StationDiscoveryReceipt{}, err
	}
	receipt, err := insertStationDiscoveryReceiptTx(ctx, tx, record, opening)
	if err != nil {
		return StationDiscoveryReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationDiscoveryReceipt{}, err
	}
	return receipt, nil
}

func insertStationDiscoveryReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	record StationDiscoveryReceiptRecord,
	opening StationDiscoveryOpening,
) (StationDiscoveryReceipt, error) {
	observation, expectation, err := validateStationDiscoveryReceipt(record, opening)
	if err != nil {
		return StationDiscoveryReceipt{}, err
	}
	if err := insertStationDiscoveryCapturesTx(ctx, tx, opening.ID, record.Observed.Evidence); err != nil {
		return StationDiscoveryReceipt{}, err
	}
	status := "succeeded"
	if record.Error != "" {
		status = "failed"
	}
	expectationSHA256 := ""
	if len(expectation) > 0 {
		expectationSHA256 = stationGapSHA256(string(expectation))
	}
	var receipt StationDiscoveryReceipt
	err = scanStationDiscoveryReceipt(tx.QueryRow(ctx, `
		INSERT INTO station_provider_discovery_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,observation,observation_sha256,expectation,expectation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''))
		RETURNING id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,status,observation,observation_sha256,expectation,expectation_sha256,error,created_at
	`, opening.ID, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, status, string(observation), stationGapSHA256(string(observation)),
		string(expectation), expectationSHA256, record.Error), &receipt)
	if err != nil {
		return StationDiscoveryReceipt{}, fmt.Errorf("persist station discovery receipt: %w", err)
	}
	return receipt, nil
}
