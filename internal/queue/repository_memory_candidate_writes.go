package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxMemoryCandidateContentBytes    = 1 << 20
	maxMemoryCandidateProvenanceBytes = 64 << 10
	maxMemoryCandidateBatchItems      = 512
)

func (r *Repository) WriteMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) (int64, error) {
	ids, err := r.WriteMemoryCandidates(ctx, []model.MemoryCandidate{candidate})
	if err != nil {
		return 0, err
	}
	return ids[0], nil
}

func (r *Repository) WriteMemoryCandidates(
	ctx context.Context,
	candidates []model.MemoryCandidate,
) ([]int64, error) {
	if len(candidates) == 0 || len(candidates) > maxMemoryCandidateBatchItems {
		return nil, fmt.Errorf(
			"memory candidate batch must contain 1..%d candidates",
			maxMemoryCandidateBatchItems,
		)
	}
	provenances := make([]string, len(candidates))
	for index, candidate := range candidates {
		provenance, err := validateNewMemoryCandidate(candidate)
		if err != nil {
			return nil, fmt.Errorf("memory candidate batch item %d: %w", index+1, err)
		}
		provenances[index] = provenance
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("memory candidate writes require PostgreSQL")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	owners := make(map[int64]int64)
	for _, candidate := range candidates {
		if candidate.JobID == 0 {
			continue
		}
		if observed, exists := owners[candidate.JobID]; exists && observed != *candidate.Generation {
			return nil, fmt.Errorf("memory candidate batch has contradictory job generation authority")
		}
		owners[candidate.JobID] = *candidate.Generation
	}
	jobIDs := make([]int64, 0, len(owners))
	for jobID := range owners {
		jobIDs = append(jobIDs, jobID)
	}
	sort.Slice(jobIDs, func(left, right int) bool { return jobIDs[left] < jobIDs[right] })
	for _, jobID := range jobIDs {
		if err := lockObservedJobGenerationTx(ctx, tx, jobID, owners[jobID]); err != nil {
			return nil, err
		}
	}
	ids := make([]int64, 0, len(candidates))
	for index, candidate := range candidates {
		var jobID, generation any
		if candidate.JobID > 0 {
			jobID = candidate.JobID
			generation = *candidate.Generation
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO memory_candidates (
				project_id, channel_id, job_id, generation, source_memory_id,
				candidate_kind, content, provenance, confidence, status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
			RETURNING id
		`, candidate.Scope.ProjectID, candidate.Scope.ChannelID, jobID, generation,
			candidate.SourceMemoryID, candidate.CandidateKind,
			candidate.Content, provenances[index], candidate.Confidence,
			model.MemoryCandidateStatusCandidate).Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) RejectCurrentMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) error {
	return r.rejectCurrentMemoryCandidate(ctx, nil, candidate)
}

func (r *Repository) RejectCurrentMemoryCandidateByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	candidate model.MemoryCandidate,
) error {
	return r.rejectCurrentMemoryCandidate(ctx, &authority, candidate)
}

func (r *Repository) rejectCurrentMemoryCandidate(
	ctx context.Context,
	authority *model.StepAttemptAuthority,
	candidate model.MemoryCandidate,
) error {
	if candidate.ID <= 0 {
		return fmt.Errorf("memory candidate identity must be positive")
	}
	if err := candidate.Scope.Validate(); err != nil {
		return err
	}
	if candidate.JobID <= 0 || candidate.Generation == nil || *candidate.Generation <= 0 {
		return fmt.Errorf("current memory candidate rejection requires a job-scoped candidate with observed generation")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if authority != nil {
		if candidate.JobID != authority.JobID || *candidate.Generation != authority.Generation {
			return staleStepAttemptError(*authority, "memory candidate owner disagrees with attempt", nil)
		}
		if jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, *authority); err != nil {
			return err
		} else if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
			return staleStepAttemptError(*authority, "memory rejection writer is not running", nil)
		}
	}
	if err := lockObservedJobGenerationTx(ctx, tx, candidate.JobID, *candidate.Generation); err != nil {
		return err
	}
	var persistedScope model.MemoryScope
	var persistedJobID, persistedGeneration *int64
	var persistedStatus string
	if err := tx.QueryRow(ctx, `
		SELECT project_id, channel_id, job_id, generation, status
		FROM memory_candidates
		WHERE id=$1
		FOR UPDATE
	`, candidate.ID).Scan(
		&persistedScope.ProjectID, &persistedScope.ChannelID,
		&persistedJobID, &persistedGeneration, &persistedStatus,
	); err != nil {
		return err
	}
	if persistedScope != candidate.Scope ||
		!sameMemoryCandidateOwner(candidate, persistedJobID, persistedGeneration) {
		return fmt.Errorf("memory candidate %d owner changed before review", candidate.ID)
	}
	if persistedStatus == model.MemoryCandidateStatusRejected {
		return tx.Commit(ctx)
	}
	if persistedStatus != model.MemoryCandidateStatusCandidate {
		return fmt.Errorf("memory candidate %d is already %q", candidate.ID, persistedStatus)
	}
	result, err := tx.Exec(ctx, `
		UPDATE memory_candidates
		SET status=$2, updated_at=NOW()
		WHERE id=$1 AND status=$3
	`, candidate.ID, model.MemoryCandidateStatusRejected, model.MemoryCandidateStatusCandidate)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("memory candidate %d status changed concurrently", candidate.ID)
	}
	return tx.Commit(ctx)
}

func lockObservedJobGenerationTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	var currentGeneration int64
	if err := tx.QueryRow(ctx, `
		SELECT current_generation FROM jobs WHERE id=$1 FOR UPDATE
	`, jobID).Scan(&currentGeneration); err != nil {
		return err
	}
	if currentGeneration != generation {
		return fmt.Errorf(
			"%w: observed job %d generation %d, current generation is %d",
			ErrStaleJobGeneration, jobID, generation, currentGeneration,
		)
	}
	return nil
}

func validateNewMemoryCandidate(candidate model.MemoryCandidate) (string, error) {
	if err := candidate.Scope.Validate(); err != nil {
		return "", err
	}
	if candidate.JobID < 0 {
		return "", fmt.Errorf("memory candidate job identity must not be negative")
	}
	if candidate.JobID == 0 && candidate.Generation != nil {
		return "", fmt.Errorf("global memory candidate must not declare a job generation")
	}
	if candidate.JobID > 0 && (candidate.Generation == nil || *candidate.Generation <= 0) {
		return "", fmt.Errorf("job memory candidate requires its observed positive generation")
	}
	if _, err := model.ParseMemoryKind(string(candidate.CandidateKind)); err != nil {
		return "", err
	}
	if candidate.Content == "" || candidate.Content != strings.TrimSpace(candidate.Content) ||
		!utf8.ValidString(candidate.Content) || strings.ContainsRune(candidate.Content, '\x00') ||
		len(candidate.Content) > maxMemoryCandidateContentBytes {
		return "", fmt.Errorf("memory candidate content must be exact PostgreSQL-compatible text of at most %d bytes", maxMemoryCandidateContentBytes)
	}
	if math.IsNaN(candidate.Confidence) || math.IsInf(candidate.Confidence, 0) ||
		candidate.Confidence < 0 || candidate.Confidence > 1 {
		return "", fmt.Errorf("memory candidate confidence must be between zero and one")
	}
	if candidate.Status != "" && candidate.Status != model.MemoryCandidateStatusCandidate {
		return "", fmt.Errorf("new memory candidate status must be %q", model.MemoryCandidateStatusCandidate)
	}
	provenance := strings.TrimSpace(string(candidate.Provenance))
	if provenance == "" {
		provenance = `{}`
	}
	if len(provenance) > maxMemoryCandidateProvenanceBytes || !json.Valid([]byte(provenance)) {
		return "", fmt.Errorf("memory candidate provenance must be valid JSON of at most %d bytes", maxMemoryCandidateProvenanceBytes)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(provenance), &object); err != nil || object == nil {
		return "", fmt.Errorf("memory candidate provenance must be a JSON object")
	}
	return provenance, nil
}

func sameMemoryCandidateOwner(candidate model.MemoryCandidate, jobID, generation *int64) bool {
	if candidate.JobID == 0 {
		return jobID == nil && generation == nil
	}
	return jobID != nil && generation != nil && candidate.Generation != nil &&
		*jobID == candidate.JobID && *generation == *candidate.Generation
}
