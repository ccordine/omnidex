package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type objectivePortableReuseCandidate struct {
	OpeningID int64
	OutcomeID int64
}

func listObjectivePortableReuseCandidatesTx(
	ctx context.Context,
	tx pgx.Tx,
	request ObjectivePortableResultReuseRequest,
	targetJob model.Job,
) ([]objectivePortableReuseCandidate, error) {
	roleplayJob, err := isRoleplayPortableReuseJob(targetJob)
	if err != nil {
		return nil, err
	}
	if !roleplayJob {
		return listSameJobObjectivePortableReuseCandidatesTx(ctx, tx, request, targetJob)
	}
	var targetMetadata channelTurnMetadata
	if err := json.Unmarshal(targetJob.Metadata, &targetMetadata); err != nil {
		return nil, fmt.Errorf("decode objective portable reuse target metadata: %w", err)
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
		      opening.work_id=$3 AND NOT (
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
	return scanObjectivePortableReuseCandidates(rows)
}

func listSameJobObjectivePortableReuseCandidatesTx(
	ctx context.Context,
	tx pgx.Tx,
	request ObjectivePortableResultReuseRequest,
	targetJob model.Job,
) ([]objectivePortableReuseCandidate, error) {
	if targetJob.ID != request.Authority.JobID {
		return nil, fmt.Errorf("objective portable reuse target differs from its exact job")
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
		      source_job.id=$2 AND source_job.status='running' AND
		      opening.work_id=$3 AND opening.generation=$4 AND
		      opening.step_id=$5 AND opening.step_attempt<$6 AND
		      source_attempt.status IN ('expired','superseded','canceled')
		ORDER BY outcome.created_at DESC,outcome.id DESC
		FOR SHARE OF opening,outcome,source_job,source_attempt
	`, request.Station, targetJob.ID, request.Job.ID,
		request.Authority.Generation, request.Authority.StepID, request.Authority.Attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjectivePortableReuseCandidates(rows)
}

type objectivePortableReuseCandidateRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanObjectivePortableReuseCandidates(
	rows objectivePortableReuseCandidateRows,
) ([]objectivePortableReuseCandidate, error) {
	var candidates []objectivePortableReuseCandidate
	for rows.Next() {
		var candidate objectivePortableReuseCandidate
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
