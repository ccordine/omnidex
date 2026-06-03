package api

import (
	"encoding/json"
	"net/url"
	"strings"
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

func TestRenderScrumBoardLiveHTML(t *testing.T) {
	html := renderScrumBoardLiveHTML(42, "job finished", map[string]any{
		"running_card_id": "card_1",
		"queued_count":    2,
	})
	if html == "" {
		t.Fatal("expected html")
	}
	for _, part := range []string{
		`data-scrum-board-refresh="42"`,
		`data-scrum-running-card="card_1"`,
		`data-scrum-queued-count="2"`,
	} {
		if !strings.Contains(html, part) {
			t.Fatalf("expected %q in html: %s", part, html)
		}
	}
}
