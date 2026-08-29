package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type stationCallReplayReplacementOrigin struct {
	Gap      StationGapOpening
	Call     StationCallOpening
	Receipt  StationCallReceipt
	Evidence LLMCallEvidence
	Outcome  StationGapOutcome
}

func loadStationCallReplayReplacementOriginTx(
	ctx context.Context,
	tx pgx.Tx,
	originGapOpeningID int64,
	originCallReceiptID int64,
) (stationCallReplayReplacementOrigin, error) {
	var count int
	var callOpeningID, evidenceID, outcomeID int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*),COALESCE(MIN(call.id),0),COALESCE(MIN(evidence.id),0),
		       COALESCE(MIN(outcome.id),0)
		FROM station_call_receipts AS receipt
		JOIN station_call_openings AS call ON call.id=receipt.opening_id
		JOIN llm_call_evidence AS evidence ON evidence.station_call_opening_id=call.id
		JOIN station_gap_outcomes AS outcome ON outcome.opening_id=call.gap_opening_id
		WHERE receipt.id=$1 AND call.gap_opening_id=$2
	`, originCallReceiptID, originGapOpeningID).Scan(
		&count, &callOpeningID, &evidenceID, &outcomeID,
	); err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin relation: %w", err,
		)
	}
	if count != 1 || callOpeningID < 1 || evidenceID < 1 || outcomeID < 1 {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"station replay replacement requires one exact persisted origin relation",
		)
	}

	origin := stationCallReplayReplacementOrigin{}
	var err error
	origin.Gap, err = loadStationCallReplayGapTx(ctx, tx, originGapOpeningID)
	if err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin gap: %w", err,
		)
	}
	origin.Call, err = loadStationCallReplayOpeningTx(ctx, tx, callOpeningID)
	if err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin call: %w", err,
		)
	}
	origin.Receipt, err = loadStationCallReplayReceiptTx(ctx, tx, originCallReceiptID)
	if err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin receipt: %w", err,
		)
	}
	origin.Evidence, err = loadStationCallReplayEvidenceTx(ctx, tx, evidenceID)
	if err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin evidence: %w", err,
		)
	}
	origin.Outcome, err = loadStationCallReplayOutcomeTx(ctx, tx, outcomeID)
	if err != nil {
		return stationCallReplayReplacementOrigin{}, fmt.Errorf(
			"load station replay replacement origin outcome: %w", err,
		)
	}
	return origin, nil
}

func loadStationCallReplayReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	receiptID int64,
) (StationCallReceipt, error) {
	var receipt StationCallReceipt
	err := scanStationCallReceipt(tx.QueryRow(ctx, `
		SELECT id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
		       gap_id,status,generation_json,generation_sha256,error,created_at
		FROM station_call_receipts WHERE id=$1
	`, receiptID), &receipt)
	return receipt, err
}

func loadStationCallReplayEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	evidenceID int64,
) (LLMCallEvidence, error) {
	var evidence LLMCallEvidence
	err := scanLLMCallEvidence(tx.QueryRow(ctx, `
		SELECT `+llmCallEvidenceSelectColumns+`
		FROM llm_call_evidence AS calls
		JOIN station_call_openings AS openings ON openings.id=calls.station_call_opening_id
		WHERE calls.id=$1
	`, evidenceID), &evidence)
	return evidence, err
}

func loadStationCallReplayOutcomeTx(
	ctx context.Context,
	tx pgx.Tx,
	outcomeID int64,
) (StationGapOutcome, error) {
	var outcome StationGapOutcome
	err := scanStationGapOutcome(tx.QueryRow(ctx, `
		SELECT id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
		       gap_id,status,response,response_sha256,projection_kind,
		       call_receipt_sha256,source_response_sha256,source_start_byte,
		       source_end_byte,error,created_at
		FROM station_gap_outcomes WHERE id=$1
	`, outcomeID), &outcome)
	return outcome, err
}
