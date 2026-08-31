package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireNoOpenStationGapsTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
) error {
	var openingID int64
	err := tx.QueryRow(ctx, `
		SELECT openings.id
		FROM station_gap_openings AS openings
		LEFT JOIN station_gap_outcomes AS outcomes ON outcomes.opening_id=openings.id
		WHERE openings.job_id=$1 AND openings.generation=$2 AND openings.step_id=$3
		  AND openings.step_attempt=$4 AND openings.worker_id=$5
		  AND outcomes.id IS NULL
		ORDER BY openings.id
		LIMIT 1
		FOR UPDATE OF openings
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(&openingID)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("step %d attempt %d has open semantic gap %d", authority.StepID, authority.Attempt, openingID)
}
