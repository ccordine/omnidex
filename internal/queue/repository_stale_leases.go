package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// CheckStaleV3StepLeases fails instead of recycling a running step identity.
// Recovery will become writable only when every worker mutation is bound to a
// persisted monotonically increasing attempt lease.
func (r *Repository) CheckStaleV3StepLeases(ctx context.Context, staleBefore time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("check stale V3 step leases: repository is unavailable")
	}
	if staleBefore.IsZero() {
		return fmt.Errorf("check stale V3 step leases: stale cutoff is required")
	}
	var stepID, jobID int64
	var workerID string
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT steps.id, steps.job_id, COALESCE(steps.worker_id, ''), steps.updated_at
		FROM job_steps AS steps
		JOIN jobs ON jobs.id=steps.job_id
		WHERE steps.status=$1
		  AND LEFT(steps.action, 3)='v3_'
		  AND steps.updated_at<$2
		  AND steps.superseded_at_generation IS NULL
		  AND steps.generation=jobs.current_generation
		  AND jobs.status IN ($3, $4)
		ORDER BY steps.updated_at ASC, steps.id ASC
		LIMIT 1
	`, model.StepStatusRunning, staleBefore.UTC(), model.JobStatusPending, model.JobStatusRunning).Scan(
		&stepID, &jobID, &workerID, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "unknown"
	}
	return fmt.Errorf(
		"%w: job %d step %d held by %q has been running since %s; automatic identity reuse is forbidden",
		ErrStepLeaseRequired, jobID, stepID, workerID, updatedAt.UTC().Format(time.RFC3339),
	)
}
