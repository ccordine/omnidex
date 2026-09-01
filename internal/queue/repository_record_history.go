package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func readHistoricalEvidencePage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalEvidence, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT record.id, record.job_id, record.step_id, record.payload_json, record.created_at,
		       steps.generation, steps.superseded_at_generation
		FROM evidence AS record
		JOIN job_steps AS steps
		  ON steps.job_id=record.job_id AND steps.id=record.step_id
		WHERE record.job_id=$1 AND record.id>$2
		ORDER BY record.id ASC
		LIMIT $3
	`, jobID, afterID, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d historical evidence: %w", jobID, err)
	}
	defer rows.Close()

	items := make([]HistoricalEvidence, 0, limit+1)
	for rows.Next() {
		var item HistoricalEvidence
		var id, recordJobID, stepID int64
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(
			&id, &recordJobID, &stepID, &payload, &createdAt,
			&item.Step.Generation, &item.Step.SupersededAtGeneration,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d historical evidence: %w", jobID, err)
		}
		if err := json.Unmarshal(payload, &item.Evidence); err != nil {
			return nil, "", fmt.Errorf("decode historical evidence %d: %w", id, err)
		}
		item.Evidence.ID = id
		item.Evidence.JobID = recordJobID
		item.Evidence.StepID = stepID
		item.Evidence.CreatedAt = createdAt
		item.Step.JobID = recordJobID
		item.Step.StepID = stepID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate job %d historical evidence: %w", jobID, err)
	}
	return finishJobHistoryPage(jobID, JobHistoryEvidence, items, limit, func(item HistoricalEvidence) int64 {
		return item.Evidence.ID
	})
}
