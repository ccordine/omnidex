package queue

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type stationGapScanner interface {
	Scan(dest ...any) error
}

func scanStationGapOpening(scanner stationGapScanner, opening *StationGapOpening) error {
	var semanticUncertainty []byte
	if err := scanner.Scan(
		&opening.ID, &opening.JobID, &opening.Generation, &opening.StepID,
		&opening.StepAttempt, &opening.WorkerID, &opening.GapID, &opening.Station,
		&opening.Scope, &opening.PortableSchema, &opening.WorkID, &opening.WorkKind,
		&opening.PortablePayload, &opening.PortablePayloadSHA256, &opening.PortableEnvelope,
		&opening.PortableEnvelopeSHA256, &opening.RendererVersion, &opening.Prompt,
		&opening.ProjectionEnvelope, &opening.ProjectionSHA256, &semanticUncertainty,
		&opening.SemanticUncertaintyContractSHA256, &opening.ContextTokens,
		&opening.MaxOutputTokens, &opening.OutputLimitMode,
		&opening.CreatedAt,
	); err != nil {
		return err
	}
	contract, err := decodeStationGapSemanticUncertainty(
		semanticUncertainty, opening.SemanticUncertaintyContractSHA256,
		opening.RendererVersion, opening.WorkKind,
	)
	if err != nil {
		return err
	}
	opening.SemanticUncertaintyContract = contract
	return nil
}

func scanStationGapOutcome(scanner stationGapScanner, outcome *StationGapOutcome) error {
	var response, responseHash, projectionKind, receiptHash, sourceHash, errorText *string
	var sourceStart, sourceEnd *int
	if err := scanner.Scan(
		&outcome.ID, &outcome.OpeningID, &outcome.JobID, &outcome.Generation,
		&outcome.StepID, &outcome.StepAttempt, &outcome.WorkerID, &outcome.GapID,
		&outcome.Status, &response, &responseHash, &projectionKind, &receiptHash,
		&sourceHash, &sourceStart, &sourceEnd, &errorText, &outcome.CreatedAt,
	); err != nil {
		return err
	}
	outcome.Response = stationGapText(response)
	outcome.ResponseSHA256 = stationGapText(responseHash)
	outcome.ProjectionKind = StationGapProjectionKind(stationGapText(projectionKind))
	outcome.CallReceiptSHA256 = stationGapText(receiptHash)
	outcome.SourceResponseSHA256 = stationGapText(sourceHash)
	if sourceStart != nil {
		outcome.SourceStartByte = *sourceStart
	}
	if sourceEnd != nil {
		outcome.SourceEndByte = *sourceEnd
	}
	outcome.Error = stationGapText(errorText)
	return nil
}

func stationGapText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func loadStationGapOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationGapOpening, error) {
	var opening StationGapOpening
	err := scanStationGapOpening(tx.QueryRow(ctx, `
		SELECT id,job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,semantic_uncertainty_contract,
			semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
			output_limit_mode,created_at
		FROM station_gap_openings WHERE id=$1 FOR SHARE
	`, openingID), &opening)
	return opening, err
}
