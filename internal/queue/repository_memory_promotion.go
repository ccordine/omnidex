package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type MemoryCandidatePromotion struct {
	Candidate model.MemoryCandidate
	Tier      string
	Tags      []string
	Embedding []float64
}

type memoryPromotionAuthority string

const (
	promotionCurrent    memoryPromotionAuthority = "current_generation"
	promotionHistorical memoryPromotionAuthority = "historical_generation"
	promotionGlobal     memoryPromotionAuthority = "global"
)

func (r *Repository) PromoteCurrentMemoryCandidate(
	ctx context.Context,
	request MemoryCandidatePromotion,
) (model.MemoryCandidatePromotionResult, error) {
	return r.promoteMemoryCandidate(ctx, request, promotionCurrent, nil)
}

func (r *Repository) PromoteCurrentMemoryCandidateByStepAttempt(
	ctx context.Context,
	stepAuthority model.StepAttemptAuthority,
	request MemoryCandidatePromotion,
) (model.MemoryCandidatePromotionResult, error) {
	return r.promoteMemoryCandidate(ctx, request, promotionCurrent, &stepAuthority)
}

func (r *Repository) PromoteHistoricalMemoryCandidate(
	ctx context.Context,
	request MemoryCandidatePromotion,
) (model.MemoryCandidatePromotionResult, error) {
	return r.promoteMemoryCandidate(ctx, request, promotionHistorical, nil)
}

func (r *Repository) PromoteGlobalMemoryCandidate(
	ctx context.Context,
	request MemoryCandidatePromotion,
) (model.MemoryCandidatePromotionResult, error) {
	return r.promoteMemoryCandidate(ctx, request, promotionGlobal, nil)
}

func (r *Repository) promoteMemoryCandidate(
	ctx context.Context,
	request MemoryCandidatePromotion,
	authority memoryPromotionAuthority,
	stepAuthority *model.StepAttemptAuthority,
) (model.MemoryCandidatePromotionResult, error) {
	if err := validateMemoryCandidatePromotion(request, authority); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	defer tx.Rollback(ctx)
	if stepAuthority != nil {
		if authority != promotionCurrent || request.Candidate.JobID != stepAuthority.JobID ||
			request.Candidate.Generation == nil || *request.Candidate.Generation != stepAuthority.Generation {
			return model.MemoryCandidatePromotionResult{}, staleStepAttemptError(*stepAuthority, "memory candidate owner disagrees with attempt", nil)
		}
		if jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, *stepAuthority); err != nil {
			return model.MemoryCandidatePromotionResult{}, err
		} else if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
			return model.MemoryCandidatePromotionResult{}, staleStepAttemptError(*stepAuthority, "memory promotion writer is not running", nil)
		}
	}

	if err := lockMemoryPromotionAuthorityTx(ctx, tx, request.Candidate, authority); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	persisted, promotedMemoryID, err := loadMemoryCandidateForPromotionTx(ctx, tx, request.Candidate.ID)
	if err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	if err := validateObservedMemoryCandidate(request.Candidate, persisted); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	if err := validatePersistedMemoryCandidate(persisted); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	if persisted.Status == request.Tier {
		if promotedMemoryID == nil {
			return model.MemoryCandidatePromotionResult{}, fmt.Errorf(
				"memory candidate %d is %q without an authoritative promoted memory", persisted.ID, persisted.Status,
			)
		}
		chunk, err := loadMemoryChunkTx(ctx, tx, *promotedMemoryID)
		if err != nil {
			return model.MemoryCandidatePromotionResult{}, err
		}
		expectedSource := memoryPromotionSource(persisted, request.Tier, authority)
		if chunk.Source != expectedSource || chunk.Kind != persisted.CandidateKind || chunk.Content != persisted.Content {
			return model.MemoryCandidatePromotionResult{}, fmt.Errorf(
				"memory candidate %d promotion authority does not match its bound memory", persisted.ID,
			)
		}
		if err := tx.Commit(ctx); err != nil {
			return model.MemoryCandidatePromotionResult{}, err
		}
		return model.MemoryCandidatePromotionResult{Candidate: persisted, Memory: &chunk}, nil
	}
	if persisted.Status != model.MemoryCandidateStatusCandidate || promotedMemoryID != nil {
		return model.MemoryCandidatePromotionResult{}, fmt.Errorf(
			"memory candidate %d cannot be promoted from status %q", persisted.ID, persisted.Status,
		)
	}

	chunk, err := insertPromotedMemoryChunkTx(ctx, tx, persisted, request, authority)
	if err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE memory_candidates
		SET status=$2, promoted_memory_id=$3, updated_at=NOW()
		WHERE id=$1 AND status=$4 AND promoted_memory_id IS NULL
		RETURNING updated_at
	`, persisted.ID, request.Tier, chunk.ID, model.MemoryCandidateStatusCandidate).Scan(&persisted.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.MemoryCandidatePromotionResult{}, fmt.Errorf(
			"memory candidate %d changed during atomic promotion", persisted.ID,
		)
	}
	if err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	persisted.Status = request.Tier
	if err := tx.Commit(ctx); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	return model.MemoryCandidatePromotionResult{Candidate: persisted, Memory: &chunk}, nil
}

func validateMemoryCandidatePromotion(request MemoryCandidatePromotion, authority memoryPromotionAuthority) error {
	if request.Candidate.ID <= 0 {
		return fmt.Errorf("memory candidate identity must be positive")
	}
	if request.Tier != model.MemoryCandidateStatusApproved && request.Tier != model.MemoryCandidateStatusDurable {
		return fmt.Errorf("memory promotion tier %q is not registered exact text", request.Tier)
	}
	if len(request.Embedding) == 0 {
		return fmt.Errorf("memory promotion requires an already-computed embedding")
	}
	for _, value := range request.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("memory promotion embedding values must be finite")
		}
	}
	switch authority {
	case promotionCurrent, promotionHistorical:
		if request.Candidate.JobID <= 0 || request.Candidate.Generation == nil || *request.Candidate.Generation <= 0 {
			return fmt.Errorf("%s memory promotion requires a job-scoped candidate with observed generation", authority)
		}
	case promotionGlobal:
		if request.Candidate.JobID != 0 || request.Candidate.Generation != nil {
			return fmt.Errorf("global memory promotion requires a global candidate")
		}
	default:
		return fmt.Errorf("memory promotion authority %q is not registered", authority)
	}
	return nil
}

func validateObservedMemoryCandidate(observed, persisted model.MemoryCandidate) error {
	if !sameMemoryCandidateOwner(observed, optionalInt64(persisted.JobID), persisted.Generation) ||
		observed.CandidateKind != persisted.CandidateKind || observed.Content != persisted.Content ||
		!bytes.Equal(observed.Provenance, persisted.Provenance) || observed.Confidence != persisted.Confidence ||
		!sameOptionalInt64(observed.SourceMemoryID, persisted.SourceMemoryID) {
		return fmt.Errorf("memory candidate %d changed after it was observed", observed.ID)
	}
	return nil
}

func validatePersistedMemoryCandidate(candidate model.MemoryCandidate) error {
	validated := candidate
	validated.Status = model.MemoryCandidateStatusCandidate
	if _, err := validateNewMemoryCandidate(validated); err != nil {
		return fmt.Errorf("persisted memory candidate %d is invalid: %w", candidate.ID, err)
	}
	return nil
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func sameOptionalInt64(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func memoryPromotionSource(candidate model.MemoryCandidate, tier string, authority memoryPromotionAuthority) string {
	if authority == promotionGlobal {
		return fmt.Sprintf("global:reviewed:%s", tier)
	}
	return fmt.Sprintf(
		"job:%d:generation:%d:%s:reviewed:%s",
		candidate.JobID, *candidate.Generation, strings.ReplaceAll(string(authority), "_", "-"), tier,
	)
}
