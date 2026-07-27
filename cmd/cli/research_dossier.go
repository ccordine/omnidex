package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeResearchDossier(root, slug, topic string, jobID int64, requestedAt time.Time, documents []researchDocument, tags []string, sourcePrefix string, storedChunks int) (string, error) {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		return "", nil
	}
	cleanSlug := sanitizeMemorySourceToken(slug)
	if cleanSlug == "" {
		cleanSlug = fmt.Sprintf("topic-%d", jobID)
	}
	if err := os.MkdirAll(cleanRoot, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(cleanRoot, fmt.Sprintf("%s-job-%d.md", cleanSlug, jobID))
	body := buildResearchDossier(topic, jobID, requestedAt, documents, tags, sourcePrefix, storedChunks)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildResearchDossier(topic string, jobID int64, requestedAt time.Time, documents []researchDocument, tags []string, sourcePrefix string, storedChunks int) string {
	if requestedAt.IsZero() {
		requestedAt = time.Now()
	}
	lines := []string{
		"# Research Dossier",
		"",
		"topic: " + strings.TrimSpace(topic),
		fmt.Sprintf("job_id: %d", jobID),
		"captured_at: " + requestedAt.UTC().Format(time.RFC3339),
		"source_prefix: " + strings.TrimSpace(sourcePrefix),
		fmt.Sprintf("stored_memory_chunks: %d", storedChunks),
		"tags: " + strings.Join(cleanDossierTags(tags), ","),
		"",
		"## Purpose",
		"",
		"This file is the full-text reference account captured for the research run. Memory chunks are optimized for retrieval; this dossier preserves the larger text Omnidex used, including synthesized report, web context, analysis context, URLs, excerpts, and source notes when available.",
		"",
	}
	for _, doc := range documents {
		section := strings.TrimSpace(doc.Section)
		if section == "" {
			section = "section"
		}
		lines = append(lines, "## "+section, "", strings.TrimSpace(doc.Content), "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func cleanDossierTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func buildResearchInstruction(topic string, now time.Time) string {
	today := now.Format("2006-01-02")
	if researchTopicLooksTechnical(topic) {
		return strings.TrimSpace(fmt.Sprintf(`Research the topic "%s" comprehensively and produce a durable technical expertise reference.

Requirements:
1) Prefer primary/official documentation and include source URLs inline.
2) Cover the current recommended project setup, core concepts, APIs, conventions, file structure, dependency/tooling expectations, testing, debugging, deployment/build notes, and production pitfalls.
3) Include small canonical examples and explain when to use each pattern.
4) Call out outdated/deprecated guidance, version-sensitive behavior, uncertainty, or conflicting information explicitly.
5) Organize output with clear markdown headings and concise bullets that are useful to an implementation planner.
6) End with a short "Last verified" line using date %s.
`, topic, today))
	}
	return strings.TrimSpace(fmt.Sprintf(`Research the topic "%s" comprehensively and produce a durable technical reference.

Requirements:
1) Cover core overview, timeline/history, terminology/glossary, key entities, major systems/mechanics, and practical FAQs.
2) Include detailed, concrete facts and edge cases. For games/media topics, include quests/missions, items/equipment, factions/characters, locations, and in-universe language/slang.
3) Prefer primary/official sources when possible and include source URLs inline.
4) Call out uncertainty or conflicting information explicitly.
5) Organize output with clear markdown headings and concise bullet points.
6) End with a short "Last verified" line using date %s.
`, topic, today))
}

func researchTopicLooksTechnical(topic string) bool {
	lower := strings.ToLower(topic)
	needles := []string{
		"api",
		"docker",
		"go lang",
		"golang",
		"javascript",
		"node",
		"php",
		"postgres",
		"postgresql",
		"pgsql",
		"react",
		"rust",
		"software",
		"typescript",
		"zig",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func normalizeResearchStoreMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "api":
		return "api"
	case "direct-db", "direct_db", "db":
		return "direct-db"
	default:
		return ""
	}
}
