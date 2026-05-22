package omni

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDocumentationResearchCatalogsDocsIntoPGMemoryAndReusesWithoutScrape(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch r.URL.Path {
		case "/tailwind/width":
			w.Write([]byte(`<!doctype html><html><body><main>
<h1>Width</h1>
<p>Utilities for setting the width of an element.</p>
<p>The w-1/2 utility sets width: 50% and can be used in responsive UI layouts.</p>
</main></body></html>`))
		case "/postgres/validation":
			w.Write([]byte(`<!doctype html><html><body><main>
<h1>Validation</h1>
<p>The required validation rule verifies that the field under validation must be present and not empty.</p>
<p>Use validation memories only as recalled documentation evidence, not as a fresh prompt.</p>
</main></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)
	question := "Catalog Tailwind width utilities and PostgreSQL required validation documentation for future project work."
	result, err := ResearchWebDocsToMemory(
		ctx,
		question,
		[]WebDocSource{
			{Name: "tailwind-width", URL: server.URL + "/tailwind/width"},
			{Name: "postgres-validation", URL: server.URL + "/postgres/validation"},
		},
		[]string{
			"w-1/2 utility sets width: 50%",
			"field under validation must be present and not empty",
		},
		store,
		WebDocResearchConfig{
			FetchTimeout: 5 * time.Second,
			ChunkConfig:  DocumentSearchConfig{ChunkChars: 140, ChunkOverlap: 35},
			MaxHits:      8,
		},
		DocResearchMemoryConfig{
			AgentID: "doc_manager",
			Tags:    []string{"tailwind", "postgres", "project-patterns"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("initial scrape requests = %d, want 2", requestCount)
	}
	if !result.Research.Found || result.Research.WorkerCount < 2 || result.Research.ChunkCount < 2 {
		t.Fatalf("research did not use chunked worker search: %#v", result.Research)
	}
	if result.StoredCount != 2 {
		t.Fatalf("stored memories = %d, want 2", result.StoredCount)
	}
	if !memoryRecordsContain(result.StoredMemories, "DOC_RESEARCH_MEMORY") {
		t.Fatalf("stored memories missing documentation header: %#v", result.StoredMemories)
	}
	if !memoryRecordsContain(result.StoredMemories, "line=") || !memoryRecordsContain(result.StoredMemories, "start_offset=") {
		t.Fatalf("stored memories missing exact location metadata: %#v", result.StoredMemories)
	}
	for _, wantTag := range []string{"documentation", "doc-research", "tailwind", "postgres", "project-patterns", "source:tailwind-width", "source:postgres-validation"} {
		if !memoryRecordHasTag(result.StoredMemories, wantTag) {
			t.Fatalf("stored memories missing tag %q: %#v", wantTag, result.StoredMemories)
		}
	}

	server.Close()
	requestCountAfterCatalog := requestCount
	answer, err := AnswerDocumentationQuestionFromMemory(ctx, "How does Tailwind w-1/2 set width?", store, []string{"tailwind"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !answer.UsedMemory || answer.NeedsScrape {
		t.Fatalf("expected memory-backed answer without scrape: %#v", answer)
	}
	if requestCount != requestCountAfterCatalog {
		t.Fatalf("memory answer performed another scrape: before=%d after=%d", requestCountAfterCatalog, requestCount)
	}
	for _, want := range []string{"Documentation authority brief", "documentation_specialist", "pgsql_memory", "tailwind-width", "w-1/2 utility sets width: 50%", "sources:"} {
		if !strings.Contains(answer.Answer, want) {
			t.Fatalf("memory answer missing %q:\n%s", want, answer.Answer)
		}
	}
	if answer.Brief.Role != "documentation_specialist" || len(answer.Brief.Sources) == 0 {
		t.Fatalf("answer missing documentation authority brief: %#v", answer.Brief)
	}
	if len(answer.Memories) == 0 {
		t.Fatal("expected answer to include source memories")
	}

	fresh, err := AnswerDocumentationQuestionFromMemory(ctx, "What does a missing Alpine directive do?", store, []string{"alpine"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.UsedMemory || !fresh.NeedsScrape {
		t.Fatalf("fresh unknown prompt should not be confused with existing memory: %#v", fresh)
	}
	for _, wantSQL := range []string{"INSERT INTO memory_chunks", "INSERT INTO tags", "FROM memory_chunks"} {
		if !runner.SawSQL(wantSQL) {
			t.Fatalf("runner did not execute SQL containing %q\nqueries:\n%s", wantSQL, strings.Join(runner.SQLLog, "\n---\n"))
		}
	}
}

func TestDocumentationAuthorityBriefClassifiesCodingGuidance(t *testing.T) {
	memories := []MemoryRecord{{
		Kind: "documentation_research",
		Content: strings.Join([]string{
			"DOC_RESEARCH_MEMORY",
			"source_name: vite-react",
			"url: https://vite.dev/guide/",
			"location: line=10 column=1 start_offset=1 end_offset=100",
			"excerpt:",
			"Install dependencies with npm install and start the dev server with npm run dev. Place React components in src/ and keep app entrypoints in src/main.jsx. The createRoot API mounts the component tree. Avoid deprecated ReactDOM.render usage. Example usage imports createRoot from react-dom/client.",
		}, "\n"),
	}}
	brief := BuildDocumentationAuthorityBrief("How do I start and structure a Vite React app?", memories)
	answer := FormatDocumentationAuthorityBrief(brief)

	for _, want := range []string{
		"getting_started:",
		"locations:",
		"apis:",
		"risks:",
		"sources:",
		"https://vite.dev/guide/",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("authority brief missing %q:\n%s", want, answer)
		}
	}
	if len(brief.GettingStarted) == 0 || len(brief.Locations) == 0 || len(brief.APIs) == 0 || len(brief.Risks) == 0 {
		t.Fatalf("brief did not classify guidance: %#v", brief)
	}
}

func TestInferDocumentationResearchTargetForZigBuildTasks(t *testing.T) {
	target := InferDocumentationResearchTarget("Build a CLI calculator application in Zig.")

	if len(target.Sources) == 0 {
		t.Fatal("expected Zig documentation sources")
	}
	if !webDocSourcesContain(target.Sources, "https://ziglang.org/learn/getting-started/") {
		t.Fatalf("missing Zig getting-started source: %#v", target.Sources)
	}
	if !webDocSourcesContain(target.Sources, "https://ziglang.org/documentation/master/") {
		t.Fatalf("missing Zig language reference source: %#v", target.Sources)
	}
	for _, want := range []string{"Run Hello World", "zig init", "std.debug.print", "pub fn main"} {
		if !stringSliceContains(target.Queries, want) {
			t.Fatalf("missing Zig query %q: %#v", want, target.Queries)
		}
	}
	if !stringSliceContains(target.Tags, "zig") {
		t.Fatalf("missing Zig tag: %#v", target.Tags)
	}
}

func TestDocResearchHitsAsMemoriesFormatsFetchedDocsForBriefs(t *testing.T) {
	memories := docResearchHitsAsMemories("Build Zig hello world", []WebDocHit{{
		Source:  WebDocSource{Name: "zig-getting-started", URL: "https://ziglang.org/learn/getting-started/"},
		Query:   "zig init",
		Line:    12,
		Column:  3,
		Excerpt: "Run Hello World. Navigate to your projects directory and run zig init. Running zig build run should then compile the executable and run it.",
	}})
	if len(memories) != 1 {
		t.Fatalf("memories = %d, want 1", len(memories))
	}
	brief := FormatDocumentationAuthorityBrief(BuildDocumentationAuthorityBrief("Build Zig hello world", memories))
	for _, want := range []string{"documentation_specialist", "zig-getting-started", "ziglang.org", "Run Hello World", "zig build run"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief missing %q: %s", want, brief)
		}
	}
}

func webDocSourcesContain(sources []WebDocSource, url string) bool {
	for _, source := range sources {
		if source.URL == url {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
