package queue

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5"
)

type roleplayPortableReuseSource struct {
	Opening                StationGapOpening
	Outcome                StationGapOutcome
	Job                    model.Job
	AttemptStatus          model.StepAttemptStatus
	CallReceipt            string
	CallReceiptSHA256      string
	ProviderResponse       string
	ProviderResponseSHA256 string
}

func validateRoleplayPortableReuseRequest(
	request RoleplayPortableResultReuseRequest,
) ([]byte, error) {
	if err := validateStepAttemptAuthority(request.Authority); err != nil {
		return nil, fmt.Errorf("roleplay portable reuse authority: %w", err)
	}
	if err := request.Job.Validate(); err != nil {
		return nil, fmt.Errorf("roleplay portable reuse root job: %w", err)
	}
	if err := request.Station.Validate(); err != nil {
		return nil, err
	}
	expected, err := stationForPortableJob(request.Job)
	if err != nil {
		return nil, err
	}
	if request.Station != expected {
		return nil, fmt.Errorf(
			"roleplay portable reuse station %q does not own root work %q",
			request.Station, request.Job.ID,
		)
	}
	envelope, err := exactjson.Canonical(request.Job)
	if err != nil {
		return nil, fmt.Errorf("canonicalize roleplay portable reuse root job: %w", err)
	}
	return envelope, nil
}

func loadRoleplayPortableReuseSourceTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
	outcomeID int64,
) (roleplayPortableReuseSource, error) {
	opening, err := loadStationGapOpeningTx(ctx, tx, openingID)
	if err != nil {
		return roleplayPortableReuseSource{}, err
	}
	var outcome StationGapOutcome
	if err := scanStationGapOutcome(tx.QueryRow(ctx, `
		SELECT id,opening_id,job_id,generation,step_id,step_attempt,worker_id,
			gap_id,status,response,response_sha256,projection_kind,call_receipt_sha256,
			source_response_sha256,source_start_byte,source_end_byte,error,created_at
		FROM station_gap_outcomes WHERE id=$1 FOR SHARE
	`, outcomeID), &outcome); err != nil {
		return roleplayPortableReuseSource{}, err
	}
	job, err := scanJob(tx.QueryRow(ctx, `
		SELECT id,instruction,pipeline,status,result,error,metadata,current_generation,
			created_at,updated_at,completed_at
		FROM jobs WHERE id=$1 FOR SHARE
	`, opening.JobID))
	if err != nil {
		return roleplayPortableReuseSource{}, err
	}
	if err := requireRoleplayPortableReuseJobBindingTx(ctx, tx, job); err != nil {
		return roleplayPortableReuseSource{}, err
	}
	var attemptStatus model.StepAttemptStatus
	var attemptWorker string
	if err := tx.QueryRow(ctx, `
		SELECT status,worker_id FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 FOR SHARE
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt).Scan(
		&attemptStatus, &attemptWorker,
	); err != nil {
		return roleplayPortableReuseSource{}, err
	}
	if attemptWorker != opening.WorkerID {
		return roleplayPortableReuseSource{}, fmt.Errorf(
			"roleplay portable reuse source attempt worker differs from its opening",
		)
	}
	var callReceipt, callReceiptSHA256, providerResponse, providerResponseSHA256 string
	if err := tx.QueryRow(ctx, `
		SELECT receipt.generation_json,receipt.generation_sha256,
		       evidence.response,evidence.response_sha256
		FROM station_call_openings AS call
		JOIN station_call_receipts AS receipt ON receipt.opening_id=call.id
		JOIN llm_call_evidence AS evidence ON evidence.station_call_opening_id=call.id
		WHERE call.gap_opening_id=$1 AND receipt.status='succeeded'
	`, opening.ID).Scan(
		&callReceipt, &callReceiptSHA256, &providerResponse, &providerResponseSHA256,
	); err != nil {
		return roleplayPortableReuseSource{}, fmt.Errorf(
			"load roleplay portable reuse source call evidence: %w", err,
		)
	}
	return roleplayPortableReuseSource{
		Opening: opening, Outcome: outcome, Job: job, AttemptStatus: attemptStatus,
		CallReceipt: callReceipt, CallReceiptSHA256: callReceiptSHA256,
		ProviderResponse: providerResponse, ProviderResponseSHA256: providerResponseSHA256,
	}, nil
}

func validateRoleplayPortableReuseSource(
	targetAuthority model.StepAttemptAuthority,
	targetJob model.Job,
	root assemblyline.PortableJob,
	owner station.ID,
	targetRoleplayAuthority []byte,
	source roleplayPortableReuseSource,
) (assemblyline.PortableResult, bool, error) {
	opening, outcome := source.Opening, source.Outcome
	if opening.JobID == targetAuthority.JobID && opening.Generation == targetAuthority.Generation &&
		opening.StepID == targetAuthority.StepID && opening.StepAttempt == targetAuthority.Attempt &&
		opening.WorkerID == targetAuthority.WorkerID {
		return assemblyline.PortableResult{}, false, fmt.Errorf(
			"roleplay portable reuse source belongs to the current attempt",
		)
	}
	if err := validateRoleplayPortableReuseSourceEligibility(
		targetAuthority, targetJob, source,
	); err != nil {
		return assemblyline.PortableResult{}, false, err
	}
	sourceRoleplayAuthority, err := canonicalRoleplayPortableReuseAuthority(source.Job)
	if err != nil {
		return assemblyline.PortableResult{}, false, err
	}
	if !bytes.Equal(sourceRoleplayAuthority, targetRoleplayAuthority) {
		return assemblyline.PortableResult{}, false, nil
	}

	portable, err := validateRoleplayPortableReuseOpening(opening, owner)
	if err != nil {
		return assemblyline.PortableResult{}, false, err
	}
	if err := requireRoleplayPortableReuseRoot(portable, root); err != nil {
		return assemblyline.PortableResult{}, false, err
	}
	if outcome.OpeningID != opening.ID || outcome.JobID != opening.JobID ||
		outcome.Generation != opening.Generation || outcome.StepID != opening.StepID ||
		outcome.StepAttempt != opening.StepAttempt || outcome.WorkerID != opening.WorkerID ||
		outcome.GapID != opening.GapID {
		return assemblyline.PortableResult{}, false, fmt.Errorf(
			"roleplay portable reuse outcome differs from its source opening",
		)
	}
	if outcome.Status != StationGapResolved ||
		outcome.ProjectionKind != StationGapProjectionExactResponse ||
		strings.TrimSpace(outcome.Response) == "" || outcome.Error != "" ||
		outcome.ResponseSHA256 != stationGapSHA256(outcome.Response) ||
		outcome.SourceResponseSHA256 != outcome.ResponseSHA256 ||
		outcome.SourceStartByte != 0 || outcome.SourceEndByte != len(outcome.Response) ||
		len(outcome.CallReceiptSHA256) != 64 || !llmEvidenceLowerHex(outcome.CallReceiptSHA256) {
		return assemblyline.PortableResult{}, false, fmt.Errorf(
			"roleplay portable reuse outcome is not one exact resolved response",
		)
	}
	if source.CallReceiptSHA256 != stationGapSHA256(source.CallReceipt) ||
		source.CallReceiptSHA256 != outcome.CallReceiptSHA256 ||
		source.ProviderResponse != outcome.Response ||
		source.ProviderResponseSHA256 != stationGapSHA256(source.ProviderResponse) ||
		source.ProviderResponseSHA256 != outcome.SourceResponseSHA256 {
		return assemblyline.PortableResult{}, false, fmt.Errorf(
			"roleplay portable reuse source hashes differ from station call evidence",
		)
	}
	result, err := materializeRoleplayPortableReuseResult(portable, root, outcome.Response)
	if err != nil {
		return assemblyline.PortableResult{}, false, err
	}
	if err := result.ValidateFor(root); err != nil {
		return assemblyline.PortableResult{}, false, fmt.Errorf(
			"validate reused roleplay portable result: %w", err,
		)
	}
	return result, true, nil
}

func materializeRoleplayPortableReuseResult(
	source assemblyline.PortableJob,
	root assemblyline.PortableJob,
	rawResponse string,
) (assemblyline.PortableResult, error) {
	if source.Kind == assemblyline.WorkResponseCorrection {
		var correction assemblyline.ResponseCorrectionInput
		if err := decodePortableGapPayload(source.Payload, &correction); err != nil {
			return assemblyline.PortableResult{}, err
		}
		candidate, err := assemblyline.ApplyResponseCorrection(
			correction.Original, correction.RetainedCandidate, rawResponse,
		)
		if err != nil {
			return assemblyline.PortableResult{}, fmt.Errorf(
				"apply reused roleplay response correction: %w", err,
			)
		}
		return assemblyline.PortableResult{
			JobID: root.ID, Candidate: candidate,
		}, nil
	}
	projection, err := assemblyline.NewExactPortableResultProjection(rawResponse)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	return assemblyline.PortableResult{
		JobID: root.ID, Candidate: rawResponse, Projection: &projection,
	}, nil
}

func validateRoleplayPortableReuseSourceEligibility(
	target model.StepAttemptAuthority,
	targetJob model.Job,
	source roleplayPortableReuseSource,
) error {
	if source.Job.Status == model.JobStatusFailed && source.Job.ID != targetJob.ID {
		return nil
	}
	if source.Job.ID == targetJob.ID && source.Job.Status == model.JobStatusRunning &&
		source.Opening.Generation == target.Generation && source.Opening.StepID == target.StepID &&
		source.Opening.StepAttempt < target.Attempt {
		switch source.AttemptStatus {
		case model.StepAttemptExpired, model.StepAttemptSuperseded, model.StepAttemptCanceled:
			return nil
		}
	}
	return fmt.Errorf(
		"roleplay portable reuse source job %d status %q attempt status %q is ineligible",
		source.Job.ID, source.Job.Status, source.AttemptStatus,
	)
}

func validateRoleplayPortableReuseOpening(
	opening StationGapOpening,
	owner station.ID,
) (assemblyline.PortableJob, error) {
	if opening.ID < 1 || opening.Station != owner || opening.GapID != opening.WorkID ||
		opening.PortablePayloadSHA256 != stationGapSHA256(opening.PortablePayload) ||
		opening.PortableEnvelopeSHA256 != stationGapSHA256(opening.PortableEnvelope) ||
		opening.ProjectionSHA256 != stationGapSHA256(opening.ProjectionEnvelope) {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"roleplay portable reuse source opening identity or hashes are invalid",
		)
	}
	portable := assemblyline.PortableJob{
		Schema: opening.PortableSchema, ID: opening.WorkID,
		Kind:    assemblyline.WorkKind(opening.WorkKind),
		Payload: []byte(opening.PortablePayload),
	}
	if err := portable.Validate(); err != nil {
		return assemblyline.PortableJob{}, err
	}
	canonical, err := exactjson.Canonical(portable)
	if err != nil {
		return assemblyline.PortableJob{}, err
	}
	expectedOwner, err := stationForPortableJob(portable)
	if err != nil {
		return assemblyline.PortableJob{}, err
	}
	if !bytes.Equal(canonical, []byte(opening.PortableEnvelope)) ||
		portable.Schema != opening.PortableSchema || portable.ID != opening.WorkID ||
		string(portable.Kind) != opening.WorkKind || string(portable.Payload) != opening.PortablePayload ||
		expectedOwner != opening.Station {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"roleplay portable reuse source opening differs from its exact PortableJob",
		)
	}
	return portable, nil
}

func requireRoleplayPortableReuseRoot(
	source assemblyline.PortableJob,
	root assemblyline.PortableJob,
) error {
	rootEnvelope, err := exactjson.Canonical(root)
	if err != nil {
		return err
	}
	if source.Kind == assemblyline.WorkResponseCorrection {
		var correction assemblyline.ResponseCorrectionInput
		if err := decodePortableGapPayload(source.Payload, &correction); err != nil {
			return err
		}
		originalEnvelope, err := exactjson.Canonical(correction.Original)
		if err != nil {
			return err
		}
		if !bytes.Equal(originalEnvelope, rootEnvelope) {
			return fmt.Errorf("response correction reuse source wraps a different root job")
		}
		return nil
	}
	sourceEnvelope, err := exactjson.Canonical(source)
	if err != nil {
		return err
	}
	if !bytes.Equal(sourceEnvelope, rootEnvelope) {
		return fmt.Errorf("roleplay portable reuse source has a different root job")
	}
	return nil
}
