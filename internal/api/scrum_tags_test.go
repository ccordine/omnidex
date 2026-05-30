package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleScrumCardTagsSuggestEmptyIncludesCard(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{`{"tags":[],"notes":"nothing new"}`}}
	server := NewServer(nil, llmClient)
	if server.scrumStore == nil {
		t.Fatal("expected scrum store")
	}

	card, err := server.scrumStore.CreateCard("Tag test", "Verify empty suggest response", "backlog")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/scrum/cards/"+card.ID+"/tags-suggest", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	server.handleScrumCardTagsSuggest(rec, req, card.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["card"]; !ok {
		t.Fatalf("response missing card: %s", rec.Body.String())
	}
	var returned ScrumCard
	if err := json.Unmarshal(payload["card"], &returned); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if returned.ID != card.ID {
		t.Fatalf("card id=%q want %q", returned.ID, card.ID)
	}
}
