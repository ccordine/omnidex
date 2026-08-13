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
	var discoveryOpeningID int64
	err := tx.QueryRow(ctx, `
		SELECT discoveries.id
		FROM station_provider_discoveries AS discoveries
		LEFT JOIN station_provider_discovery_receipts AS receipts ON receipts.opening_id=discoveries.id
		WHERE discoveries.job_id=$1 AND discoveries.generation=$2 AND discoveries.step_id=$3
		  AND discoveries.step_attempt=$4 AND discoveries.worker_id=$5
		  AND receipts.id IS NULL
		ORDER BY discoveries.id
		LIMIT 1
		FOR UPDATE OF discoveries
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(&discoveryOpeningID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		return fmt.Errorf("step %d attempt %d has open provider discovery %d", authority.StepID, authority.Attempt, discoveryOpeningID)
	}
	var callOpeningID int64
	err = tx.QueryRow(ctx, `
		SELECT calls.id
		FROM station_call_openings AS calls
		LEFT JOIN station_call_receipts AS receipts ON receipts.opening_id=calls.id
		WHERE calls.job_id=$1 AND calls.generation=$2 AND calls.step_id=$3
		  AND calls.step_attempt=$4 AND calls.worker_id=$5
		  AND receipts.id IS NULL
		ORDER BY calls.id
		LIMIT 1
		FOR UPDATE OF calls
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(&callOpeningID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		return fmt.Errorf("step %d attempt %d has open station call %d", authority.StepID, authority.Attempt, callOpeningID)
	}
	var callWithoutEvidenceID int64
	err = tx.QueryRow(ctx, `
		SELECT calls.id
		FROM station_call_openings AS calls
		JOIN station_call_receipts AS receipts ON receipts.opening_id=calls.id
		LEFT JOIN llm_call_evidence AS evidence ON evidence.station_call_opening_id=calls.id
		WHERE calls.job_id=$1 AND calls.generation=$2 AND calls.step_id=$3
		  AND calls.step_attempt=$4 AND calls.worker_id=$5
		  AND evidence.id IS NULL
		ORDER BY calls.id
		LIMIT 1
		FOR UPDATE OF calls
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(&callWithoutEvidenceID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		return fmt.Errorf(
			"step %d attempt %d has station call %d without immutable evidence",
			authority.StepID, authority.Attempt, callWithoutEvidenceID,
		)
	}
	var openingID int64
	err = tx.QueryRow(ctx, `
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
