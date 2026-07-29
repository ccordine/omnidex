package queue

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func inferMemoryCategories(kind string, tags []string) []string {
	out := []string{}
	add := func(category string) {
		category = normalizeMemoryCategory(category)
		if category == "" {
			return
		}
		for _, existing := range out {
			if existing == category {
				return
			}
		}
		out = append(out, category)
	}

	switch normalizeMemoryKind(kind) {
	case model.MemoryKindProcedural:
		add("strategy")
	case model.MemoryKindReference:
		add("reference")
	case model.MemoryKindPreference:
		add("preference")
		add("personal")
	case model.MemoryKindInstruction:
		add("instruction")
	}

	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			add(strings.TrimPrefix(tag, "category:"))
			continue
		}
		switch {
		case strings.HasPrefix(tag, "project:"):
			add("project")
		case strings.HasPrefix(tag, "session:"), strings.HasPrefix(tag, "channel:"):
			add("personal")
		case strings.HasPrefix(tag, "provider:"):
			add("integration")
		case strings.HasPrefix(tag, "query:"), tag == "research", tag == "web_search":
			add("research")
		case tag == "success-playbook", tag == "learned-skill":
			add("strategy")
		}
	}
	if len(out) == 0 {
		add("general")
	}
	return out
}

func memoryCategoryTags(categories []string) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		if normalized := normalizeMemoryCategory(category); normalized != "" {
			out = append(out, "category:"+normalized)
		}
	}
	return out
}

func memoryCategoryFilters(tags []string) []string {
	out := []string{}
	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			if category := normalizeMemoryCategory(strings.TrimPrefix(tag, "category:")); category != "" {
				out = appendCleanTags(out, category)
			}
		}
	}
	return out
}

func normalizeMemoryCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.TrimPrefix(category, "category:")
	category = strings.ReplaceAll(category, "_", "-")
	category = strings.ReplaceAll(category, " ", "-")
	switch category {
	case "personal", "person", "user":
		return "personal"
	case "project", "codebase", "workspace", "repo", "repository":
		return "project"
	case "language", "languages", "programming-language":
		return "language"
	case "database", "db", "sql", "pgsql", "postgres", "postgresql":
		return "database"
	case "infrastructure", "infra", "docker", "container", "deployment", "devops":
		return "infrastructure"
	case "frontend", "ui", "react", "vite":
		return "frontend"
	case "integration", "api", "provider", "model-provider":
		return "integration"
	case "strategy", "procedural", "playbook", "skill":
		return "strategy"
	case "reference", "research", "documentation", "docs":
		return "research"
	case "preference", "instruction", "verification", "troubleshooting", "security", "general":
		return category
	default:
		if category == "" || len(category) > 40 {
			return ""
		}
		return category
	}
}

func appendCleanTags(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := map[string]struct{}{}
	for _, existing := range cleanTags(out) {
		seen[existing] = struct{}{}
	}
	for _, value := range cleanTags(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func normalizeMemoryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.MemoryKindProcedural:
		return model.MemoryKindProcedural
	case model.MemoryKindInstruction:
		return model.MemoryKindInstruction
	case model.MemoryKindPreference:
		return model.MemoryKindPreference
	case model.MemoryKindReference:
		return model.MemoryKindReference
	default:
		return model.MemoryKindEpisodic
	}
}

func vectorLiteral(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%f", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
