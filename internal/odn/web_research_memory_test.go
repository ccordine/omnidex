package odn

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestResearchWebToMemoryStoresSearchResultsWithSourceTags(t *testing.T) {
	ctx := context.Background()
	searcher := fakeWebSearchService{
		results: []websearch.Result{
			{
				Provider:    "duckduckgo",
				SearchURL:   "https://duckduckgo.com/html/?q=weather+Thailand",
				URL:         "https://example.com/weather/thailand",
				Title:       "Thailand Weather",
				Snippet:     "Thailand weather summary.",
				Content:     "Bangkok weather is warm and humid. Phuket weather is rainy in monsoon season.",
				RetrievedAt: time.Date(2026, 5, 18, 18, 0, 0, 0, time.UTC),
			},
			{
				Provider:    "google",
				SearchURL:   "https://www.google.com/search?q=weather+Thailand",
				URL:         "https://weather.example.org/thailand",
				Title:       "Weather in Thailand now",
				Content:     "Current conditions list temperature, humidity, and wind by city.",
				RetrievedAt: time.Date(2026, 5, 18, 18, 1, 0, 0, time.UTC),
			},
		},
	}
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)

	result, err := ResearchWebToMemory(ctx, "weather Thailand now", searcher, store, WebResearchMemoryConfig{
		AgentID:   "research_manager",
		MaxChunks: 4,
		Tags:      []string{"weather", "thailand"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.StoredCount != 2 {
		t.Fatalf("stored = %d, want 2", result.StoredCount)
	}
	memories, err := store.SearchMemory(ctx, "Bangkok weather", []string{"weather"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) == 0 {
		t.Fatal("expected searchable memory")
	}
	if !memoryRecordsContain(memories, "WEB_RESEARCH_MEMORY") {
		t.Fatalf("missing memory header: %#v", memories)
	}
	if !memoryRecordsContain(memories, "url: https://example.com/weather/thailand") {
		t.Fatalf("missing source URL: %#v", memories)
	}
	if !memoryRecordHasTag(memories, "provider:duckduckgo") {
		t.Fatalf("missing provider tag: %#v", memories)
	}
	if !memoryRecordHasTag(memories, "host:example.com") {
		t.Fatalf("missing host tag: %#v", memories)
	}
	for _, wantSQL := range []string{"CREATE TABLE IF NOT EXISTS memory_chunks", "INSERT INTO memory_chunks", "INSERT INTO tags"} {
		if !runner.SawSQL(wantSQL) {
			t.Fatalf("runner did not execute SQL containing %q", wantSQL)
		}
	}
}

func TestBuildWebResearchMemoryChunksSkipsEmptyContentAndCaps(t *testing.T) {
	results := []websearch.Result{
		{Provider: "duckduckgo", URL: "https://a.example/1", Content: "one"},
		{Provider: "duckduckgo", URL: "https://a.example/2", Content: ""},
		{Provider: "yahoo", URL: "https://b.example/3", Content: "three"},
	}

	chunks := buildWebResearchMemoryChunks("query", results, WebResearchMemoryConfig{MaxChunks: 1, ChunkSize: 10})

	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "query: query") {
		t.Fatalf("chunk missing query:\n%s", chunks[0].Content)
	}
	if !containsString(chunks[0].Tags, "web") || !containsString(chunks[0].Tags, "research") {
		t.Fatalf("chunk missing base tags: %#v", chunks[0].Tags)
	}
}

type fakeWebSearchService struct {
	results []websearch.Result
	err     error
}

func (f fakeWebSearchService) SearchAll(ctx context.Context, query string) ([]websearch.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func memoryRecordHasTag(records []MemoryRecord, tag string) bool {
	for _, record := range records {
		if containsString(record.Tags, tag) {
			return true
		}
	}
	return false
}
