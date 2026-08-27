package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const roleplayPortableResultReuseColumns = `
	id,receipt_schema,
	target_job_id,target_generation,target_step_id,target_step_attempt,target_worker_id,
	target_station,target_root_work_id,target_work_kind,target_portable_payload,
	target_portable_payload_sha256,target_portable_envelope,target_portable_envelope_sha256,
	source_job_id,source_generation,source_step_id,source_step_attempt,source_worker_id,
	source_gap_opening_id,source_gap_outcome_id,source_work_id,
	source_portable_envelope_sha256,source_call_receipt_sha256,source_response_sha256,
	roleplay_authority,roleplay_authority_sha256,created_at`

// ReuseRoleplayPortableResult finds and atomically receipts one prior exact
// accepted response. A false result means no eligible prior response exists;
// malformed or ambiguous persisted authority is always an error.
func (r *Repository) ReuseRoleplayPortableResult(
	ctx context.Context,
	request RoleplayPortableResultReuseRequest,
) (RoleplayPortableResultReuse, bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return RoleplayPortableResultReuse{}, false, fmt.Errorf(
			"roleplay portable result reuse requires PostgreSQL and context",
		)
	}
	targetEnvelope, err := validateRoleplayPortableReuseRequest(request)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, request.Authority)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return RoleplayPortableResultReuse{}, false, staleStepAttemptError(
			request.Authority, "roleplay portable reuse target is not running", nil,
		)
	}
	targetJob, err := lockedJobTx(ctx, tx, request.Authority.JobID)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	targetRoleplayAuthority, err := canonicalRoleplayPortableReuseAuthority(targetJob)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	if err := requireRoleplayPortableReuseJobBindingTx(ctx, tx, targetJob); err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}

	existing, exists, err := loadCurrentRoleplayPortableReuseTx(ctx, tx, request)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	if exists {
		reuse, err := validatePersistedRoleplayPortableReuseTx(
			ctx, tx, request, targetJob, targetEnvelope, targetRoleplayAuthority, existing,
		)
		if err != nil {
			return RoleplayPortableResultReuse{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RoleplayPortableResultReuse{}, false, err
		}
		return reuse, true, nil
	}

	candidates, err := listRoleplayPortableReuseCandidatesTx(ctx, tx, request, targetJob)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	// Candidate order selects provenance only after every matching source has
	// materialized the same complete result. No receipt exists while ambiguity
	// is still possible.
	var selectedSource roleplayPortableReuseSource
	var selectedResult assemblyline.PortableResult
	selected := false
	for _, candidate := range candidates {
		source, err := loadRoleplayPortableReuseSourceTx(
			ctx, tx, candidate.OpeningID, candidate.OutcomeID,
		)
		if err != nil {
			return RoleplayPortableResultReuse{}, false, err
		}
		result, matches, err := validateRoleplayPortableReuseSource(
			request.Authority, targetJob, request.Job, request.Station,
			targetRoleplayAuthority, source,
		)
		if err != nil {
			return RoleplayPortableResultReuse{}, false, err
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
		if !sameRoleplayPortableReuseResult(selectedResult, result) {
			return RoleplayPortableResultReuse{}, false, fmt.Errorf(
				"roleplay portable reuse sources %d and %d resolve root work %q to divergent exact results",
				selectedSource.Outcome.ID, source.Outcome.ID, request.Job.ID,
			)
		}
	}
	if !selected {
		if err := tx.Commit(ctx); err != nil {
			return RoleplayPortableResultReuse{}, false, err
		}
		return RoleplayPortableResultReuse{}, false, nil
	}
	row, err := insertRoleplayPortableReuseTx(
		ctx, tx, request, targetEnvelope, targetRoleplayAuthority, selectedSource,
	)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	reuse, err := validatePersistedRoleplayPortableReuse(
		request, targetEnvelope, targetRoleplayAuthority, row, selectedSource, selectedResult,
	)
	if err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RoleplayPortableResultReuse{}, false, err
	}
	return reuse, true, nil
}

func sameRoleplayPortableReuseResult(
	left assemblyline.PortableResult,
	right assemblyline.PortableResult,
) bool {
	// Projection records how code recovered Candidate from one model response;
	// it is provenance, not another semantic result. Direct and corrected calls
	// can therefore resolve the same root to the same exact candidate while
	// legitimately carrying different projection metadata.
	return left.JobID == right.JobID && left.Candidate == right.Candidate
}

func loadCurrentRoleplayPortableReuseTx(
	ctx context.Context,
	tx pgx.Tx,
	request RoleplayPortableResultReuseRequest,
) (roleplayPortableResultReuseRow, bool, error) {
	var row roleplayPortableResultReuseRow
	err := scanRoleplayPortableResultReuse(tx.QueryRow(ctx, `
		SELECT `+roleplayPortableResultReuseColumns+`
		FROM roleplay_portable_result_reuses
		WHERE target_job_id=$1 AND target_generation=$2 AND target_step_id=$3 AND
		      target_step_attempt=$4 AND target_worker_id=$5 AND
		      target_station=$6 AND target_root_work_id=$7
		FOR SHARE
	`, request.Authority.JobID, request.Authority.Generation, request.Authority.StepID,
		request.Authority.Attempt, request.Authority.WorkerID, request.Station,
		request.Job.ID), &row)
	if errors.Is(err, pgx.ErrNoRows) {
		return roleplayPortableResultReuseRow{}, false, nil
	}
	if err != nil {
		return roleplayPortableResultReuseRow{}, false, err
	}
	return row, true, nil
}

type roleplayPortableReuseCandidate struct {
	OpeningID int64
	OutcomeID int64
}

func listRoleplayPortableReuseCandidatesTx(
	ctx context.Context,
	tx pgx.Tx,
	request RoleplayPortableResultReuseRequest,
	targetJob model.Job,
) ([]roleplayPortableReuseCandidate, error) {
	var targetMetadata channelTurnMetadata
	if err := json.Unmarshal(targetJob.Metadata, &targetMetadata); err != nil {
		return nil, fmt.Errorf("decode roleplay portable reuse target metadata: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT opening.id,outcome.id
		FROM station_gap_openings AS opening
		JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
		JOIN jobs AS source_job ON source_job.id=opening.job_id
		JOIN job_step_attempts AS source_attempt ON
		     source_attempt.job_id=opening.job_id AND
		     source_attempt.generation=opening.generation AND
		     source_attempt.step_id=opening.step_id AND
		     source_attempt.attempt=opening.step_attempt
		WHERE opening.station=$1 AND
		      outcome.status='resolved' AND outcome.projection_kind='exact_response' AND
		      source_job.pipeline='chat' AND source_job.metadata->>'channel_mode'='roleplay' AND
		      source_job.metadata->>'channel_id'=$2 AND
		      (
		        opening.work_id=$3 OR
		        (opening.work_kind='response_correction' AND
		         opening.portable_payload::jsonb->'original'->>'id'=$3)
		      ) AND NOT (
		        opening.job_id=$4 AND opening.generation=$5 AND opening.step_id=$6 AND
		        opening.step_attempt=$7 AND opening.worker_id=$8
		      ) AND (
		        (source_job.status='failed' AND source_job.id<>$4) OR
		        (source_job.id=$4 AND source_job.status='running' AND
		         opening.generation=$5 AND opening.step_id=$6 AND opening.step_attempt<$7 AND
		         source_attempt.status IN ('expired','superseded','canceled'))
		      )
		ORDER BY outcome.created_at DESC,outcome.id DESC
		FOR SHARE OF opening,outcome,source_job,source_attempt
	`, request.Station, targetMetadata.ChannelID, request.Job.ID,
		request.Authority.JobID, request.Authority.Generation, request.Authority.StepID,
		request.Authority.Attempt, request.Authority.WorkerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []roleplayPortableReuseCandidate
	for rows.Next() {
		var candidate roleplayPortableReuseCandidate
		if err := rows.Scan(&candidate.OpeningID, &candidate.OutcomeID); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}
