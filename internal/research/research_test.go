package research

import (
	"strings"
	"testing"
	"time"
)

func TestSearchQueryFocusesTechnicalDocs(t *testing.T) {
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
		if got := SearchQuery(topic); !strings.Contains(got, want) {
			t.Fatalf("SearchQuery(%q)=%q, want containing %q", topic, got, want)
		}
	}
}

func TestOfficialSourceURLsCoversExpertiseTopics(t *testing.T) {
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
		urls := OfficialSourceURLs(topic)
		found := false
		for _, url := range urls {
			if url == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("OfficialSourceURLs(%q)=%v, missing %q", topic, urls, want)
		}
	}
}

func TestViteRoutingWinsOverReactTemplateText(t *testing.T) {
	topic := "Vite expert reference for React templates"
	if got := SearchQuery(topic); !strings.Contains(got, "Vite official documentation") {
		t.Fatalf("SearchQuery(%q)=%q, want Vite query", topic, got)
	}
	urls := OfficialSourceURLs(topic)
	if len(urls) == 0 || urls[0] != "https://vite.dev/guide/" {
		t.Fatalf("OfficialSourceURLs(%q)=%v, want Vite docs first", topic, urls)
	}
}

func TestBuildInstructionUsesTechnicalShapeForReact(t *testing.T) {
	got := BuildInstruction("React JS expert reference", time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"durable technical expertise reference", "current recommended project setup", "APIs", "testing", "production pitfalls", "Last verified"} {
		if !strings.Contains(got, want) {
			t.Fatalf("technical research instruction missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "quests/missions") {
		t.Fatalf("technical research instruction should not use media/game requirements:\n%s", got)
	}
}

func TestDocumentSourceSlugUsesURL(t *testing.T) {
	got := DocumentSourceSlug(Document{
		Section: "official-source",
		Content: "Research memory\nurl: https://react.dev/reference/react/useState\ncontent:\nbody",
	}, 0)
	if got != "react-dev-reference-react-usestate" {
		t.Fatalf("source slug=%q", got)
	}
}

func TestPrepareChunksUsesDistinctSourceLabels(t *testing.T) {
	docs := []Document{
		{Section: "official-source", Content: "url: https://react.dev/learn\ncontent:\n" + strings.Repeat("react ", 200)},
		{Section: "official-source", Content: "url: https://vite.dev/guide/\ncontent:\n" + strings.Repeat("vite ", 200)},
	}
	chunks := PrepareChunks(docs, PrepareOptions{
		Topic:        "React and Vite",
		Slug:         "react-vite",
		SourcePrefix: "research",
		ChunkSize:    120,
		MaxChunks:    4,
		Tags:         []string{"expertise"},
	})
	if len(chunks) != 4 {
		t.Fatalf("chunks=%d want 4", len(chunks))
	}
	if chunks[0].Source == chunks[1].Source {
		t.Fatalf("distinct documents should have distinct source labels: %#v", chunks[:2])
	}
	if !strings.Contains(chunks[0].Content, "source_url=") {
		t.Fatalf("chunk metadata missing source url: %s", chunks[0].Content)
	}
}
