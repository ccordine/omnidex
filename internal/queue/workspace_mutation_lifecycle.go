package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func rejectUnresolvedWorkspaceMutationsTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) error {
	var operationID, status string
	err := tx.QueryRow(ctx, `
		SELECT id,status FROM workspace_mutation_operations
		WHERE job_id=$1 AND generation=$2 AND status NOT IN ($3,$4)
		ORDER BY id LIMIT 1 FOR UPDATE
	`, jobID, generation, workspaceMutationVerified,
		workspaceMutationVerificationFailed).Scan(&operationID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check unresolved workspace mutations for job %d: %w", jobID, err)
	}
	return fmt.Errorf(
		"%w: job %d generation %d has workspace mutation %s in state %q",
		ErrWorkspaceMutationUnresolved, jobID, generation, operationID, status,
	)
}
