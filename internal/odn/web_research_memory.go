package odn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

const (
	defaultResearchMemoryAgent = "odn_researcher"
	defaultResearchMemoryKind  = "web_research"
	defaultResearchChunkChars  = 1800
	defaultResearchMaxChunks   = 12
)

type WebSearchService interface {
	SearchAll(ctx context.Context, query string) ([]websearch.Result, error)
}

type WebResearchMemoryConfig struct {
	AgentID   string
	Kind      string
	MaxChunks int
	ChunkSize int
	Tags      []string
}

type WebResearchMemoryResult struct {
	Query          string
	Results        []websearch.Result
	StoredMemories []MemoryRecord
	StoredCount    int
	SkippedCount   int
}

func ResearchWebToMemory(ctx context.Context, query string, searcher WebSearchService, memory *PGMemoryStore, cfg WebResearchMemoryConfig) (WebResearchMemoryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return WebResearchMemoryResult{}, fmt.Errorf("research query is required")
	}
	if searcher == nil {
		return WebResearchMemoryResult{}, fmt.Errorf("web search service is required")
	}
	if memory == nil {
		return WebResearchMemoryResult{}, fmt.Errorf("memory store is required")
	}
	cfg = normalizeWebResearchMemoryConfig(cfg)

	results, err := searcher.SearchAll(ctx, query)
	if err != nil {
		return WebResearchMemoryResult{}, err
	}
	out := WebResearchMemoryResult{Query: query, Results: results}
	if err := memory.EnsureSchema(ctx); err != nil {
		return out, err
	}

	chunks := buildWebResearchMemoryChunks(query, results, cfg)
	for _, chunk := range chunks {
		record, err := memory.AddMemory(ctx, cfg.AgentID, cfg.Kind, chunk.Content, chunk.Tags)
		if err != nil {
			return out, err
		}
		out.StoredMemories = append(out.StoredMemories, record)
		out.StoredCount++
	}
	out.SkippedCount = maxInt(0, len(results)-out.StoredCount)
	return out, nil
}

type webResearchMemoryChunk struct {
	Content string
	Tags    []string
}

func buildWebResearchMemoryChunks(query string, results []websearch.Result, cfg WebResearchMemoryConfig) []webResearchMemoryChunk {
	chunks := make([]webResearchMemoryChunk, 0, minInt(len(results), cfg.MaxChunks))
	baseTags := cleanMemoryTags(append([]string{"web", "research", "query:" + query}, cfg.Tags...))
	for _, result := range results {
		if len(chunks) >= cfg.MaxChunks {
			break
		}
		content := formatWebResearchMemoryContent(query, result, cfg.ChunkSize)
		if strings.TrimSpace(content) == "" {
			continue
		}
		tags := append([]string{}, baseTags...)
		if provider := strings.TrimSpace(result.Provider); provider != "" {
			tags = append(tags, "provider:"+provider)
		}
		if host := hostTagFromURL(result.URL); host != "" {
			tags = append(tags, "host:"+host)
		}
		chunks = append(chunks, webResearchMemoryChunk{Content: content, Tags: cleanMemoryTags(tags)})
	}
	return chunks
}

func formatWebResearchMemoryContent(query string, result websearch.Result, maxContent int) string {
	body := strings.TrimSpace(result.Content)
	if body == "" {
		body = strings.TrimSpace(result.Snippet)
	}
	if body == "" {
		return ""
	}
	if maxContent <= 0 {
		maxContent = defaultResearchChunkChars
	}
	if len(body) > maxContent {
		body = body[:maxContent] + "\n...[truncated]"
	}
	retrieved := result.RetrievedAt
	if retrieved.IsZero() {
		retrieved = time.Now().UTC()
	}
	return strings.TrimSpace(strings.Join([]string{
		"WEB_RESEARCH_MEMORY",
		"query: " + strings.TrimSpace(query),
		"provider: " + strings.TrimSpace(result.Provider),
		"title: " + strings.TrimSpace(result.Title),
		"url: " + strings.TrimSpace(result.URL),
		"search_url: " + strings.TrimSpace(result.SearchURL),
		"retrieved_at: " + retrieved.Format(time.RFC3339),
		"content:",
		body,
	}, "\n"))
}

func normalizeWebResearchMemoryConfig(cfg WebResearchMemoryConfig) WebResearchMemoryConfig {
	if strings.TrimSpace(cfg.AgentID) == "" {
		cfg.AgentID = defaultResearchMemoryAgent
	}
	if strings.TrimSpace(cfg.Kind) == "" {
		cfg.Kind = defaultResearchMemoryKind
	}
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = defaultResearchMaxChunks
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = defaultResearchChunkChars
	}
	return cfg
}

func hostTagFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "/")
	if len(parts) < 3 {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parts[2]))
	host = strings.TrimPrefix(host, "www.")
	host = strings.ReplaceAll(host, ":", "-")
	return host
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
