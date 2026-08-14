package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrumCardDeleteRequiresExactObservedRevision(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{}`,
		`{"expected_updated_at":null}`,
		`{"expected_updated_at":"2026-08-13T08:00:00-04:00"}`,
		`{"expected_updated_at":"2026-08-13T12:00:00Z","cascade":true}`,
	} {
		request := httptest.NewRequest("DELETE", "/", strings.NewReader(body))
		if _, err := decodeScrumCardDeleteRequest(httptest.NewRecorder(), request); err == nil {
			t.Errorf("decodeScrumCardDeleteRequest(%s) unexpectedly succeeded", body)
		}
	}
	request := httptest.NewRequest("DELETE", "/", strings.NewReader(
		`{"expected_updated_at":"2026-08-13T12:00:00.123456Z"}`,
	))
	if _, err := decodeScrumCardDeleteRequest(httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
}

func TestManualScrumCardMutationQueryRejectsProjectAliases(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"/v1/scrum/cards/card-1",
		"/v1/scrum/cards/card-1?project_id=01",
		"/v1/scrum/cards/card-1?project_id=1&project_id=2",
		"/v1/scrum/cards/card-1?project_id=1&agent=cursor",
	} {
		request := httptest.NewRequest("PATCH", target, nil)
		if _, err := decodeScrumMutationProjectID(request, "Scrum card editable patch"); err == nil {
			t.Fatalf("target %q unexpectedly accepted", target)
		}
	}
}
