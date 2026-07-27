package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScrumCardModalPayloadIsTypedAndContainsNoHTMLBundle(t *testing.T) {
	card := ScrumCard{ID: "card_1", Title: "Modal test"}
	payloadMap := scrumModalPayload(&scrumModalRenderContext{
		Card:      card,
		Board:     ScrumBoard{ID: "project_1", Cards: []ScrumCard{}},
		Tab:       "card",
		PlayQueue: map[string]any{"queued_count": 0},
	})
	body, err := json.Marshal(payloadMap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"bundle"`) || strings.Contains(string(body), `data-recyclr-target`) || strings.Contains(string(body), `data-scrum-modal-card-id`) {
		t.Fatalf("modal context must not include legacy HTML bundle: %s", body)
	}
	var payload struct {
		Card struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"card"`
		Board       ScrumBoard         `json:"board"`
		Tab         string             `json:"tab"`
		Files       []string           `json:"files"`
		PlayQueue   map[string]any     `json:"play_queue"`
		ModelFields []scrumConfigField `json:"model_fields"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Card.ID != card.ID || payload.Card.Title != "Modal test" {
		t.Fatalf("unexpected card payload: %#v", payload.Card)
	}
	if payload.Tab != "card" {
		t.Fatalf("tab=%q want card", payload.Tab)
	}
	if payload.Board.ID == "" {
		t.Fatalf("expected board context: %#v", payload.Board)
	}
	if len(payload.Board.Cards) != 0 {
		t.Fatalf("modal payload must not embed the full board card list: %d cards", len(payload.Board.Cards))
	}
	if payload.PlayQueue == nil {
		t.Fatal("expected play queue context")
	}
}
