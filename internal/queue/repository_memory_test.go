package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestMemoryCategoriesComeOnlyFromKindAndStructuredCategories(t *testing.T) {
	categories, err := memoryCategoriesFor(
		model.MemoryKind(model.MemoryKindProcedural),
		[]model.MemoryCategory{model.MemoryCategoryProject, model.MemoryCategoryLanguage},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []model.MemoryCategory{
		model.MemoryCategoryStrategy, model.MemoryCategoryProject, model.MemoryCategoryLanguage,
	} {
		if !hasMemoryCategory(categories, want) {
			t.Fatalf("categories missing %q: %#v", want, categories)
		}
	}
	if _, err := memoryCategoriesFor(
		model.MemoryKind(model.MemoryKindReference),
		[]model.MemoryCategory{model.MemoryCategory("postgres")},
	); err == nil {
		t.Fatal("semantic technology alias was accepted as a structured category")
	}
}

func TestMemoryKindHasOneDeterministicBaseCategory(t *testing.T) {
	wants := map[model.MemoryKind]model.MemoryCategory{
		model.MemoryKind(model.MemoryKindEpisodic):    model.MemoryCategoryPersonal,
		model.MemoryKind(model.MemoryKindProcedural):  model.MemoryCategoryStrategy,
		model.MemoryKind(model.MemoryKindInstruction): model.MemoryCategoryInstruction,
		model.MemoryKind(model.MemoryKindPreference):  model.MemoryCategoryPreference,
		model.MemoryKind(model.MemoryKindReference):   model.MemoryCategoryResearch,
	}
	for kind, want := range wants {
		categories, err := memoryCategoriesFor(kind, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(categories) < 1 || categories[0] != want {
			t.Fatalf("kind %q categories=%#v want first %q", kind, categories, want)
		}
	}
}

func TestMemoryBatchValidatesEveryWriteBeforePostgres(t *testing.T) {
	writes := []MemoryChunkWrite{
		{Input: validMemoryInput("first")},
		{Input: model.MemoryInput{
			Scope:  model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"},
			Source: "manual", Kind: model.MemoryKind("unknown"), Content: "second",
		}},
	}
	if _, err := (&Repository{}).AddMemoryChunks(context.Background(), writes); err == nil ||
		!strings.Contains(err.Error(), "not registered exact text") {
		t.Fatalf("batch validation error=%v", err)
	}
}

func TestMemoryRetrievalRejectsImplicitLimitsAndMalformedEmbeddings(t *testing.T) {
	for _, fixture := range []struct {
		embedding []float64
		limit     int
	}{
		{limit: 0}, {limit: 33}, {embedding: []float64{1}, limit: 8},
	} {
		if _, err := (&Repository{}).FindRelevantMemory(
			context.Background(), model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"},
			fixture.embedding, fixture.limit,
		); err == nil {
			t.Fatalf("invalid retrieval fixture %+v was accepted", fixture)
		}
	}
}

func TestMemoryWriteSourceHasNoDistanceOverwriteOrSemanticSourceParsing(t *testing.T) {
	raw, err := os.ReadFile("repository_memory.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"inferredMemoryCorrectionDistance", "UPDATE memory_chunks",
		"normalizeMemoryKind", "memoryKindAllowsSemanticCorrection",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("memory write source contains forbidden implicit authority %q", forbidden)
		}
	}
}

func TestMemoryVectorLiteralPreservesExactFloatIdentity(t *testing.T) {
	want := "[0.12345678901234568]"
	if got := vectorLiteral([]float64{0.12345678901234568}); got != want {
		t.Fatalf("vector literal=%q want=%q", got, want)
	}
}

func validMemoryInput(content string) model.MemoryInput {
	return model.MemoryInput{
		Scope:  model.MemoryScope{ProjectID: 1, ChannelID: "channel-one"},
		Source: "manual", Kind: model.MemoryKind(model.MemoryKindReference), Content: content,
		Tags: []string{"scope:user"}, Categories: []model.MemoryCategory{model.MemoryCategoryResearch},
	}
}

func hasMemoryCategory(values []model.MemoryCategory, want model.MemoryCategory) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
