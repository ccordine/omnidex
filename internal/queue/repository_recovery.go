package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// RecoverStaleV3Steps releases expired V3 worker leases. Coding work resumes
// from the ordinary workspace left by atomic tool calls; it has no separate
// source-version or orchestration ledger to replay.
func (r *Repository) RecoverStaleV3Steps(ctx context.Context, staleBefore time.Time) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, fmt.Errorf("recover stale V3 steps: repository is unavailable")
	}
	if staleBefore.IsZero() {
		return 0, fmt.Errorf("recover stale V3 steps: stale cutoff is required")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT s.id, s.job_id, COALESCE(s.worker_id, '') AS previous_worker
			FROM job_steps s
			JOIN jobs j ON j.id = s.job_id
			WHERE s.status = $1
			  AND LEFT(s.action, 3) = 'v3_'
			  AND s.updated_at < $2
			  AND j.status IN ($3, $4)
			FOR UPDATE OF s SKIP LOCKED
		), recovered AS (
			UPDATE job_steps s
			SET status = $5,
			    worker_id = NULL,
			    started_at = NULL,
			    finished_at = NULL,
			    error = NULL,
			    updated_at = NOW()
			FROM candidates c
			WHERE s.id = c.id
			RETURNING s.id, c.job_id, c.previous_worker
		)
		SELECT id, job_id, previous_worker FROM recovered
	`, model.StepStatusRunning, staleBefore.UTC(), model.JobStatusPending, model.JobStatusRunning, model.StepStatusPending)
	if err != nil {
		return 0, err
	}
	type recoveredStep struct {
		id             int64
		jobID          int64
		previousWorker string
	}
	recovered := make([]recoveredStep, 0)
	for rows.Next() {
		var item recoveredStep
		if err := rows.Scan(&item.id, &item.jobID, &item.previousWorker); err != nil {
			rows.Close()
			return 0, err
		}
		recovered = append(recovered, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, item := range recovered {
		worker := strings.Join(strings.Fields(item.previousWorker), "_")
		if worker == "" {
			worker = "unknown"
		}
		event := fmt.Sprintf(
			"time=%s event=step_recovered reason=stale_v3_worker_lease previous_worker=%s",
			time.Now().UTC().Format(time.RFC3339), worker,
		)
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, 'event', $2)
		`, item.id, event); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET updated_at = NOW() WHERE id = $1`, item.jobID); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(recovered)), nil
}
