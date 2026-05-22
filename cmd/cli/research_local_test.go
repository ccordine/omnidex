package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestResearchEntryFresh(t *testing.T) {
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	freshEntry := researchEntry{LastResearchedAt: now.Add(-12 * time.Hour).Format(time.RFC3339)}
	staleEntry := researchEntry{LastResearchedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)}

	fresh, _ := researchEntryFresh(freshEntry, now, 2)
	if !fresh {
		t.Fatalf("expected fresh entry to be fresh")
	}

	fresh, _ = researchEntryFresh(staleEntry, now, 2)
	if fresh {
		t.Fatalf("expected stale entry to be stale")
	}
}

func TestCollectResearchDocuments(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{ID: 42, Result: "Comprehensive answer"},
		Contexts: []model.StepContext{
			{ID: 1, Key: "web_search", Value: "Source: google\nURL: https://example.com\nContent: detail"},
			{ID: 2, Key: "analyze", Value: "Deep analysis content"},
		},
	}

	docs := collectResearchDocuments("Cyberpunk 2077", details, true, true)
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(docs))
	}

	docs = collectResearchDocuments("Cyberpunk 2077", details, false, false)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc when contexts excluded, got %d", len(docs))
	}
}

func TestBuildResearchDossierPreservesFullTextSources(t *testing.T) {
	captured := time.Date(2026, 5, 22, 6, 0, 0, 0, time.UTC)
	dossier := buildResearchDossier("Rust", 42, captured, []researchDocument{
		{Section: "report", Content: "Synthesized report with https://doc.rust-lang.org/book/"},
		{Section: "web-context", Content: "Fetched source text and excerpts"},
	}, []string{"expertise", "rust", "rust"}, "research", 7)

	for _, want := range []string{
		"# Research Dossier",
		"topic: Rust",
		"job_id: 42",
		"stored_memory_chunks: 7",
		"tags: expertise,rust",
		"## report",
		"https://doc.rust-lang.org/book/",
		"## web-context",
		"Fetched source text and excerpts",
	} {
		if !strings.Contains(dossier, want) {
			t.Fatalf("dossier missing %q:\n%s", want, dossier)
		}
	}
}

func TestResearchSearchQueryFocusesTechnicalDocs(t *testing.T) {
	tests := map[string]string{
		"React JS expert reference":   "React official documentation",
		"Node.js expert reference":    "Node.js official documentation",
		"Vite expert reference":       "Vite official documentation",
		"Rust expert reference":       "official Rust documentation",
		"Go lang backend services":    "go.dev official documentation",
		"PHP production applications": "php.net manual",
		"Docker compose builds":       "Docker official documentation",
		"pgsql indexing and tuning":   "PostgreSQL official documentation",
		"JavaScript async runtime":    "MDN JavaScript reference",
	}
	for topic, want := range tests {
		if got := researchSearchQuery(topic); !strings.Contains(got, want) {
			t.Fatalf("researchSearchQuery(%q)=%q, want containing %q", topic, got, want)
		}
	}
}

func TestOfficialResearchSourceURLsCoversRequestedExpertiseTopics(t *testing.T) {
	tests := map[string]string{
		"React JS expert reference":   "https://react.dev/learn",
		"Node.js expert reference":    "https://nodejs.org/api/",
		"Vite expert reference":       "https://vite.dev/guide/",
		"Rust expert reference":       "https://doc.rust-lang.org/book/",
		"Go lang backend services":    "https://go.dev/doc/",
		"PHP production applications": "https://www.php.net/manual/en/",
		"Docker compose builds":       "https://docs.docker.com/get-started/",
		"pgsql indexing and tuning":   "https://www.postgresql.org/docs/current/",
		"JavaScript async runtime":    "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference",
	}
	for topic, want := range tests {
		urls := officialResearchSourceURLs(topic)
		found := false
		for _, url := range urls {
			if url == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("officialResearchSourceURLs(%q)=%v, missing %q", topic, urls, want)
		}
	}
}

func TestViteResearchRoutingWinsOverReactTemplateText(t *testing.T) {
	topic := "Vite expert reference for React templates"
	if got := researchSearchQuery(topic); !strings.Contains(got, "Vite official documentation") {
		t.Fatalf("researchSearchQuery(%q)=%q, want Vite query", topic, got)
	}
	urls := officialResearchSourceURLs(topic)
	if len(urls) == 0 || urls[0] != "https://vite.dev/guide/" {
		t.Fatalf("officialResearchSourceURLs(%q)=%v, want Vite docs first", topic, urls)
	}
}

func TestBuildResearchInstructionUsesTechnicalShapeForReact(t *testing.T) {
	got := buildResearchInstruction("React JS expert reference", time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"durable technical expertise reference",
		"current recommended project setup",
		"APIs",
		"testing",
		"production pitfalls",
		"Last verified",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("technical research instruction missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "quests/missions") {
		t.Fatalf("technical research instruction should not use media/game requirements:\n%s", got)
	}
}

func TestResearchHTMLToTextExtractsReadableText(t *testing.T) {
	got := researchHTMLToText("<html><body><h1>Rust &amp; Cargo</h1><p>Ownership</p></body></html>")
	if got != "Rust & Cargo Ownership" {
		t.Fatalf("text=%q", got)
	}
}

func TestPrefixResearchChunkMetadataIncludesSourceURL(t *testing.T) {
	got := prefixResearchChunkMetadata(researchDocument{
		Section: "official-source",
		Content: "Research memory\nurl: https://example.test/docs\ncontent:\nbody",
	}, "chunk body")
	if !strings.Contains(got, "section=official-source") {
		t.Fatalf("metadata missing section: %q", got)
	}
	if !strings.Contains(got, "source_url=https://example.test/docs") {
		t.Fatalf("metadata missing source url: %q", got)
	}
	if !strings.Contains(got, "chunk body") {
		t.Fatalf("metadata missing chunk body: %q", got)
	}
}

func TestResearchDocumentSourceSlugUsesURL(t *testing.T) {
	got := researchDocumentSourceSlug(researchDocument{
		Section: "official-source",
		Content: "Research memory\nurl: https://react.dev/reference/react/useState\ncontent:\nbody",
	}, 0)
	if got != "react-dev-reference-react-usestate" {
		t.Fatalf("source slug=%q", got)
	}
}

func TestWriteResearchDossierCreatesStableMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path, err := writeResearchDossier(dir, "rust-expert", "Rust", 99, time.Date(2026, 5, 22, 6, 0, 0, 0, time.UTC), []researchDocument{
		{Section: "report", Content: "full report"},
	}, []string{"expertise"}, "research", 1)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "rust-expert-job-99.md" {
		t.Fatalf("path=%q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "full report") {
		t.Fatalf("dossier did not preserve document body:\n%s", data)
	}
}

func TestInferResearchTags(t *testing.T) {
	tags := inferResearchTags("Cyberpunk 2077 quests and items", "cyberpunk-2077")
	if len(tags) == 0 {
		t.Fatalf("expected tags")
	}

	want := map[string]struct{}{
		"research":             {},
		"topic-cyberpunk-2077": {},
		"cyberpunk":            {},
		"quests":               {},
		"items":                {},
	}
	for tag := range want {
		found := false
		for _, value := range tags {
			if value == tag {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expected tag %q in %v", tag, tags)
		}
	}
}
