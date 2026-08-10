package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func rejectUnresolvedRepositoryMutationsTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) error {
	var operationID, status string
	err := tx.QueryRow(ctx, `
		SELECT id, status
		FROM repository_mutation_operations
		WHERE job_id=$1 AND generation=$2
		  AND status IN ($3,$4,$5)
		ORDER BY id
		LIMIT 1
		FOR UPDATE
	`, jobID, generation, repositoryMutationPrepared,
		repositoryMutationApplying, repositoryMutationIndeterminate).Scan(
		&operationID, &status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check unresolved repository mutations for job %d: %w", jobID, err)
	}
	return fmt.Errorf(
		"%w: job %d generation %d has repository mutation %s in state %q",
		ErrRepositoryMutationUnresolved, jobID, generation, operationID, status,
	)
}
