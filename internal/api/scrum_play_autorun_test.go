package api

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
)

func TestIsScrumPlayQueueJob(t *testing.T) {
	scrumMeta, _ := json.Marshal(map[string]any{"source": "omni-scrum", "project_id": 1})
	if !isScrumPlayQueueJob(scrumMeta) {
		t.Fatal("expected omni-scrum job")
	}
	llmMeta, _ := json.Marshal(map[string]any{"source": "scrum_card_llm", "project_id": 1})
	if !isScrumPlayQueueJob(llmMeta) {
		t.Fatal("expected scrum_card_llm job")
	}
	otherMeta, _ := json.Marshal(map[string]any{"source": "chat"})
	if isScrumPlayQueueJob(otherMeta) {
		t.Fatal("expected non-scrum job to be ignored")
	}
	if isScrumPlayQueueJob(nil) {
		t.Fatal("expected empty metadata to be ignored")
	}
}

func TestScrumRequestFromContextHasURL(t *testing.T) {
	req := scrumRequestFromContext(nil)
	if req == nil {
		t.Fatal("expected request")
	}
	if req.URL == nil {
		t.Fatal("expected request URL")
	}
	if req.URL.String() != (&url.URL{}).String() {
		t.Fatalf("unexpected URL: %q", req.URL.String())
	}
}

func TestPublishScrumBoardRefreshIncludesBoardBundle(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	board := ScrumBoard{
		Columns: []string{"assigned", "in_progress"},
		Cards: []ScrumCard{
			{ID: "c1", Title: "Ship it", Column: "assigned"},
		},
	}
	server.publishScrumBoardRefresh(context.Background(), 42, "job finished", board)
	hub := server.ensureRealtimeHub()
	if hub == nil {
		t.Fatal("expected realtime hub")
	}
}
