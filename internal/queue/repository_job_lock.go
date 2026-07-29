package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockedJobTx(ctx context.Context, tx pgx.Tx, jobID int64) (model.Job, error) {
	return scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID))
}
