package main

import (
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
