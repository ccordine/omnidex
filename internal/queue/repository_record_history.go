package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func readHistoricalArtifactPage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalArtifact, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT artifact.id, artifact.job_id, artifact.step_id, artifact.kind,
		       artifact.version, artifact.payload_json, artifact.created_at,
		       steps.generation, steps.superseded_at_generation
		FROM artifacts AS artifact
		JOIN job_steps AS steps
		  ON steps.job_id=artifact.job_id AND steps.id=artifact.step_id
		WHERE artifact.job_id=$1 AND artifact.id>$2
		ORDER BY artifact.id ASC
		LIMIT $3
	`, jobID, afterID, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d historical artifacts: %w", jobID, err)
	}
	defer rows.Close()

	items := make([]HistoricalArtifact, 0, limit+1)
	for rows.Next() {
		var item HistoricalArtifact
		var id int64
		var payload []byte
		if err := rows.Scan(
			&id, &item.Artifact.JobID, &item.Artifact.StepID, &item.Artifact.Kind,
			&item.Artifact.Version, &payload, &item.Artifact.CreatedAt,
			&item.Step.Generation, &item.Step.SupersededAtGeneration,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d historical artifact: %w", jobID, err)
		}
		item.Artifact.ID = strconv.FormatInt(id, 10)
		item.Artifact.Payload = append([]byte(nil), payload...)
		item.Step.JobID = item.Artifact.JobID
		item.Step.StepID = item.Artifact.StepID
		item.cursorID = id
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate job %d historical artifacts: %w", jobID, err)
	}
	return finishJobHistoryPage(jobID, JobHistoryArtifacts, items, limit, func(item HistoricalArtifact) int64 {
		return item.cursorID
	})
}

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

func readHistoricalClaimPage(
	ctx context.Context,
	tx pgx.Tx,
	jobID, afterID int64,
	limit int,
) ([]HistoricalClaim, string, error) {
	rows, err := tx.Query(ctx, `
		SELECT claim.id, claim.job_id, claim.step_id, claim.text, claim.normalized_text,
		       claim.status, claim.confidence, claim.created_at,
		       steps.generation, steps.superseded_at_generation
		FROM claims AS claim
		JOIN job_steps AS steps
		  ON steps.job_id=claim.job_id AND steps.id=claim.step_id
		WHERE claim.job_id=$1 AND claim.id>$2
		ORDER BY claim.id ASC
		LIMIT $3
	`, jobID, afterID, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list job %d historical claims: %w", jobID, err)
	}
	defer rows.Close()

	items := make([]HistoricalClaim, 0, limit+1)
	for rows.Next() {
		var item HistoricalClaim
		if err := rows.Scan(
			&item.Claim.ID, &item.Claim.JobID, &item.Claim.StepID, &item.Claim.Text,
			&item.Claim.NormalizedText, &item.Claim.Status, &item.Claim.Confidence,
			&item.Claim.CreatedAt, &item.Step.Generation, &item.Step.SupersededAtGeneration,
		); err != nil {
			return nil, "", fmt.Errorf("scan job %d historical claim: %w", jobID, err)
		}
		item.Step.JobID = item.Claim.JobID
		item.Step.StepID = item.Claim.StepID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate job %d historical claims: %w", jobID, err)
	}
	return finishJobHistoryPage(jobID, JobHistoryClaims, items, limit, func(item HistoricalClaim) int64 {
		return item.Claim.ID
	})
}
