package queue

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/specialists"
)

func TestActiveWorkerSkillRequiresExactlyOneFrozenEmbedding(t *testing.T) {
	t.Parallel()

	version := specialists.SkillVersion{
		Spec:    specialists.Spec{ID: "learned_0123456789abcdef0123456789abcdef"},
		Version: 1, Status: specialists.SkillStatusActive,
	}
	if err := requireFrozenActiveSkillEmbedding(version, 1); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int64{0, 2} {
		if err := requireFrozenActiveSkillEmbedding(version, count); err == nil ||
			!strings.Contains(err.Error(), "exactly one frozen identity") {
			t.Fatalf("embedding count %d error=%v", count, err)
		}
	}
}

func TestWorkerSkillMatchingRejectsUnboundedOrInvalidEmbeddings(t *testing.T) {
	t.Parallel()

	repository := &Repository{}
	if _, err := repository.FindActiveWorkerSkillMatches(
		context.Background(), "ollama", "embed", []float64{0.1}, maxWorkerSkillMatches+1,
	); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("FindActiveWorkerSkillMatches() error=%v, want hard limit", err)
	}
	if _, _, _, err := validatedWorkerSkillQuery(
		"ollama", "embed", []float64{0.1, math.NaN()},
	); err == nil || !strings.Contains(err.Error(), "non-finite") {
		t.Fatalf("validatedWorkerSkillQuery() error=%v, want finite-value failure", err)
	}
}

func TestActiveWorkerSkillLookupRejectsMissingEmbeddingIdentity(t *testing.T) {
	t.Parallel()

	repository := &Repository{}
	if _, err := repository.HasActiveWorkerSkillEmbeddings(
		context.Background(), "", "embed",
	); err == nil || !strings.Contains(err.Error(), "provider and model") {
		t.Fatalf("HasActiveWorkerSkillEmbeddings() error=%v", err)
	}
}
