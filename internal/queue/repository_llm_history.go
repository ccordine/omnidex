package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func readHistoricalLLMCallPage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalLLMCall, string, error) {
	rows, err := tx.Query(ctx, `SELECT `+llmCallEvidenceSelectColumns+`
		FROM llm_call_evidence AS calls
		WHERE calls.job_id=$1 AND calls.id>$2
		ORDER BY calls.id ASC
		LIMIT $3
	`, jobID, afterID, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d historical LLM calls: %w", jobID, err)
	}

	items := make([]HistoricalLLMCall, 0, limit+1)
	stepIDs := make([]int64, 0, limit+1)
	for rows.Next() {
		var item HistoricalLLMCall
		if err := scanLLMCallEvidence(rows, &item.Call); err != nil {
			rows.Close()
			return nil, "", fmt.Errorf("scan job %d historical LLM call: %w", jobID, err)
		}
		items = append(items, item)
		stepIDs = append(stepIDs, item.Call.StepID)
	}
	iterationErr := rows.Err()
	rows.Close()
	if iterationErr != nil {
		return nil, "", fmt.Errorf("iterate job %d historical LLM calls: %w", jobID, iterationErr)
	}
	if len(items) == 0 {
		return items, "", nil
	}

	references, err := loadHistoricalStepReferences(ctx, tx, jobID, stepIDs)
	if err != nil {
		return nil, "", err
	}
	for index := range items {
		reference, exists := references[items[index].Call.StepID]
		if !exists {
			return nil, "", fmt.Errorf(
				"job %d historical LLM call %d has no immutable step %d",
				jobID, items[index].Call.ID, items[index].Call.StepID,
			)
		}
		if items[index].Call.JobGeneration != reference.Generation {
			return nil, "", fmt.Errorf(
				"job %d historical LLM call %d generation %d does not match step %d generation %d",
				jobID, items[index].Call.ID, items[index].Call.JobGeneration,
				reference.StepID, reference.Generation,
			)
		}
		items[index].Step = reference
	}
	return finishJobHistoryPage(jobID, JobHistoryLLMCalls, items, limit, func(item HistoricalLLMCall) int64 {
		return item.Call.ID
	})
}

func loadHistoricalStepReferences(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	stepIDs []int64,
) (map[int64]HistoricalStepReference, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, job_id, generation, superseded_at_generation
		FROM job_steps
		WHERE job_id=$1 AND id=ANY($2::bigint[])
		ORDER BY id ASC
	`, jobID, stepIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve job %d historical LLM steps: %w", jobID, err)
	}
	defer rows.Close()
	references := make(map[int64]HistoricalStepReference, len(stepIDs))
	for rows.Next() {
		var item HistoricalStepReference
		if err := rows.Scan(
			&item.StepID, &item.JobID, &item.Generation, &item.SupersededAtGeneration,
		); err != nil {
			return nil, fmt.Errorf("scan job %d historical LLM step: %w", jobID, err)
		}
		references[item.StepID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job %d historical LLM steps: %w", jobID, err)
	}
	return references, nil
}
