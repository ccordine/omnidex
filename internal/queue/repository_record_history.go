package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/evidence"

	"github.com/jackc/pgx/v5"
)

func readHistoricalEvidencePage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalEvidence, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT record.id, record.job_id, record.step_id, record.kind,
		       COALESCE(record.source_type,''), COALESCE(record.source_ref,''),
		       record.payload_json, record.created_at,
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
		var kind, sourceType, sourceRef string
		var createdAt time.Time
		if err := rows.Scan(
			&id, &recordJobID, &stepID, &kind, &sourceType, &sourceRef, &payload, &createdAt,
			&item.Step.Generation, &item.Step.SupersededAtGeneration,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d historical evidence: %w", jobID, err)
		}
		item.Evidence, err = historicalEvidenceRecord(kind, sourceType, sourceRef, payload)
		if err != nil {
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

// Native journals and session receipts share the evidence table with public
// records, but have different payload types. Project their public facts instead
// of decoding them as Record or exposing raw model envelopes and wire frames.
func historicalEvidenceRecord(kind, sourceType, sourceRef string, payload []byte) (evidence.Record, error) {
	var record evidence.Record
	switch kind {
	case "coding_agent":
		var entry struct {
			Phase        string   `json:"phase"`
			Role         string   `json:"role"`
			TaskID       string   `json:"task_id"`
			Generation   int64    `json:"generation"`
			Notes        []string `json:"notes"`
			WrittenFiles []string `json:"written_files"`
		}
		if err := json.Unmarshal(payload, &entry); err != nil {
			return record, err
		}
		record = evidence.Record{Kind: kind, SourceType: sourceType, SourceRef: sourceRef,
			Summary: entry.Phase, FilePaths: entry.WrittenFiles,
			Metadata: map[string]any{"phase": entry.Phase, "role": entry.Role, "task_id": entry.TaskID, "generation": entry.Generation}}
		if len(entry.Notes) == 1 && (entry.Phase == "execution_response" || entry.Phase == "review_response" || entry.Phase == "task_incomplete" || entry.Phase == "planning_incomplete") {
			record.Summary = entry.Notes[0]
		}
	case "channel_session_message", "coding_planning_amendment":
		record = evidence.Record{Kind: kind, SourceType: sourceType, SourceRef: sourceRef, Summary: kind}
	default:
		if err := json.Unmarshal(payload, &record); err != nil {
			return record, err
		}
	}
	return record, nil
}
