package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) OpenStationGap(
	ctx context.Context,
	record StationGapOpenRecord,
) (StationGapOpening, error) {
	opening, err := validateStationGapOpening(record)
	if err != nil {
		return StationGapOpening{}, err
	}
	if r == nil || r.pool == nil {
		return StationGapOpening{}, fmt.Errorf("station gap opening requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationGapOpening{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireRunningStationAttemptTx(ctx, tx, record.Authority); err != nil {
		return StationGapOpening{}, err
	}
	if err := insertStationGapOpeningTx(ctx, tx, &opening); err != nil {
		return StationGapOpening{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationGapOpening{}, err
	}
	return opening, nil
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

func insertStationGapOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	opening *StationGapOpening,
) error {
	semanticUncertainty, err := canonicalStationGapSemanticUncertainty(*opening)
	if err != nil {
		return fmt.Errorf("persist exact station gap semantic uncertainty: %w", err)
	}
	err = scanStationGapOpening(tx.QueryRow(ctx, `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,semantic_uncertainty_contract,
			semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
			output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24
		)
		RETURNING id,job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,semantic_uncertainty_contract,
			semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
			output_limit_mode,created_at
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope, opening.PortableSchema,
		opening.WorkID, opening.WorkKind, opening.PortablePayload, opening.PortablePayloadSHA256,
		opening.PortableEnvelope, opening.PortableEnvelopeSHA256, opening.RendererVersion,
		opening.Prompt, opening.ProjectionEnvelope,
		opening.ProjectionSHA256, semanticUncertainty,
		opening.SemanticUncertaintyContractSHA256, opening.ContextTokens, opening.MaxOutputTokens,
		opening.OutputLimitMode), opening)
	if err != nil {
		return fmt.Errorf("persist exact station gap opening: %w", err)
	}
	return nil
}

func (r *Repository) CloseStationGap(
	ctx context.Context,
	record StationGapTerminalRecord,
) (StationGapOutcome, error) {
	if err := validateStationGapTerminal(record); err != nil {
		return StationGapOutcome{}, err
	}
	if r == nil || r.pool == nil {
		return StationGapOutcome{}, fmt.Errorf("station gap outcome requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return StationGapOutcome{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireStationGapClosingAuthorityTx(ctx, tx, record); err != nil {
		return StationGapOutcome{}, err
	}
	var outcome StationGapOutcome
	if err := insertStationGapOutcomeTx(ctx, tx, record, &outcome); err != nil {
		return StationGapOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationGapOutcome{}, err
	}
	return outcome, nil
}

func insertStationGapOutcomeTx(
	ctx context.Context,
	tx pgx.Tx,
	record StationGapTerminalRecord,
	outcome *StationGapOutcome,
) error {
	responseHash := ""
	if record.Response != "" {
		responseHash = stationGapSHA256(record.Response)
	}
	err := scanStationGapOutcome(tx.QueryRow(ctx, `
		INSERT INTO station_gap_outcomes (
			opening_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			status,response,response_sha256,projection_kind,call_receipt_sha256,
			source_response_sha256,source_start_byte,source_end_byte,error
		)
		SELECT openings.id,openings.job_id,openings.generation,openings.step_id,
			openings.step_attempt,openings.worker_id,openings.gap_id,$7,
			NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),
			NULLIF($12,''),$13,$14,NULLIF($15,'')
		FROM station_gap_openings AS openings
		WHERE openings.id=$1 AND openings.job_id=$2 AND openings.generation=$3
		  AND openings.step_id=$4 AND openings.step_attempt=$5
		  AND openings.worker_id=$6 AND openings.gap_id=$16
		RETURNING id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,status,response,response_sha256,projection_kind,call_receipt_sha256,
			source_response_sha256,source_start_byte,source_end_byte,error,created_at
	`, record.OpeningID, record.Authority.JobID, record.Authority.Generation,
		record.Authority.StepID, record.Authority.Attempt, record.Authority.WorkerID,
		string(record.Status), record.Response, responseHash,
		stationGapProjectionKind(record.Projection), stationGapProjectionCallReceipt(record.Projection),
		stationGapProjectionSourceResponse(record.Projection), stationGapProjectionStart(record.Projection),
		stationGapProjectionEnd(record.Projection), record.Error, record.GapID), outcome)
	if err != nil {
		return fmt.Errorf("persist exact station gap outcome: %w", err)
	}
	return nil
}

func stationGapProjectionKind(projection *StationGapSourceProjection) string {
	if projection == nil {
		return ""
	}
	return string(projection.Kind)
}

func stationGapProjectionCallReceipt(projection *StationGapSourceProjection) string {
	if projection == nil {
		return ""
	}
	return projection.CallReceiptSHA256
}

func stationGapProjectionSourceResponse(projection *StationGapSourceProjection) string {
	if projection == nil {
		return ""
	}
	return projection.SourceResponseSHA256
}

func stationGapProjectionStart(projection *StationGapSourceProjection) any {
	if projection == nil {
		return nil
	}
	return projection.StartByte
}

func stationGapProjectionEnd(projection *StationGapSourceProjection) any {
	if projection == nil {
		return nil
	}
	return projection.EndByte
}

func requireStationGapClosingAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	record StationGapTerminalRecord,
) error {
	opening, err := loadStationGapOpeningTx(ctx, tx, record.OpeningID)
	if err != nil {
		return err
	}
	if opening.JobID != record.Authority.JobID || opening.Generation != record.Authority.Generation ||
		opening.StepID != record.Authority.StepID || opening.StepAttempt != record.Authority.Attempt ||
		opening.WorkerID != record.Authority.WorkerID || opening.GapID != record.GapID {
		return fmt.Errorf("station gap outcome differs from its exact original attempt")
	}
	var status model.StepAttemptStatus
	var workerID string
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		FOR SHARE
	`, record.Authority.JobID, record.Authority.Generation,
		record.Authority.StepID, record.Authority.Attempt).Scan(&status, &workerID); err != nil {
		return err
	}
	if workerID != record.Authority.WorkerID {
		return fmt.Errorf("station gap outcome worker differs from its exact original attempt")
	}
	if status == model.StepAttemptActive {
		jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, record.Authority)
		if err != nil {
			return err
		}
		if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
			return fmt.Errorf("%w: station gap outcome attempt is not running", ErrStaleStepAttempt)
		}
		return nil
	}
	if record.Status != StationGapFailed {
		return fmt.Errorf("terminalized station gap attempt may only append a failed outcome")
	}
	switch status {
	case model.StepAttemptCanceled, model.StepAttemptSuperseded, model.StepAttemptExpired:
		return nil
	default:
		return fmt.Errorf("station gap outcome cannot append after attempt status %q", status)
	}
}
