package omni

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResearchWebDocsParsesAndSearchesLocalDocs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tailwind":
			w.Write([]byte(`<!doctype html><html><head><title>Tailwind Width</title></head><body><main><h1>Width</h1><p>Utilities for setting the width of an element.</p><p>The w-1/2 utility sets width: 50%.</p></main></body></html>`))
		case "/postgres":
			w.Write([]byte(`<!doctype html><html><body><h1>Validation</h1><p>The required validation rule verifies that the field under validation must be present and not empty.</p></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := ResearchWebDocs(
		context.Background(),
		"Find Tailwind width and PostgreSQL required validation rule details.",
		[]WebDocSource{
			{Name: "tailwind", URL: server.URL + "/tailwind"},
			{Name: "postgres", URL: server.URL + "/postgres"},
		},
		[]string{
			"Utilities for setting the width of an element",
			"field under validation must be present and not empty",
		},
		WebDocResearchConfig{
			FetchTimeout: 5 * time.Second,
			ChunkConfig:  DocumentSearchConfig{ChunkChars: 160, ChunkOverlap: 40},
			MaxHits:      10,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Fatal("expected documentation hits")
	}
	if len(result.Hits) != 2 {
		t.Fatalf("hits = %d, want 2: %#v", len(result.Hits), result.Hits)
	}
	if !webDocHitsContain(result.Hits, "tailwind", "Utilities for setting the width of an element") {
		t.Fatalf("missing Tailwind width hit: %#v", result.Hits)
	}
	if !webDocHitsContain(result.Hits, "postgres", "field under validation must be present and not empty") {
		t.Fatalf("missing PostgreSQL validation hit: %#v", result.Hits)
	}
	for _, hit := range result.Hits {
		if hit.StartOffset < 0 || hit.Line <= 0 || hit.Column <= 0 {
			t.Fatalf("hit lacks exact location: %#v", hit)
		}
	}
}

func TestHTMLToSearchableTextRemovesScriptAndKeepsText(t *testing.T) {
	text := HTMLToSearchableText(`<html><head><meta name="description" content="Meta phrase for docs."/></head><script>hiddenNeedle()</script><body><h1>Docs</h1><p>Visible phrase.</p></body></html>`)

	if strings.Contains(text, "hiddenNeedle") {
		t.Fatalf("script content leaked into searchable text: %q", text)
	}
	if !strings.Contains(text, "Visible phrase") {
		t.Fatalf("visible text missing: %q", text)
	}
	if !strings.Contains(text, "Meta phrase for docs") {
		t.Fatalf("meta description missing: %q", text)
	}
}

func TestLiveWebDocResearchTailwindAndPostgreSQL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live documentation fetch in short mode")
	}

	result, err := ResearchWebDocs(
		context.Background(),
		"Find exact documentation snippets for Tailwind width utilities and PostgreSQL required validation.",
		[]WebDocSource{
			{Name: "tailwind-width", URL: "https://tailwindcss.com/docs/width"},
			{Name: "postgres-validation", URL: "https://postgres.com/docs/12.x/validation"},
		},
		[]string{
			"Utilities for setting the width of an element",
			"field under validation must be present",
		},
		WebDocResearchConfig{
			FetchTimeout: 20 * time.Second,
			ChunkConfig:  DocumentSearchConfig{ChunkChars: 2500, ChunkOverlap: 300},
			MaxHits:      8,
		},
	)
	if err != nil {
		t.Skipf("live documentation fetch unavailable: %v", err)
	}
	if !webDocHitsContain(result.Hits, "tailwind-width", "Utilities for setting the width of an element") {
		t.Fatalf("missing live Tailwind width documentation hit: %#v", result.Hits)
	}
	if !webDocHitsContain(result.Hits, "postgres-validation", "field under validation must be present") {
		t.Fatalf("missing live PostgreSQL validation documentation hit: %#v", result.Hits)
	}
}

func webDocHitsContain(hits []WebDocHit, sourceName, text string) bool {
	for _, hit := range hits {
		if hit.Source.Name == sourceName && strings.Contains(strings.ToLower(hit.Excerpt), strings.ToLower(text)) {
			return true
		}
	}
	return false
}
