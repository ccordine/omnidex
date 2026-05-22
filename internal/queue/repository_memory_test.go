package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestMemoryKindAllowsSemanticCorrectionExcludesReferenceChunks(t *testing.T) {
	for _, kind := range []string{model.MemoryKindReference, model.MemoryKindEpisodic} {
		if memoryKindAllowsSemanticCorrection(kind) {
			t.Fatalf("kind %q should not use semantic correction", kind)
		}
	}
	for _, kind := range []string{model.MemoryKindProcedural, model.MemoryKindInstruction, model.MemoryKindPreference} {
		if !memoryKindAllowsSemanticCorrection(kind) {
			t.Fatalf("kind %q should use semantic correction", kind)
		}
	}
}

func TestInferMemoryCategoriesFromKindTagsAndContent(t *testing.T) {
	categories := inferMemoryCategories(
		model.MemoryKindProcedural,
		"Successful Go and React project strategy verified with docker compose and PostgreSQL migrations.",
		[]string{"project:omni-nxt-f54144e2", "react", "docker", "category:custom-skill"},
	)

	for _, want := range []string{"strategy", "project", "language", "frontend", "infrastructure", "database", "verification", "custom-skill"} {
		if !hasString(categories, want) {
			t.Fatalf("categories missing %q: %#v", want, categories)
		}
	}
}

func TestMemoryCategoryTagsAndFilters(t *testing.T) {
	categories := []string{"personal", "project", "database"}
	tags := memoryCategoryTags(categories)
	for _, want := range []string{"category:personal", "category:project", "category:database"} {
		if !hasString(tags, want) {
			t.Fatalf("category tags missing %q: %#v", want, tags)
		}
	}

	filters := memoryCategoryFilters([]string{"react", "category:project", "category:pgsql"})
	for _, want := range []string{"project", "database"} {
		if !hasString(filters, want) {
			t.Fatalf("category filters missing %q: %#v", want, filters)
		}
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
