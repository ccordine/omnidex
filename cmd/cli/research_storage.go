package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

func openResearchMemoryStore(ctx context.Context, mode string, apiClient *client.Client, databaseURL, embeddingBaseURL, embeddingModel string) (researchMemoryStore, func(), error) {
	switch mode {
	case "api":
		return apiResearchMemoryStore{client: apiClient}, func() {}, nil
	case "direct-db":
		cleanURL := strings.TrimSpace(databaseURL)
		if cleanURL == "" {
			return nil, func() {}, fmt.Errorf("--store direct-db requires --database-url or DATABASE_URL")
		}
		pool, err := db.Connect(ctx, cleanURL)
		if err != nil {
			return nil, func() {}, fmt.Errorf("connect research database: %w", err)
		}
		repo := queue.New(pool)
		embedder := ollama.New(embeddingBaseURL, "", embeddingModel, 2*time.Minute, 0)
		return directDBResearchMemoryStore{repo: repo, embedder: embedder}, pool.Close, nil
	default:
		return nil, func() {}, fmt.Errorf("invalid research memory store mode %q", mode)
	}
}

func researchSearchQuery(topic string) string {
	clean := strings.TrimSpace(topic)
	if clean == "" {
		return ""
	}
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 180 {
		clean = clean[:180]
	}
	lower := strings.ToLower(clean)
	switch {
	case strings.Contains(lower, "vite"):
		return clean + " Vite official documentation guide config build plugins HMR production"
	case strings.Contains(lower, "react"):
		return clean + " React official documentation react.dev learn reference hooks components Vite"
	case strings.Contains(lower, "node.js") || strings.Contains(lower, "nodejs") || strings.Contains(lower, "node js"):
		return clean + " Node.js official documentation API learn event loop modules streams security"
	case strings.Contains(lower, "rust"):
		return clean + " official Rust documentation reference Cargo book Rustonomicon Tokio docs"
	case strings.Contains(lower, "golang") || strings.Contains(lower, "go "):
		return clean + " go.dev official documentation Effective Go standard library"
	case strings.Contains(lower, "php"):
		return clean + " php.net manual official documentation composer psr"
	case strings.Contains(lower, "docker"):
		return clean + " Docker official documentation compose buildfile best practices"
	case strings.Contains(lower, "postgres") || strings.Contains(lower, "pgsql") || strings.Contains(lower, "postgresql"):
		return clean + " PostgreSQL official documentation current manual"
	case strings.Contains(lower, "javascript") || strings.Contains(lower, "node"):
		return clean + " MDN JavaScript reference Node.js official documentation"
	default:
		return clean + " official documentation reference guide"
	}
}

func normalizeResearchReasoning(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "deep":
		return "deep"
	case "auto":
		return "auto"
	case "fast":
		return "fast"
	default:
		return ""
	}
}

func inferResearchTags(topic, slug string) []string {
	out := []string{"research", "reference", "knowledge-base"}
	if clean := strings.TrimSpace(slug); clean != "" {
		out = append(out, "topic-"+clean)
	}
	for _, token := range strings.Fields(normalizeForMatch(topic)) {
		if len(token) < 3 {
			continue
		}
		out = append(out, token)
	}
	out = append(out, "as-of-"+time.Now().Format("2006"))
	return out
}

func loadResearchIndex(path string) (researchIndex, error) {
	index := researchIndex{Entries: map[string]researchEntry{}}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return index, nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, nil
		}
		return index, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return index, nil
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, err
	}
	if index.Entries == nil {
		index.Entries = map[string]researchEntry{}
	}
	return index, nil
}

func saveResearchIndex(path string, index researchIndex) error {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil
	}
	if index.Entries == nil {
		index.Entries = map[string]researchEntry{}
	}

	dir := filepath.Dir(cleanPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(cleanPath, encoded, 0o644)
}

func researchEntryFresh(entry researchEntry, now time.Time, refreshDays int) (bool, time.Duration) {
	if refreshDays <= 0 {
		return false, 0
	}
	timestamp := strings.TrimSpace(entry.LastResearchedAt)
	if timestamp == "" {
		return false, 0
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false, 0
	}
	age := now.Sub(parsed)
	if age < 0 {
		age = 0
	}
	freshWindow := time.Duration(refreshDays) * 24 * time.Hour
	return age < freshWindow, age
}
