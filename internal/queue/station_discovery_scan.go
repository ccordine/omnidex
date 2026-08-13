package queue

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func scanStationDiscoveryOpening(scanner stationGapScanner, opening *StationDiscoveryOpening) error {
	var selection string
	if err := scanner.Scan(
		&opening.ID, &opening.GapOpeningID, &opening.JobID, &opening.Generation,
		&opening.StepID, &opening.StepAttempt, &opening.WorkerID, &opening.GapID,
		&selection, &opening.SelectionSHA256, &opening.Challenge, &opening.CreatedAt,
	); err != nil {
		return err
	}
	opening.Selection = append(json.RawMessage(nil), selection...)
	return nil
}

func loadStationDiscoveryOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationDiscoveryOpening, error) {
	var opening StationDiscoveryOpening
	err := scanStationDiscoveryOpening(tx.QueryRow(ctx, `
		SELECT id,gap_opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,selection,selection_sha256,challenge,created_at
		FROM station_provider_discoveries WHERE id=$1 FOR SHARE
	`, openingID), &opening)
	return opening, err
}

func scanStationDiscoveryReceipt(scanner stationGapScanner, receipt *StationDiscoveryReceipt) error {
	var observation string
	var expectation, expectationSHA256, errorText *string
	if err := scanner.Scan(
		&receipt.ID, &receipt.OpeningID, &receipt.JobID, &receipt.Generation,
		&receipt.StepID, &receipt.StepAttempt, &receipt.WorkerID, &receipt.GapID,
		&receipt.Status, &observation, &receipt.ObservationSHA256,
		&expectation, &expectationSHA256, &errorText, &receipt.CreatedAt,
	); err != nil {
		return err
	}
	receipt.Observation = append(json.RawMessage(nil), observation...)
	if expectation != nil {
		receipt.Expectation = append(json.RawMessage(nil), (*expectation)...)
	}
	receipt.ExpectationSHA256 = llmEvidenceString(expectationSHA256)
	receipt.Error = llmEvidenceString(errorText)
	return nil
}

func loadStationDiscoveryReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	receiptID int64,
) (StationDiscoveryReceipt, error) {
	var receipt StationDiscoveryReceipt
	err := scanStationDiscoveryReceipt(tx.QueryRow(ctx, `
		SELECT id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,status,observation,observation_sha256,expectation,expectation_sha256,error,created_at
		FROM station_provider_discovery_receipts WHERE id=$1 FOR SHARE
	`, receiptID), &receipt)
	return receipt, err
}
