package queue

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func memoryCategoriesFor(
	kind model.MemoryKind,
	explicit []model.MemoryCategory,
) ([]model.MemoryCategory, error) {
	if _, err := model.ParseMemoryKind(string(kind)); err != nil {
		return nil, err
	}
	if err := model.ValidateMemoryCategories(explicit); err != nil {
		return nil, err
	}
	base := model.MemoryCategoryGeneral
	switch kind {
	case model.MemoryKindEpisodic:
		base = model.MemoryCategoryPersonal
	case model.MemoryKindProcedural:
		base = model.MemoryCategoryStrategy
	case model.MemoryKindInstruction:
		base = model.MemoryCategoryInstruction
	case model.MemoryKindPreference:
		base = model.MemoryCategoryPreference
	case model.MemoryKindReference:
		base = model.MemoryCategoryResearch
	}
	out := []model.MemoryCategory{base}
	if kind == model.MemoryKindPreference {
		out = append(out, model.MemoryCategoryPersonal)
	}
	seen := make(map[model.MemoryCategory]struct{}, len(out)+len(explicit))
	for _, category := range out {
		seen[category] = struct{}{}
	}
	for _, category := range explicit {
		if _, exists := seen[category]; exists {
			continue
		}
		seen[category] = struct{}{}
		out = append(out, category)
	}
	return out, nil
}

// cleanTags is used only by read filters. Durable writes use
// model.ValidateMemoryInputTags and preserve their exact caller-provided values.
func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}
