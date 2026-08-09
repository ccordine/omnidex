package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxClaimedStepContextItems = 256
	maxClaimedStepContextBytes = 256 << 10
)

func loadClaimedStepContexts(
	ctx context.Context,
	tx pgx.Tx,
	jobID, currentStepID int64,
) ([]model.StepContext, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.step_id, c.key, c.value, c.created_at
		FROM step_contexts AS c
		JOIN job_steps AS steps ON steps.id=c.step_id
		WHERE steps.job_id=$1
		  AND steps.superseded_at_generation IS NULL
		  AND (steps.status=$2 OR steps.id=$3)
		ORDER BY c.id ASC
		LIMIT $4
	`, jobID, model.StepStatusCompleted, currentStepID, maxClaimedStepContextItems+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contexts := make([]model.StepContext, 0, 8)
	totalBytes := 0
	for rows.Next() {
		item, err := scanStepContext(rows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, item)
		if len(contexts) > maxClaimedStepContextItems {
			return nil, fmt.Errorf(
				"%w: job %d needs more than %d step-context items",
				ErrContextProjectionBudget, jobID, maxClaimedStepContextItems,
			)
		}
		totalBytes += len(item.Key) + len(item.Value)
		if totalBytes > maxClaimedStepContextBytes {
			return nil, fmt.Errorf(
				"%w: job %d step contexts require %d bytes; maximum is %d",
				ErrContextProjectionBudget, jobID, totalBytes, maxClaimedStepContextBytes,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contexts, nil
}
