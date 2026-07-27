package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleScrumCardTagsSuggestRequiresProjectDB(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{outputs: []string{`{"tags":[],"notes":"nothing new"}`}})
	req := httptest.NewRequest(http.MethodPost, "/v1/scrum/cards/card_1/tags-suggest?project_id=1", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
