package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleScrumCardTagsSuggestRequiresProjectDB(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{outputs: []string{`{"tags":[],"notes":"nothing new"}`}})
	if server.scrumStore == nil {
		t.Fatal("expected scrum store")
	}

	card, err := server.scrumStore.CreateCard("Tag test", "Verify queue-only suggest", "backlog")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/scrum/cards/"+card.ID+"/tags-suggest", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	server.handleScrumCardTagsSuggest(rec, req, card.ID)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
