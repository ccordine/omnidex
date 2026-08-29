package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeScrumCardCreateRequestCanonicalizesTitleAndPreservesDescription(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(
		"POST", "/v1/scrum/cards?project_id=14",
		strings.NewReader(`{"title":" Exact card ","description":"  Preserve me.\n","column":"ready"}`),
	)
	decoded, err := decodeScrumCardCreateRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Title != "Exact card" || decoded.Description != "  Preserve me.\n" || decoded.Column != "ready" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestDecodeScrumCardCreateProjectQueryRejectsAliases(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"/v1/scrum/cards",
		"/v1/scrum/cards?project_id=014",
		"/v1/scrum/cards?project_id=14&project_id=15",
		"/v1/scrum/cards?project_id=14&agent=x",
	} {
		request := httptest.NewRequest("POST", target, nil)
		if _, err := decodeScrumMutationProjectID(request, "Scrum card create request"); err == nil {
			t.Fatalf("target %q unexpectedly accepted", target)
		}
	}
}

func TestDecodeScrumCardCreateRequestRejectsImplicitAndRetiredAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body, message string
	}{
		{name: "missing title", body: `{"description":"x","column":"ready"}`, message: "title is required"},
		{name: "blank title", body: `{"title":"  ","description":"x","column":"ready"}`, message: "must not be blank"},
		{name: "missing description", body: `{"title":"x","column":"ready"}`, message: "description is required"},
		{name: "missing column", body: `{"title":"x","description":""}`, message: "column is required"},
		{name: "column alias", body: `{"title":"x","description":"","column":" Ready"}`, message: "not registered"},
		{name: "unknown", body: `{"title":"x","description":"","column":"ready","model":"x"}`, message: "unknown field"},
		{name: "duplicate", body: `{"title":"x","title":"y","description":"","column":"ready"}`, message: "duplicate key"},
		{name: "trailing", body: `{"title":"x","description":"","column":"ready"} {}`, message: "trailing"},
		{name: "removed ticket", body: `{"title":"x","description":"","column":"ready","create_ticket":true}`, message: "removed Scrum card ticket"},
		{name: "removed config", body: `{"title":"x","description":"","column":"ready","create_ticket_config":{}}`, message: "removed Scrum card ticket"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/v1/scrum/cards?project_id=14", strings.NewReader(test.body))
			_, err := decodeScrumCardCreateRequest(httptest.NewRecorder(), request)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want containing %q", err, test.message)
			}
		})
	}
}
