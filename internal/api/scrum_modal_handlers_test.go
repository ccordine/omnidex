package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrumCardModalReturnsTypedContextWithoutBundle(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	if server.scrumStore == nil {
		t.Fatal("expected scrum store")
	}
	card, err := server.scrumStore.CreateCard("Modal test", "Verify server modal HTML", "backlog")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/"+card.ID+"/modal?tab=card", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"bundle"`) || strings.Contains(body, `data-recyclr-target`) || strings.Contains(body, `data-scrum-modal-card-id`) {
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
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
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
	if payload.PlayQueue == nil {
		t.Fatal("expected play queue context")
	}
}
