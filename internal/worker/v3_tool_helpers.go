package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func diversifyMemoryMatchesBySourceURL(matches []model.MemoryMatch, limit int) []model.MemoryMatch {
	if len(matches) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	selected := make([]model.MemoryMatch, 0, limit)
	selectedIDs := map[int64]struct{}{}
	seenURLs := map[string]struct{}{}
	for _, match := range matches {
		url := memoryMatchSourceURL(match.Content)
		if url == "" {
			continue
		}
		if _, ok := seenURLs[url]; ok {
			continue
		}
		selected = append(selected, match)
		selectedIDs[match.ID] = struct{}{}
		seenURLs[url] = struct{}{}
		if len(selected) >= limit {
			return selected
		}
	}
	for _, match := range matches {
		if _, ok := selectedIDs[match.ID]; ok {
			continue
		}
		selected = append(selected, match)
		if len(selected) >= limit {
			break
		}
	}
	return selected
}

func memoryMatchSourceURL(content string) string {
	for _, line := range strings.Split(content, "\n") {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(clean), "source_url=") {
			return strings.TrimSpace(clean[len("source_url="):])
		}
	}
	return ""
}

func memoryRetrievalEvidenceRecords(query string, scopeTags []string, ranked []model.MemoryMatch) []evidence.Record {
	if len(ranked) == 0 {
		return []evidence.Record{{
			Kind:       evidence.KindModelJudgment,
			SourceType: "memory",
			SourceRef:  "memory.retrieve:no_matches",
			Summary:    "Memory retrieval completed with no relevant matches.",
			Confidence: 1,
			Metadata: map[string]any{
				"query":      trimForBudget(query, 500),
				"scope_tags": append([]string(nil), scopeTags...),
				"matches":    0,
			},
		}}
	}
	records := make([]evidence.Record, 0, minInt(len(ranked), 8))
	for index, match := range ranked {
		if index >= 8 {
			break
		}
		records = append(records, evidence.Record{
			Kind:       evidence.KindMemoryExcerpt,
			SourceType: "memory",
			SourceRef:  fmt.Sprintf("memory:%d", match.ID),
			Excerpt:    trimForBudget(match.Content, 800),
			Summary:    match.Kind,
			Confidence: match.Score,
			Metadata:   map[string]any{"tags": match.Tags, "categories": match.Categories},
		})
	}
	return records
}

func mustRegisterTool(registry *toolruntime.Registry, spec toolruntime.Spec, handler toolruntime.Handler) {
	if err := registry.Register(spec, handler); err != nil {
		panic(err)
	}
}

func toolInputString(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func toolInputInt(input map[string]any, key string, fallback int) int {
	if input == nil {
		return fallback
	}
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func toolWarnings(add bool, warning string) []string {
	if !add || strings.TrimSpace(warning) == "" {
		return nil
	}
	return []string{strings.TrimSpace(warning)}
}

func memoryFacetsForTool(facets []model.MemoryFacet) []map[string]any {
	out := make([]map[string]any, 0, len(facets))
	for _, facet := range facets {
		out = append(out, map[string]any{"name": facet.Name, "count": facet.Count})
	}
	return out
}

func evidenceRecordMap(record evidence.Record) map[string]any {
	return map[string]any{
		"id":              record.ID,
		"kind":            record.Kind,
		"source_type":     record.SourceType,
		"source_ref":      record.SourceRef,
		"tool_name":       record.ToolName,
		"specialist_id":   metadataText(record.Metadata, "specialist_id"),
		"subtask_id":      metadataText(record.Metadata, "subtask_id"),
		"objective_id":    metadataText(record.Metadata, "subtask_objective_id"),
		"summary":         record.Summary,
		"excerpt":         record.Excerpt,
		"command":         record.Command,
		"file_paths":      append([]string(nil), record.FilePaths...),
		"confidence":      record.Confidence,
		"supports_claims": append([]string(nil), record.SupportsClaims...),
		"warnings":        append([]string(nil), record.Warnings...),
	}
}
