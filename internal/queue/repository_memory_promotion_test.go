package queue

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCurrentMemoryPromotionRequiresJobGenerationAuthority(t *testing.T) {
	candidate := model.MemoryCandidate{ID: 7, CandidateKind: model.MemoryKindReference, Content: "bounded"}
	request := MemoryCandidatePromotion{
		Candidate: candidate,
		Tier:      model.MemoryCandidateStatusDurable,
		Embedding: []float64{0.25},
	}
	if _, err := (&Repository{}).PromoteCurrentMemoryCandidate(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "job-scoped") {
		t.Fatalf("global current-generation promotion error=%v", err)
	}

	generation := int64(1)
	request.Candidate.JobID = 9
	request.Candidate.Generation = &generation
	request.Embedding = make([]float64, model.MemoryEmbeddingDimensions)
	request.Embedding[0] = math.NaN()
	if _, err := (&Repository{}).PromoteCurrentMemoryCandidate(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "finite") {
		t.Fatalf("invalid embedding error=%v", err)
	}
}

func TestMemoryPromotionAuthoritiesAreNotInterchangeable(t *testing.T) {
	generation := int64(1)
	jobCandidate := model.MemoryCandidate{
		ID: 11, JobID: 3, Generation: &generation,
		CandidateKind: model.MemoryKindReference, Content: "job fact",
	}
	globalCandidate := model.MemoryCandidate{
		ID: 12, CandidateKind: model.MemoryKindReference, Content: "global fact",
	}
	base := MemoryCandidatePromotion{
		Tier:      model.MemoryCandidateStatusApproved,
		Embedding: make([]float64, model.MemoryEmbeddingDimensions),
	}

	base.Candidate = globalCandidate
	if _, err := (&Repository{}).PromoteHistoricalMemoryCandidate(context.Background(), base); err == nil ||
		!strings.Contains(err.Error(), "job-scoped") {
		t.Fatalf("historical global promotion error=%v", err)
	}
	base.Candidate = jobCandidate
	if _, err := (&Repository{}).PromoteGlobalMemoryCandidate(context.Background(), base); err == nil ||
		!strings.Contains(err.Error(), "global") {
		t.Fatalf("global job promotion error=%v", err)
	}
}
