package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListHistoricalMemoryCandidates(ctx context.Context, jobID int64, status string, limit int) ([]model.MemoryCandidate, error) {
	return r.listMemoryCandidates(ctx, jobID, status, limit, false)
}

func (r *Repository) ListCurrentMemoryCandidates(ctx context.Context, jobID int64, status string, limit int) ([]model.MemoryCandidate, error) {
	if jobID <= 0 {
		return nil, fmt.Errorf("current memory candidates require a positive job ID")
	}
	return r.listMemoryCandidates(ctx, jobID, status, limit, true)
}

func (r *Repository) listMemoryCandidates(ctx context.Context, jobID int64, status string, limit int, current bool) ([]model.MemoryCandidate, error) {
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("memory candidate limit must be between 1 and 500")
	}
	query := `
		SELECT candidates.id, candidates.project_id, candidates.channel_id,
		       candidates.job_id, candidates.generation,
		       candidates.source_memory_id, candidates.candidate_kind, candidates.content,
		       candidates.provenance, candidates.confidence, candidates.status,
		       candidates.created_at, candidates.updated_at
		FROM memory_candidates AS candidates`
	if current {
		query += ` JOIN jobs ON jobs.id=candidates.job_id`
	}
	query += ` WHERE ($1=0 OR candidates.job_id=$1)
		AND ($2='' OR candidates.status=$2)`
	if current {
		query += ` AND candidates.generation=jobs.current_generation`
	}
	query += ` ORDER BY candidates.confidence DESC, candidates.id ASC LIMIT $3`
	rows, err := r.pool.Query(ctx, query, jobID, strings.TrimSpace(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.MemoryCandidate, 0, limit)
	for rows.Next() {
		var item model.MemoryCandidate
		var jobIDValue *int64
		if err := rows.Scan(&item.ID, &item.Scope.ProjectID, &item.Scope.ChannelID,
			&jobIDValue, &item.Generation, &item.SourceMemoryID,
			&item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence,
			&item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if jobIDValue != nil {
			item.JobID = *jobIDValue
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetMemoryCandidate(ctx context.Context, id int64) (model.MemoryCandidate, error) {
	if id <= 0 {
		return model.MemoryCandidate{}, pgx.ErrNoRows
	}
	var item model.MemoryCandidate
	var jobID *int64
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, channel_id, job_id, generation, source_memory_id, candidate_kind, content,
		       provenance, confidence, status, created_at, updated_at
		FROM memory_candidates
		WHERE id = $1
	`, id).Scan(&item.ID, &item.Scope.ProjectID, &item.Scope.ChannelID,
		&jobID, &item.Generation, &item.SourceMemoryID,
		&item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence,
		&item.Status, &item.CreatedAt, &item.UpdatedAt)
	if jobID != nil {
		item.JobID = *jobID
	}
	return item, err
}

func (r *Repository) DeleteMemoryChunk(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid memory id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_chunks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteMemoryCandidate(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid candidate id")
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM memory_candidates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) MindStats(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	queries := map[string]string{
		"memory_chunks":     `SELECT COUNT(*) FROM memory_chunks`,
		"memory_candidates": `SELECT COUNT(*) FROM memory_candidates`,
		"candidate_pending": `SELECT COUNT(*) FROM memory_candidates WHERE status = 'candidate'`,
		"jobs":              `SELECT COUNT(*) FROM jobs`,
	}
	for key, query := range queries {
		var count int64
		if err := r.pool.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, nil
}
