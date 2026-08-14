package queue

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func scanStationCallOpening(scanner stationGapScanner, opening *StationCallOpening) error {
	var expectation string
	if err := scanner.Scan(
		&opening.ID, &opening.GapOpeningID, &opening.DiscoveryReceiptID,
		&opening.JobID, &opening.Generation,
		&opening.StepID, &opening.StepAttempt, &opening.WorkerID, &opening.GapID,
		&opening.Protocol, &opening.TokenizerProfile, &opening.ProviderMethod,
		&opening.ProviderEndpoint, &opening.WireRequest, &opening.WireRequestSHA256,
		&opening.WireRequestBytes, &expectation, &opening.ExpectationSHA256,
		&opening.ObservationChallenge, &opening.Model, &opening.ContextTokens,
		&opening.MaxInputTokens, &opening.MaxOutputTokens, &opening.ModelInput,
		&opening.ModelInputSHA256, &opening.ModelInputBytes,
		&opening.ModelInputTokenCeiling, &opening.CreatedAt,
	); err != nil {
		return err
	}
	opening.Expectation = append(json.RawMessage(nil), expectation...)
	return nil
}

func loadStationCallOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationCallOpening, error) {
	var opening StationCallOpening
	err := scanStationCallOpening(tx.QueryRow(ctx, `
		SELECT id,gap_opening_id,discovery_receipt_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			protocol,tokenizer_profile,provider_method,provider_endpoint,
			wire_request,wire_request_sha256,wire_request_bytes,expectation,
			expectation_sha256,observation_challenge,model,context_tokens,max_input_tokens,
			max_output_tokens,model_input,model_input_sha256,model_input_bytes,
			model_input_token_upper_bound,created_at
		FROM station_call_openings WHERE id=$1 FOR SHARE
	`, openingID), &opening)
	return opening, err
}

func scanStationCallReceipt(scanner stationGapScanner, receipt *StationCallReceipt) error {
	var generationJSON string
	var errorText *string
	if err := scanner.Scan(
		&receipt.ID, &receipt.OpeningID, &receipt.JobID, &receipt.Generation,
		&receipt.StepID, &receipt.StepAttempt, &receipt.WorkerID, &receipt.GapID,
		&receipt.Status, &generationJSON, &receipt.GenerationSHA256, &errorText,
		&receipt.CreatedAt,
	); err != nil {
		return err
	}
	receipt.GenerationJSON = append(json.RawMessage(nil), generationJSON...)
	receipt.Error = llmEvidenceString(errorText)
	return nil
}
