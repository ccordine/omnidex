package queue

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

type stationGapScanner interface {
	Scan(dest ...any) error
}

func scanStationGapOpening(scanner stationGapScanner, opening *StationGapOpening) error {
	var schema string
	if err := scanner.Scan(
		&opening.ID, &opening.JobID, &opening.Generation, &opening.StepID,
		&opening.StepAttempt, &opening.WorkerID, &opening.GapID, &opening.Station,
		&opening.Scope, &opening.PortableSchema, &opening.WorkID, &opening.WorkKind,
		&opening.PortablePayload, &opening.PortablePayloadSHA256, &opening.PortableEnvelope,
		&opening.PortableEnvelopeSHA256, &opening.RendererVersion, &opening.Prompt, &schema,
		&opening.ProjectionEnvelope, &opening.ProjectionSHA256, &opening.ContextTokens,
		&opening.MaxOutputTokens, &opening.CreatedAt,
	); err != nil {
		return err
	}
	opening.ResponseSchema = append(json.RawMessage(nil), schema...)
	return nil
}

func scanStationGapOutcome(scanner stationGapScanner, outcome *StationGapOutcome) error {
	var response, responseHash, errorText *string
	if err := scanner.Scan(
		&outcome.ID, &outcome.OpeningID, &outcome.JobID, &outcome.Generation,
		&outcome.StepID, &outcome.StepAttempt, &outcome.WorkerID, &outcome.GapID,
		&outcome.Status, &response, &responseHash, &errorText, &outcome.CreatedAt,
	); err != nil {
		return err
	}
	outcome.Response = llmEvidenceString(response)
	outcome.ResponseSHA256 = llmEvidenceString(responseHash)
	outcome.Error = llmEvidenceString(errorText)
	return nil
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
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,response_schema,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens,created_at
		FROM station_gap_openings WHERE id=$1 FOR SHARE
	`, openingID), &opening)
	return opening, err
}
