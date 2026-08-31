package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadJobHistoryPage(
	ctx context.Context,
	jobID int64,
	request JobHistoryRequest,
) (JobHistoryPage, error) {
	position, err := validateJobHistoryRequest(jobID, request)
	if err != nil {
		return JobHistoryPage{}, err
	}
	if r == nil || r.pool == nil {
		return JobHistoryPage{}, fmt.Errorf("job history requires PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return JobHistoryPage{}, fmt.Errorf("begin job %d history read: %w", jobID, err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM jobs WHERE id=$1)`, jobID).Scan(&exists); err != nil {
		return JobHistoryPage{}, fmt.Errorf("resolve job %d history authority: %w", jobID, err)
	}
	if !exists {
		return JobHistoryPage{}, pgx.ErrNoRows
	}

	page := JobHistoryPage{JobID: jobID, Stream: request.Stream}
	switch request.Stream {
	case JobHistoryGenerations:
		page.Generations, page.NextCursor, err = readJobGenerationHistoryPage(
			ctx, tx, jobID, position, request.Limit,
		)
	case JobHistorySteps:
		page.Steps, page.NextCursor, err = readHistoricalStepPage(
			ctx, tx, jobID, position, request.Limit,
		)
	case JobHistoryEvidence:
		page.Evidence, page.NextCursor, err = readHistoricalEvidencePage(
			ctx, tx, jobID, position, request.Limit,
		)
	default:
		err = fmt.Errorf("%w: stream %q is not registered", ErrInvalidJobHistoryRequest, request.Stream)
	}
	if err != nil {
		return JobHistoryPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JobHistoryPage{}, fmt.Errorf("commit job %d history read: %w", jobID, err)
	}
	return page, nil
}

func readJobGenerationHistoryPage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterGeneration int64,
	limit int,
) ([]JobGenerationHistory, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT job_id, generation, purpose, predecessor_generation,
		       boundary_action, feedback, feedback_sha256, created_at
		FROM job_generations
		WHERE job_id=$1 AND generation>$2
		ORDER BY generation ASC
		LIMIT $3
	`, jobID, afterGeneration, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d generations: %w", jobID, err)
	}
	defer rows.Close()

	items := make([]JobGenerationHistory, 0, limit+1)
	for rows.Next() {
		var item JobGenerationHistory
		var boundary, feedback, feedbackSHA *string
		if err := rows.Scan(
			&item.JobID, &item.Generation, &item.Purpose, &item.PredecessorGeneration,
			&boundary, &feedback, &feedbackSHA, &item.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d generation history: %w", jobID, err)
		}
		item.BoundaryAction = stringOrEmpty(boundary)
		item.Feedback = stringOrEmpty(feedback)
		item.FeedbackSHA256 = stringOrEmpty(feedbackSHA)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate job %d generation history: %w", jobID, err)
	}
	return finishJobHistoryPage(jobID, JobHistoryGenerations, items, limit, func(item JobGenerationHistory) int64 {
		return item.Generation
	})
}

func readHistoricalStepPage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalStep, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, job_id, generation, superseded_at_generation, action, sort_index,
		       status, worker_id, started_at, finished_at, created_at, updated_at
		FROM job_steps
		WHERE job_id=$1 AND id>$2
		ORDER BY id ASC
		LIMIT $3
	`, jobID, afterID, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d historical steps: %w", jobID, err)
	}
	defer rows.Close()

	items := make([]HistoricalStep, 0, limit+1)
	for rows.Next() {
		var item HistoricalStep
		var workerID *string
		if err := rows.Scan(
			&item.StepID, &item.JobID, &item.Generation, &item.SupersededAtGeneration,
			&item.Action, &item.SortIndex, &item.Status, &workerID, &item.StartedAt,
			&item.FinishedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d historical step: %w", jobID, err)
		}
		item.WorkerID = stringOrEmpty(workerID)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate job %d historical steps: %w", jobID, err)
	}
	return finishJobHistoryPage(jobID, JobHistorySteps, items, limit, func(item HistoricalStep) int64 {
		return item.StepID
	})
}
