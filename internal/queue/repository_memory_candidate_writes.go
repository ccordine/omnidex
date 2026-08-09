package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxMemoryCandidateContentBytes    = 1 << 20
	maxMemoryCandidateProvenanceBytes = 64 << 10
)

func (r *Repository) WriteMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) (int64, error) {
	provenance, err := validateNewMemoryCandidate(candidate)
	if err != nil {
		return 0, err
	}
	if candidate.JobID == 0 {
		var id int64
		err := r.pool.QueryRow(ctx, `
			INSERT INTO memory_candidates (
				job_id, generation, source_memory_id, candidate_kind, content,
				provenance, confidence, status
			) VALUES (NULL, NULL, $1, $2, $3, $4::jsonb, $5, $6)
			RETURNING id
		`, candidate.SourceMemoryID, candidate.CandidateKind, candidate.Content,
			provenance, candidate.Confidence, model.MemoryCandidateStatusCandidate).Scan(&id)
		return id, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := lockObservedJobGenerationTx(ctx, tx, candidate.JobID, *candidate.Generation); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO memory_candidates (
			job_id, generation, source_memory_id, candidate_kind, content,
			provenance, confidence, status
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		RETURNING id
	`, candidate.JobID, *candidate.Generation, candidate.SourceMemoryID, candidate.CandidateKind,
		candidate.Content, provenance, candidate.Confidence, model.MemoryCandidateStatusCandidate).Scan(&id); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) RejectCurrentMemoryCandidate(ctx context.Context, candidate model.MemoryCandidate) error {
	if candidate.ID <= 0 {
		return fmt.Errorf("memory candidate identity must be positive")
	}
	if candidate.JobID <= 0 || candidate.Generation == nil || *candidate.Generation <= 0 {
		return fmt.Errorf("current memory candidate rejection requires a job-scoped candidate with observed generation")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockObservedJobGenerationTx(ctx, tx, candidate.JobID, *candidate.Generation); err != nil {
		return err
	}
	var persistedJobID, persistedGeneration *int64
	var persistedStatus string
	if err := tx.QueryRow(ctx, `
		SELECT job_id, generation, status
		FROM memory_candidates
		WHERE id=$1
		FOR UPDATE
	`, candidate.ID).Scan(&persistedJobID, &persistedGeneration, &persistedStatus); err != nil {
		return err
	}
	if !sameMemoryCandidateOwner(candidate, persistedJobID, persistedGeneration) {
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
	if candidate.JobID < 0 {
		return "", fmt.Errorf("memory candidate job identity must not be negative")
	}
	if candidate.JobID == 0 && candidate.Generation != nil {
		return "", fmt.Errorf("global memory candidate must not declare a job generation")
	}
	if candidate.JobID > 0 && (candidate.Generation == nil || *candidate.Generation <= 0) {
		return "", fmt.Errorf("job memory candidate requires its observed positive generation")
	}
	if candidate.CandidateKind != strings.TrimSpace(candidate.CandidateKind) || !registeredMemoryKind(candidate.CandidateKind) {
		return "", fmt.Errorf("memory candidate kind %q is not registered exact text", candidate.CandidateKind)
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

func registeredMemoryKind(kind string) bool {
	switch kind {
	case model.MemoryKindEpisodic, model.MemoryKindProcedural, model.MemoryKindInstruction,
		model.MemoryKindPreference, model.MemoryKindReference:
		return true
	default:
		return false
	}
}

func sameMemoryCandidateOwner(candidate model.MemoryCandidate, jobID, generation *int64) bool {
	if candidate.JobID == 0 {
		return jobID == nil && generation == nil
	}
	return jobID != nil && generation != nil && candidate.Generation != nil &&
		*jobID == candidate.JobID && *generation == *candidate.Generation
}
