package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/jackc/pgx/v5"
)

type WebEvidenceRecord struct {
	ID       int64
	JobID    int64
	Acquired webresearch.AcquiredEvidence
}

// RecordWebEvidence retains the actual query, discovery and fetch reports,
// including full bounded source text. Equal observations remain separate reads.
func (r *Repository) RecordWebEvidence(ctx context.Context, jobID int64, acquired webresearch.AcquiredEvidence) (WebEvidenceRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID < 1 {
		return WebEvidenceRecord{}, fmt.Errorf("record web evidence requires PostgreSQL, context, and a positive job ID")
	}
	if _, _, err := acquired.Project(); err != nil {
		return WebEvidenceRecord{}, err
	}
	payload, err := json.Marshal(acquired)
	if err != nil {
		return WebEvidenceRecord{}, fmt.Errorf("encode acquired web evidence: %w", err)
	}
	var record WebEvidenceRecord
	err = r.pool.QueryRow(ctx, `
		INSERT INTO web_evidence (job_id, acquisition)
		VALUES ($1,$2::jsonb) RETURNING id,job_id
	`, jobID, payload).Scan(&record.ID, &record.JobID)
	if err != nil {
		return WebEvidenceRecord{}, fmt.Errorf("record acquired web evidence for job %d: %w", jobID, err)
	}
	if err := json.Unmarshal(payload, &record.Acquired); err != nil {
		return WebEvidenceRecord{}, fmt.Errorf("decode recorded web evidence: %w", err)
	}
	return record, nil
}

func (r *Repository) GetWebEvidence(ctx context.Context, jobID, id int64) (WebEvidenceRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID < 1 || id < 1 {
		return WebEvidenceRecord{}, fmt.Errorf("read web evidence requires PostgreSQL, context, and exact job and record IDs")
	}
	return readWebEvidence(ctx, r.pool, jobID, id)
}

func readWebEvidence(ctx context.Context, reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, jobID, id int64) (WebEvidenceRecord, error) {
	var record WebEvidenceRecord
	var payload []byte
	err := reader.QueryRow(ctx, `
		SELECT id,job_id,acquisition FROM web_evidence WHERE job_id=$1 AND id=$2
	`, jobID, id).Scan(&record.ID, &record.JobID, &payload)
	if err != nil {
		return WebEvidenceRecord{}, fmt.Errorf("read web evidence %d for job %d: %w", id, jobID, err)
	}
	if err := exactjson.ValidateObject(payload, &record.Acquired, "recorded web acquisition"); err != nil {
		return WebEvidenceRecord{}, err
	}
	if err := json.Unmarshal(payload, &record.Acquired); err != nil {
		return WebEvidenceRecord{}, fmt.Errorf("decode acquired web evidence: %w", err)
	}
	return record, nil
}
