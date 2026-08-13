package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestParseScrumJobReference(t *testing.T) {
	scrumMeta, _ := json.Marshal(map[string]any{"source": "omni-scrum", "project_id": 1})
	if _, err := parseScrumJobReference(scrumMeta); err == nil {
		t.Fatal("omni-scrum metadata without a card must fail")
	}
	scrumMeta, _ = json.Marshal(map[string]any{"source": "omni-scrum", "project_id": 1, "scrum_card_id": "card-1"})
	ref, err := parseScrumJobReference(scrumMeta)
	if err != nil {
		t.Fatal(err)
	}
	if !ref.IsScrum || ref.ProjectID != 1 || ref.CardID != "card-1" {
		t.Fatalf("unexpected Scrum reference: %#v", ref)
	}

	llmMeta, _ := json.Marshal(map[string]any{
		"source":        "scrum_card_llm",
		"project_id":    2,
		"scrum_card_id": "card-2",
		"action":        "tags_suggest",
	})
	ref, err = parseScrumJobReference(llmMeta)
	if err != nil {
		t.Fatal(err)
	}
	if ref.IsScrum {
		t.Fatalf("removed Scrum LLM metadata was accepted: %#v", ref)
	}
	otherMeta, _ := json.Marshal(map[string]any{"source": "chat"})
	ref, err = parseScrumJobReference(otherMeta)
	if err != nil {
		t.Fatal(err)
	}
	if ref.IsScrum {
		t.Fatal("expected non-scrum job to be ignored")
	}
	for _, raw := range [][]byte{
		[]byte(`{`),
		[]byte(`{"source":42}`),
		[]byte(`{"source":"omni-scrum","project_id":"1","scrum_card_id":"card-1"}`),
	} {
		if _, err := parseScrumJobReference(raw); err == nil {
			t.Fatalf("metadata %q must fail loudly", raw)
		}
	}
}

func TestScrumAutorunHasNoMetadataProjectFallback(t *testing.T) {
	source := readAPISource(t, "scrum_play_autorun.go")
	for _, forbidden := range []string{
		"resolveJobProjectRef",
		"scrumCardIDFromJobMetadata",
		"isScrumPlayQueueJob",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum autorun contains metadata fallback %q", forbidden)
		}
	}
}

func TestScrumRequestFromContextHasURL(t *testing.T) {
	req := scrumRequestFromContext(context.Background())
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

func TestScrumRequestFromContextRejectsNilContext(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil context must fail loudly")
		}
	}()
	_ = scrumRequestFromContext(nil)
}

func TestPublishScrumBoardRefreshFailsWithoutAuthoritativeRepository(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	board := ScrumBoard{
		Columns: []string{"assigned", "in_progress"},
		Cards: []ScrumCard{
			{ID: "c1", Title: "Ship it", Column: "assigned"},
		},
	}
	if err := server.publishScrumBoardRefresh(context.Background(), 42, "job finished", board); err == nil {
		t.Fatal("board refresh without PostgreSQL must fail")
	}
	hub, err := server.requireRealtimeHub()
	if err != nil {
		t.Fatal(err)
	}
	if hub == nil {
		t.Fatal("expected realtime hub")
	}
}
