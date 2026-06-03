package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrumCardModalReturnsBundle(t *testing.T) {
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
	for _, want := range []string{
		`"bundle"`,
		`data-recyclr-target`,
		`"tab":"card"`,
		`data-scrum-modal-card-id`,
		`scrum#closeModal`,
		`card-ticket`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}
