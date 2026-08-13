package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockMemoryPromotionAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	candidate model.MemoryCandidate,
	authority memoryPromotionAuthority,
) error {
	switch authority {
	case promotionCurrent:
		return lockObservedJobGenerationTx(ctx, tx, candidate.JobID, *candidate.Generation)
	case promotionHistorical:
		var currentGeneration int64
		if err := tx.QueryRow(ctx, `
			SELECT current_generation FROM jobs WHERE id=$1 FOR UPDATE
		`, candidate.JobID).Scan(&currentGeneration); err != nil {
			return err
		}
		if *candidate.Generation >= currentGeneration {
			return fmt.Errorf(
				"historical memory promotion requires a retired generation; observed %d, current %d",
				*candidate.Generation, currentGeneration,
			)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM job_generations WHERE job_id=$1 AND generation=$2
			)
		`, candidate.JobID, *candidate.Generation).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("historical memory promotion generation does not exist")
		}
		return nil
	case promotionGlobal:
		return nil
	default:
		return fmt.Errorf("memory promotion authority %q is not registered", authority)
	}
}

func loadMemoryCandidateForPromotionTx(
	ctx context.Context,
	tx pgx.Tx,
	candidateID int64,
) (model.MemoryCandidate, *int64, error) {
	var candidate model.MemoryCandidate
	var jobID, promotedMemoryID *int64
	err := tx.QueryRow(ctx, `
		SELECT id, project_id, channel_id, job_id, generation, source_memory_id, candidate_kind, content,
		       provenance, confidence, status, promoted_memory_id, created_at, updated_at
		FROM memory_candidates
		WHERE id=$1
		FOR UPDATE
	`, candidateID).Scan(
		&candidate.ID, &candidate.Scope.ProjectID, &candidate.Scope.ChannelID,
		&jobID, &candidate.Generation, &candidate.SourceMemoryID,
		&candidate.CandidateKind, &candidate.Content, &candidate.Provenance,
		&candidate.Confidence, &candidate.Status, &promotedMemoryID,
		&candidate.CreatedAt, &candidate.UpdatedAt,
	)
	if jobID != nil {
		candidate.JobID = *jobID
	}
	return candidate, promotedMemoryID, err
}

func insertPromotedMemoryChunkTx(
	ctx context.Context,
	tx pgx.Tx,
	candidate model.MemoryCandidate,
	request MemoryCandidatePromotion,
	authority memoryPromotionAuthority,
) (model.MemoryChunk, error) {
	source := memoryPromotionSource(candidate, request.Tier, authority)
	var chunk model.MemoryChunk
	err := tx.QueryRow(ctx, `
		INSERT INTO memory_chunks (project_id, channel_id, source, kind, content, embedding)
		VALUES ($1, $2, $3, $4, $5, $6::vector)
		RETURNING id, project_id, channel_id, source, kind, content, created_at
	`, candidate.Scope.ProjectID, candidate.Scope.ChannelID, source, candidate.CandidateKind,
		candidate.Content, vectorLiteral(request.Embedding)).Scan(
		&chunk.ID, &chunk.Scope.ProjectID, &chunk.Scope.ChannelID,
		&chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt,
	)
	if err != nil {
		return model.MemoryChunk{}, err
	}
	tags := append([]string(nil), request.Tags...)
	tags = append(tags, string(candidate.CandidateKind), "reviewed", "promotion:"+string(authority), "provenance:reviewed")
	if request.Tier == model.MemoryCandidateStatusDurable {
		tags = append(tags, model.MemoryTrustTagDurable)
	} else {
		tags = append(tags, model.MemoryTrustTagApproved)
	}
	if candidate.Generation != nil {
		tags = append(tags, fmt.Sprintf("generation:%d", *candidate.Generation))
	}
	if err := attachMemoryTaxonomyTx(
		ctx, tx, chunk.ID, candidate.CandidateKind, tags, request.Categories,
	); err != nil {
		return model.MemoryChunk{}, err
	}
	return chunk, nil
}

func loadMemoryChunkTx(ctx context.Context, tx pgx.Tx, memoryID int64) (model.MemoryChunk, error) {
	var chunk model.MemoryChunk
	err := tx.QueryRow(ctx, `
		SELECT id, project_id, channel_id, source, kind, content, created_at
		FROM memory_chunks
		WHERE id=$1
	`, memoryID).Scan(&chunk.ID, &chunk.Scope.ProjectID, &chunk.Scope.ChannelID,
		&chunk.Source, &chunk.Kind, &chunk.Content, &chunk.CreatedAt)
	return chunk, err
}
