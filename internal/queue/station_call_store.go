package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) OpenStationCall(
	ctx context.Context,
	record StationCallOpenRecord,
) (StationCallOpening, error) {
	if r == nil || r.pool == nil {
		return StationCallOpening{}, fmt.Errorf("station call opening requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationCallOpening{}, err
	}
	defer tx.Rollback(ctx)
	persistedGap, err := loadStationGapOpeningTx(ctx, tx, record.Gap.ID)
	if err != nil {
		return StationCallOpening{}, fmt.Errorf("load exact station gap opening: %w", err)
	}
	record.Gap = persistedGap
	discovery, err := loadStationDiscoveryReceiptTx(ctx, tx, record.Discovery.ID)
	if err != nil {
		return StationCallOpening{}, fmt.Errorf("load station discovery receipt: %w", err)
	}
	record.Discovery = discovery
	opening, err := validateStationCallOpening(record)
	if err != nil {
		return StationCallOpening{}, err
	}
	if err := requireRunningStationAttemptTx(ctx, tx, record.Authority); err != nil {
		return StationCallOpening{}, err
	}
	if err := insertStationCallOpeningTx(ctx, tx, &opening); err != nil {
		return StationCallOpening{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationCallOpening{}, err
	}
	return opening, nil
}

func insertStationCallOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	opening *StationCallOpening,
) error {
	err := scanStationCallOpening(tx.QueryRow(ctx, `
		INSERT INTO station_call_openings (
			gap_opening_id,discovery_receipt_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			protocol,tokenizer_profile,provider_method,provider_endpoint,
			wire_request,wire_request_sha256,wire_request_bytes,
			expectation,expectation_sha256,observation_challenge,model,
			context_tokens,max_input_tokens,max_output_tokens,output_limit_mode,model_input,
			model_input_sha256,model_input_bytes,model_input_token_upper_bound
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
			$19,$20,$21,$22,$23,$24,$25,$26,$27
		)
		RETURNING id,gap_opening_id,discovery_receipt_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			protocol,tokenizer_profile,provider_method,provider_endpoint,
			wire_request,wire_request_sha256,wire_request_bytes,expectation,
			expectation_sha256,observation_challenge,model,context_tokens,max_input_tokens,
			max_output_tokens,output_limit_mode,model_input,model_input_sha256,model_input_bytes,
			model_input_token_upper_bound,created_at
	`, opening.GapOpeningID, opening.DiscoveryReceiptID, opening.JobID, opening.Generation, opening.StepID,
		opening.StepAttempt, opening.WorkerID, opening.GapID, opening.Protocol,
		opening.TokenizerProfile, opening.ProviderMethod, opening.ProviderEndpoint,
		opening.WireRequest, opening.WireRequestSHA256, opening.WireRequestBytes,
		string(opening.Expectation), opening.ExpectationSHA256, opening.ObservationChallenge,
		opening.Model, opening.ContextTokens, opening.MaxInputTokens, opening.MaxOutputTokens,
		opening.OutputLimitMode, opening.ModelInput, opening.ModelInputSHA256, opening.ModelInputBytes,
		opening.ModelInputTokenCeiling), opening)
	if err != nil {
		return fmt.Errorf("persist exact station call opening: %w", err)
	}
	return nil
}

// RecordStationCallReceipt persists a terminal provider receipt for the exact
// original attempt even if cancellation terminalized that attempt after the
// request crossed the provider boundary.
func (r *Repository) RecordStationCallReceipt(
	ctx context.Context,
	record StationCallReceiptRecord,
) (StationCallReceipt, error) {
	terminal, err := r.recordStationCallReceiptAndEvidence(ctx, StationCallReceiptEvidenceRecord{
		Receipt: record, EvidenceAttempt: 1,
	}, true)
	return terminal.Receipt, err
}

// RecordStationCallReceiptAndEvidence commits the exact provider receipt and
// the evidence row derived from that receipt as one append-only transition.
func (r *Repository) RecordStationCallReceiptAndEvidence(
	ctx context.Context,
	record StationCallReceiptEvidenceRecord,
) (StationCallReceiptEvidence, error) {
	return r.recordStationCallReceiptAndEvidence(ctx, record, false)
}

func (r *Repository) recordStationCallReceiptAndEvidence(
	ctx context.Context,
	record StationCallReceiptEvidenceRecord,
	useOpenedModel bool,
) (StationCallReceiptEvidence, error) {
	if r == nil || r.pool == nil {
		return StationCallReceiptEvidence{}, fmt.Errorf("station call receipt requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	defer tx.Rollback(ctx)
	opening, err := loadStationCallOpeningTx(ctx, tx, record.Receipt.OpeningID)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	gap, err := loadStationGapOpeningTx(ctx, tx, opening.GapOpeningID)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	var attemptStatus model.StepAttemptStatus
	var attemptWorkerID string
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR SHARE
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt).Scan(
		&attemptStatus, &attemptWorkerID,
	); err != nil {
		return StationCallReceiptEvidence{}, err
	}
	receiptRecord, err := bindStationCallReceiptAttempt(
		record.Receipt, opening, attemptStatus, attemptWorkerID,
	)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	receipt, err := insertStationCallReceiptTx(ctx, tx, receiptRecord, opening)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	requestedModel := record.RequestedModel
	if useOpenedModel {
		requestedModel = opening.Model
	}
	evidenceRecord, err := receipt.LLMCallEvidenceRecord(
		receiptRecord.Authority, gap, opening, requestedModel,
		record.EvidenceAttempt, record.LatencyMS,
	)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	evidence, err := insertLLMCallEvidenceTx(ctx, tx, evidenceRecord)
	if err != nil {
		return StationCallReceiptEvidence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationCallReceiptEvidence{}, err
	}
	return StationCallReceiptEvidence{Receipt: receipt, Evidence: evidence}, nil
}

func insertStationCallReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	record StationCallReceiptRecord,
	opening StationCallOpening,
) (StationCallReceipt, error) {
	generationJSON, err := validateStationCallReceipt(record, opening)
	if err != nil {
		return StationCallReceipt{}, err
	}
	if err := insertStationCallCapturesTx(ctx, tx, opening.ID, record.Result); err != nil {
		return StationCallReceipt{}, err
	}
	status := "succeeded"
	if record.Error != "" {
		status = "failed"
	}
	var receipt StationCallReceipt
	err = scanStationCallReceipt(tx.QueryRow(ctx, `
		INSERT INTO station_call_receipts (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,generation_json,generation_sha256,error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''))
		RETURNING id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,status,generation_json,generation_sha256,error,created_at
	`, opening.ID, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, status, string(generationJSON),
		stationGapSHA256(string(generationJSON)), record.Error), &receipt)
	if err != nil {
		return StationCallReceipt{}, fmt.Errorf("persist exact station call receipt: %w", err)
	}
	return receipt, nil
}

func bindStationCallReceiptAttempt(
	record StationCallReceiptRecord,
	opening StationCallOpening,
	status model.StepAttemptStatus,
	workerID string,
) (StationCallReceiptRecord, error) {
	if workerID != opening.WorkerID || workerID != record.Authority.WorkerID {
		return StationCallReceiptRecord{}, fmt.Errorf("station call receipt worker differs from its exact attempt")
	}
	undispatchedIdentity := record.Result.ProviderRequestDisposition == llm.ProviderRequestNotDispatched &&
		record.Result.ProviderResponseDisposition == ""
	switch status {
	case model.StepAttemptActive:
		if record.Result.ProviderRequestFailureReason != "" {
			return StationCallReceiptRecord{}, fmt.Errorf("active station call cannot claim ended authority")
		}
	case model.StepAttemptCanceled, model.StepAttemptSuperseded, model.StepAttemptExpired:
		if !undispatchedIdentity {
			if record.Result.ProviderRequestFailureReason != "" {
				return StationCallReceiptRecord{}, fmt.Errorf(
					"station call crossed a provider boundary and cannot claim a local authority failure reason",
				)
			}
			return record, nil
		}
		want, err := stationAuthorityFailureReason(status)
		if err != nil {
			return StationCallReceiptRecord{}, err
		}
		if record.Result.ProviderRequestFailureReason != "" &&
			record.Result.ProviderRequestFailureReason != want {
			return StationCallReceiptRecord{}, fmt.Errorf(
				"station call authority failure reason %q does not match attempt status %q",
				record.Result.ProviderRequestFailureReason, status,
			)
		}
		record.Result.ProviderRequestFailureReason = want
		if record.Result.ProviderRequestSHA256 == "" {
			record.Result.ProviderRequestSHA256 = opening.WireRequestSHA256
		}
	default:
		return StationCallReceiptRecord{}, fmt.Errorf(
			"station call receipt cannot append for attempt status %q", status,
		)
	}
	return record, nil
}
