package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleScrumCardTagsSuggestIsGoneWithoutProjectDB(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodPost, "/v1/scrum/cards/card_1/tags-suggest?project_id=1", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
