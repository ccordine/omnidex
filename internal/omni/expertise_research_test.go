package omni

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestExpertiseResearchJobBuildsOmnibusMemoryAndRecallsAdjacentKnowledge(t *testing.T) {
	ctx := context.Background()
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)
	planner := &fakeCommandDecisionClient{responses: []string{
		`{"subject":"trigonometry","research_queries":["trigonometry unit circle identities source","trigonometry sine cosine tangent lesson applications","calculus derivatives of trigonometric functions"],"adjacent_topics":["unit circle","calculus","geometry","lesson plans"],"success_criteria":["core definitions sourced","adjacent calculus links sourced","future app lessons can cite memories"]}`,
	}}
	searcher := trackingExpertiseSearchService{resultsByQuery: map[string][]websearch.Result{
		"trigonometry unit circle identities source": {
			{
				Provider:    "test",
				Title:       "Unit circle and trigonometric identities",
				URL:         "https://math.example.edu/unit-circle",
				Content:     "The unit circle defines sine as the y-coordinate and cosine as the x-coordinate. The identity sin^2(theta)+cos^2(theta)=1 anchors many trigonometry lessons.",
				RetrievedAt: time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
			},
		},
		"trigonometry sine cosine tangent lesson applications": {
			{
				Provider:    "test",
				Title:       "Sine cosine tangent applications",
				URL:         "https://lessons.example.org/trig-applications",
				Content:     "A lesson plan can connect sine, cosine, and tangent to right triangles, graphing periodic motion, and interactive practice problems.",
				RetrievedAt: time.Date(2026, 5, 18, 12, 1, 0, 0, time.UTC),
			},
		},
		"calculus derivatives of trigonometric functions": {
			{
				Provider:    "test",
				Title:       "Derivatives of trigonometric functions",
				URL:         "https://calculus.example.net/trig-derivatives",
				Content:     "Calculus extends trigonometry with derivative rules such as d/dx sin(x)=cos(x), useful for a later calculus lesson module.",
				RetrievedAt: time.Date(2026, 5, 18, 12, 2, 0, 0, time.UTC),
			},
		},
	}}

	result, err := BuildExpertiseResearchOmnibus(ctx, "trigonometry", planner, &searcher, store, ExpertiseResearchConfig{
		AgentID:        "expertise_manager",
		MaxQueries:     5,
		MaxResultsEach: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls = %d, want 1", planner.calls)
	}
	if searcher.calls != 3 {
		t.Fatalf("search calls = %d, want 3", searcher.calls)
	}
	if result.StoredCount != 4 {
		t.Fatalf("stored memories = %d, want 4: %#v", result.StoredCount, result.StoredMemories)
	}
	for _, want := range []string{"EXPERTISE_SOURCE_MEMORY", "EXPERTISE_OMNIBUS_MEMORY", "unit circle", "d/dx sin(x)=cos(x)"} {
		if !memoryRecordsContain(result.StoredMemories, want) {
			t.Fatalf("expertise memories missing %q: %#v", want, result.StoredMemories)
		}
	}
	for _, wantTag := range []string{"expertise", "expertise:trigonometry", "related:calculus", "related:lesson-plans", "expertise-omnibus"} {
		if !memoryRecordHasTag(result.StoredMemories, wantTag) {
			t.Fatalf("expertise memory missing tag %q: %#v", wantTag, result.StoredMemories)
		}
	}

	recall, err := RecallExpertiseFromMemory(ctx, "trigonometry", "Build a math app lesson about sine and cosine with calculus extension.", store, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !recall.UsedMemory || recall.NeedsResearch {
		t.Fatalf("expected expertise recall from memory: %#v", recall)
	}
	if len(recall.Direct) == 0 {
		t.Fatalf("expected direct expertise memories: %#v", recall)
	}
	if len(recall.Related) == 0 {
		t.Fatalf("expected adjacent related memories: %#v", recall)
	}
	for _, want := range []string{"Answered from expertise memory", "trigonometry", "EXPERTISE_SOURCE_MEMORY"} {
		if !strings.Contains(recall.Answer, want) {
			t.Fatalf("recall answer missing %q:\n%s", want, recall.Answer)
		}
	}
	if searcher.calls != 3 {
		t.Fatalf("recall should not perform new web searches; calls=%d", searcher.calls)
	}
	for _, wantSQL := range []string{"INSERT INTO memory_chunks", "INSERT INTO tags", "FROM memory_chunks"} {
		if !runner.SawSQL(wantSQL) {
			t.Fatalf("runner did not execute SQL containing %q\nqueries:\n%s", wantSQL, strings.Join(runner.SQLLog, "\n---\n"))
		}
	}
}

func TestExpertiseResearchRequiresLLMPlanner(t *testing.T) {
	_, err := BuildExpertiseResearchOmnibus(context.Background(), "calculus", nil, &trackingExpertiseSearchService{}, NewPGMemoryStore(newFakeMemoryRunner()), ExpertiseResearchConfig{})
	if err == nil {
		t.Fatal("expected missing planner to fail")
	}
}

func TestExpertiseRecallReturnsNeedsResearchWhenNoMemoryMatches(t *testing.T) {
	recall, err := RecallExpertiseFromMemory(context.Background(), "abstract algebra", "Build a lesson on groups.", NewPGMemoryStore(newFakeMemoryRunner()), 5)
	if err != nil {
		t.Fatal(err)
	}
	if recall.UsedMemory || !recall.NeedsResearch {
		t.Fatalf("expected missing expertise memory to request research: %#v", recall)
	}
}

type trackingExpertiseSearchService struct {
	resultsByQuery map[string][]websearch.Result
	calls          int
	queries        []string
}

func (s *trackingExpertiseSearchService) SearchAll(ctx context.Context, query string) ([]websearch.Result, error) {
	s.calls++
	s.queries = append(s.queries, query)
	return s.resultsByQuery[query], nil
}
