package queue

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertObjectivePortableReuseTx(
	ctx context.Context,
	tx pgx.Tx,
	request ObjectivePortableResultReuseRequest,
	targetEnvelope []byte,
	targetObjectiveAuthority []byte,
	source objectivePortableReuseSource,
) (objectivePortableResultReuseRow, error) {
	opening, outcome := source.Opening, source.Outcome
	var row objectivePortableResultReuseRow
	err := scanObjectivePortableResultReuse(tx.QueryRow(ctx, `
		INSERT INTO objective_portable_result_reuses (
			receipt_schema,
			target_job_id,target_generation,target_step_id,target_step_attempt,target_worker_id,
			target_station,target_root_work_id,target_work_kind,target_portable_payload,
			target_portable_payload_sha256,target_portable_envelope,target_portable_envelope_sha256,
			source_job_id,source_generation,source_step_id,source_step_attempt,source_worker_id,
			source_gap_opening_id,source_gap_outcome_id,source_work_id,
			source_portable_envelope_sha256,source_call_receipt_sha256,source_response_sha256,
			objective_authority,objective_authority_sha256
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
			$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26
		)
		RETURNING `+objectivePortableResultReuseColumns+`
	`, ObjectivePortableResultReuseReceiptSchemaV1,
		request.Authority.JobID, request.Authority.Generation, request.Authority.StepID,
		request.Authority.Attempt, request.Authority.WorkerID, request.Station,
		request.Job.ID, request.Job.Kind, string(request.Job.Payload),
		stationGapSHA256(string(request.Job.Payload)), string(targetEnvelope),
		stationGapSHA256(string(targetEnvelope)),
		opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.ID, outcome.ID, opening.WorkID,
		opening.PortableEnvelopeSHA256, outcome.CallReceiptSHA256, outcome.ResponseSHA256,
		string(targetObjectiveAuthority), stationGapSHA256(string(targetObjectiveAuthority)),
	), &row)
	if err != nil {
		return objectivePortableResultReuseRow{}, fmt.Errorf(
			"persist objective portable result reuse receipt: %w", err,
		)
	}
	return row, nil
}

func validatePersistedObjectivePortableReuseTx(
	ctx context.Context,
	tx pgx.Tx,
	request ObjectivePortableResultReuseRequest,
	targetJob model.Job,
	targetEnvelope []byte,
	targetObjectiveAuthority []byte,
	row objectivePortableResultReuseRow,
) (ObjectivePortableResultReuse, error) {
	source, err := loadObjectivePortableReuseSourceTx(
		ctx, tx, row.Receipt.SourceGapOpeningID, row.Receipt.SourceGapOutcomeID,
	)
	if err != nil {
		return ObjectivePortableResultReuse{}, err
	}
	result, matches, err := validateObjectivePortableReuseSource(
		request.Authority, targetJob, request.Job, request.Station,
		targetObjectiveAuthority, source,
	)
	if err != nil {
		return ObjectivePortableResultReuse{}, err
	}
	if !matches {
		return ObjectivePortableResultReuse{}, fmt.Errorf(
			"persisted objective portable reuse receipt has different objective authority",
		)
	}
	return validatePersistedObjectivePortableReuse(
		request, targetEnvelope, targetObjectiveAuthority, row, source, result,
	)
}

func validatePersistedObjectivePortableReuse(
	request ObjectivePortableResultReuseRequest,
	targetEnvelope []byte,
	targetObjectiveAuthority []byte,
	row objectivePortableResultReuseRow,
	source objectivePortableReuseSource,
	result assemblyline.PortableResult,
) (ObjectivePortableResultReuse, error) {
	receipt := row.Receipt
	if receipt.Schema != ObjectivePortableResultReuseReceiptSchemaV1 ||
		receipt.TargetAuthority != request.Authority || receipt.Station != request.Station ||
		receipt.RootWorkID != request.Job.ID || receipt.TargetWorkKind != request.Job.Kind ||
		row.TargetPortablePayload != string(request.Job.Payload) ||
		receipt.TargetPortablePayloadSHA256 != stationGapSHA256(string(request.Job.Payload)) ||
		!bytes.Equal([]byte(row.TargetPortableEnvelope), targetEnvelope) ||
		receipt.TargetPortableEnvelopeSHA256 != stationGapSHA256(string(targetEnvelope)) ||
		!bytes.Equal([]byte(row.ObjectiveAuthority), targetObjectiveAuthority) ||
		receipt.ObjectiveAuthoritySHA256 != stationGapSHA256(string(targetObjectiveAuthority)) {
		return ObjectivePortableResultReuse{}, fmt.Errorf(
			"persisted objective portable reuse receipt differs from its current target authority",
		)
	}
	expectedSourceAuthority := model.StepAttemptAuthority{
		JobID: source.Opening.JobID, Generation: source.Opening.Generation,
		StepID: source.Opening.StepID, Attempt: source.Opening.StepAttempt,
		WorkerID: source.Opening.WorkerID,
	}
	if receipt.SourceAuthority != expectedSourceAuthority ||
		receipt.SourceGapOpeningID != source.Opening.ID ||
		receipt.SourceGapOutcomeID != source.Outcome.ID ||
		receipt.SourceWorkID != source.Opening.WorkID ||
		receipt.SourcePortableEnvelopeSHA256 != source.Opening.PortableEnvelopeSHA256 ||
		receipt.SourceCallReceiptSHA256 != source.Outcome.CallReceiptSHA256 ||
		receipt.SourceResponseSHA256 != source.Outcome.ResponseSHA256 {
		return ObjectivePortableResultReuse{}, fmt.Errorf(
			"persisted objective portable reuse receipt differs from its exact source evidence",
		)
	}
	return ObjectivePortableResultReuse{Result: result, Receipt: receipt}, nil
}
