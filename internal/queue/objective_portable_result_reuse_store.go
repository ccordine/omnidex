package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const objectivePortableResultReuseColumns = `
	id,receipt_schema,
	target_job_id,target_generation,target_step_id,target_step_attempt,target_worker_id,
	target_station,target_root_work_id,target_work_kind,target_portable_payload,
	target_portable_payload_sha256,target_portable_envelope,target_portable_envelope_sha256,
	source_job_id,source_generation,source_step_id,source_step_attempt,source_worker_id,
	source_gap_opening_id,source_gap_outcome_id,source_work_id,
	source_portable_envelope_sha256,source_call_receipt_sha256,source_response_sha256,
	objective_authority,objective_authority_sha256,created_at`

// ReuseObjectivePortableResult finds and atomically receipts one prior exact
// accepted response. A false result means no eligible prior response exists;
// malformed or ambiguous persisted authority is always an error.
func (r *Repository) ReuseObjectivePortableResult(
	ctx context.Context,
	request ObjectivePortableResultReuseRequest,
) (ObjectivePortableResultReuse, bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ObjectivePortableResultReuse{}, false, fmt.Errorf(
			"objective portable result reuse requires PostgreSQL and context",
		)
	}
	targetEnvelope, err := validateObjectivePortableReuseRequest(request)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, request.Authority)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return ObjectivePortableResultReuse{}, false, staleStepAttemptError(
			request.Authority, "objective portable reuse target is not running", nil,
		)
	}
	targetJob, err := lockedJobTx(ctx, tx, request.Authority.JobID)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	targetObjectiveAuthority, targetRoleplay, err := canonicalObjectivePortableReuseAuthority(targetJob)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	boundRoleplay, err := requireObjectivePortableReuseJobBindingTx(ctx, tx, targetJob)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	if boundRoleplay != targetRoleplay {
		return ObjectivePortableResultReuse{}, false, fmt.Errorf(
			"objective portable reuse authority branch differs from its binding",
		)
	}

	existing, exists, err := loadCurrentObjectivePortableReuseTx(ctx, tx, request)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	if exists {
		reuse, err := validatePersistedObjectivePortableReuseTx(
			ctx, tx, request, targetJob, targetEnvelope, targetObjectiveAuthority, existing,
		)
		if err != nil {
			return ObjectivePortableResultReuse{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ObjectivePortableResultReuse{}, false, err
		}
		return reuse, true, nil
	}

	candidates, err := listObjectivePortableReuseCandidatesTx(ctx, tx, request, targetJob)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	// Candidate order selects provenance only after every matching source has
	// materialized the same complete result. No receipt exists while ambiguity
	// is still possible.
	var selectedSource objectivePortableReuseSource
	var selectedResult assemblyline.PortableResult
	selected := false
	for _, candidate := range candidates {
		source, err := loadObjectivePortableReuseSourceTx(
			ctx, tx, candidate.OpeningID, candidate.OutcomeID,
		)
		if err != nil {
			return ObjectivePortableResultReuse{}, false, err
		}
		result, matches, err := validateObjectivePortableReuseSource(
			request.Authority, targetJob, request.Job, request.Station,
			targetObjectiveAuthority, source,
		)
		if err != nil {
			return ObjectivePortableResultReuse{}, false, err
		}
		if !matches {
			continue
		}
		if !selected {
			selectedSource = source
			selectedResult = result
			selected = true
			continue
		}
		if !sameObjectivePortableReuseResult(selectedResult, result) {
			return ObjectivePortableResultReuse{}, false, fmt.Errorf(
				"objective portable reuse sources %d and %d resolve root work %q to divergent exact results",
				selectedSource.Outcome.ID, source.Outcome.ID, request.Job.ID,
			)
		}
	}
	if !selected {
		if err := tx.Commit(ctx); err != nil {
			return ObjectivePortableResultReuse{}, false, err
		}
		return ObjectivePortableResultReuse{}, false, nil
	}
	row, err := insertObjectivePortableReuseTx(
		ctx, tx, request, targetEnvelope, targetObjectiveAuthority, selectedSource,
	)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	reuse, err := validatePersistedObjectivePortableReuse(
		request, targetEnvelope, targetObjectiveAuthority, row, selectedSource, selectedResult,
	)
	if err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ObjectivePortableResultReuse{}, false, err
	}
	return reuse, true, nil
}

func sameObjectivePortableReuseResult(
	left assemblyline.PortableResult,
	right assemblyline.PortableResult,
) bool {
	// Projection records how code recovered Candidate from one model response;
	// it is provenance, not another semantic result. Direct and corrected calls
	// can therefore resolve the same root to the same exact candidate while
	// legitimately carrying different projection metadata.
	return left.JobID == right.JobID && left.Candidate == right.Candidate
}

func loadCurrentObjectivePortableReuseTx(
	ctx context.Context,
	tx pgx.Tx,
	request ObjectivePortableResultReuseRequest,
) (objectivePortableResultReuseRow, bool, error) {
	var row objectivePortableResultReuseRow
	err := scanObjectivePortableResultReuse(tx.QueryRow(ctx, `
		SELECT `+objectivePortableResultReuseColumns+`
		FROM objective_portable_result_reuses
		WHERE target_job_id=$1 AND target_generation=$2 AND target_step_id=$3 AND
		      target_step_attempt=$4 AND target_worker_id=$5 AND
		      target_station=$6 AND target_root_work_id=$7
		FOR SHARE
	`, request.Authority.JobID, request.Authority.Generation, request.Authority.StepID,
		request.Authority.Attempt, request.Authority.WorkerID, request.Station,
		request.Job.ID), &row)
	if errors.Is(err, pgx.ErrNoRows) {
		return objectivePortableResultReuseRow{}, false, nil
	}
	if err != nil {
		return objectivePortableResultReuseRow{}, false, err
	}
	return row, true, nil
}
