package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnboundedScrumCardGetIsRetired(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/cards/card_1?project_id=1", nil)
	response := httptest.NewRecorder()
	(&Server{}).handleScrumCardByID(response, request)
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "bounded card modal") {
		t.Fatalf("retirement response does not identify the authoritative replacement: %s", response.Body.String())
	}
}
