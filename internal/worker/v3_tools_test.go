package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

func TestMemoryRetrievalEvidenceRecordsIncludesNoMatchEvidence(t *testing.T) {
	records := memoryRetrievalEvidenceRecords("rust ownership", []string{"expertise", "rust"}, nil)
	if len(records) != 1 {
		t.Fatalf("records=%d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != evidence.KindModelJudgment {
		t.Fatalf("kind=%q, want %q", record.Kind, evidence.KindModelJudgment)
	}
	if record.SourceRef != "memory.retrieve:no_matches" {
		t.Fatalf("source_ref=%q", record.SourceRef)
	}
	if !strings.Contains(record.Summary, "no relevant matches") {
		t.Fatalf("summary should describe no-match retrieval evidence: %q", record.Summary)
	}
	if got := record.Metadata["matches"]; got != 0 {
		t.Fatalf("matches metadata=%v, want 0", got)
	}
}

func TestMemoryRetrievalEvidenceRecordsLimitsMatchedEvidence(t *testing.T) {
	matches := make([]model.MemoryMatch, 10)
	for i := range matches {
		matches[i] = model.MemoryMatch{
			ID:      int64(i + 1),
			Kind:    "expertise_research",
			Content: "memory content",
			Tags:    []string{"expertise"},
			Score:   0.8,
		}
	}
	records := memoryRetrievalEvidenceRecords("go context", nil, matches)
	if len(records) != 8 {
		t.Fatalf("records=%d, want 8", len(records))
	}
	if records[0].Kind != evidence.KindMemoryExcerpt || records[0].SourceRef != "memory:1" {
		t.Fatalf("first record=%#v", records[0])
	}
	if records[7].SourceRef != "memory:8" {
		t.Fatalf("last record source=%q, want memory:8", records[7].SourceRef)
	}
}

func TestInferredMemoryScopeTagsAddsTechnicalExpertiseTags(t *testing.T) {
	got := inferredMemoryScopeTags("Tokio spawn_blocking in Rust runtime", []string{"project:abc"})
	want := []string{"rust", "expertise", "tokio"}
	for _, tag := range want {
		found := false
		for _, gotTag := range got {
			if gotTag == tag {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("inferred tags=%v, missing %q", got, tag)
		}
	}
}

func TestDiversifyMemoryMatchesBySourceURLPrefersDistinctSources(t *testing.T) {
	matches := []model.MemoryMatch{
		{ID: 1, Content: "source_url=https://a.test\nA1"},
		{ID: 2, Content: "source_url=https://a.test\nA2"},
		{ID: 3, Content: "source_url=https://b.test\nB1"},
		{ID: 4, Content: "source_url=https://c.test\nC1"},
	}
	got := diversifyMemoryMatchesBySourceURL(matches, 3)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	urls := map[string]struct{}{}
	for _, match := range got {
		urls[memoryMatchSourceURL(match.Content)] = struct{}{}
	}
	for _, want := range []string{"https://a.test", "https://b.test", "https://c.test"} {
		if _, ok := urls[want]; !ok {
			t.Fatalf("urls=%v missing %s", urls, want)
		}
	}
}
