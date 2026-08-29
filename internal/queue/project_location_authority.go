package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// requireProjectLocationWithoutActiveWorkTx runs only after the caller has
// locked the project row. That project-before-job order matches job creation
// and workspace mutation preparation.
func requireProjectLocationWithoutActiveWorkTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
) error {
	var jobID int64
	var status string
	err := tx.QueryRow(ctx, `
		SELECT id,status
		FROM jobs
		WHERE project_id=$1 AND status NOT IN ($2,$3,$4)
		ORDER BY id
		LIMIT 1
		FOR SHARE
	`, projectID, model.JobStatusCompleted, model.JobStatusFailed,
		model.JobStatusCanceled).Scan(&jobID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect active work before project location change: %w", err)
	}
	return fmt.Errorf("%w: job %d remains %q", ErrProjectActiveWork, jobID, status)
}
