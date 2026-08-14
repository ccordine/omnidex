package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDirectScrumBoardMutationIsRetired(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	request := httptest.NewRequest(http.MethodPut, "/v1/scrum?project_id=14", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	server.handleScrum(response, request)
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "direct Scrum board mutation is retired") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDecodeScrumAutoWorkRequestPreservesOneExactTypedConfig(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(
		"PATCH", "/v1/scrum?project_id=14",
		strings.NewReader(`{"auto_work":{"enabled":true,"source_columns":["ready","assigned"]}}`),
	)
	decoded, err := decodeScrumAutoWorkRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Enabled || len(decoded.SourceColumns) != 2 ||
		decoded.SourceColumns[0] != "ready" || decoded.SourceColumns[1] != "assigned" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestDecodeScrumAutoWorkRequestRejectsFallbackAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body, message string
	}{
		{name: "missing config", body: `{}`, message: "auto_work is required"},
		{name: "null config", body: `{"auto_work":null}`, message: "must be one object"},
		{name: "missing enabled", body: `{"auto_work":{"source_columns":["assigned"]}}`, message: "enabled is required"},
		{name: "missing columns", body: `{"auto_work":{"enabled":true}}`, message: "source_columns is required"},
		{name: "empty columns", body: `{"auto_work":{"enabled":true,"source_columns":[]}}`, message: "at least one"},
		{name: "unknown config field", body: `{"auto_work":{"enabled":true,"source_columns":["assigned"],"model":"x"}}`, message: "unknown field"},
		{name: "duplicate config field", body: `{"auto_work":{"enabled":true,"enabled":false,"source_columns":["assigned"]}}`, message: "duplicate key"},
		{name: "unknown top level", body: `{"auto_work":{"enabled":true,"source_columns":["assigned"]},"agent":"x"}`, message: "unknown field"},
		{name: "duplicate top level", body: `{"auto_work":{"enabled":true,"source_columns":["assigned"]},"auto_work":{"enabled":false,"source_columns":["assigned"]}}`, message: "duplicate key"},
		{name: "column alias", body: `{"auto_work":{"enabled":true,"source_columns":[" assigned"]}}`, message: "not registered"},
		{name: "duplicate column", body: `{"auto_work":{"enabled":true,"source_columns":["assigned","assigned"]}}`, message: "duplicate"},
		{name: "trailing", body: `{"auto_work":{"enabled":true,"source_columns":["assigned"]}} {}`, message: "trailing"},
		{name: "removed auto review", body: `{"auto_review":{}}`, message: "removed Scrum auto-review"},
		{name: "removed ticket", body: `{"create_ticket":{}}`, message: "removed Scrum create-ticket"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("PATCH", "/v1/scrum?project_id=14", strings.NewReader(test.body))
			_, err := decodeScrumAutoWorkRequest(httptest.NewRecorder(), request)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want containing %q", err, test.message)
			}
		})
	}
}

func TestDecodeScrumAutoWorkProjectQueryIsExact(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("PATCH", "/v1/scrum?project_id=14", nil)
	projectID, err := decodeScrumMutationProjectID(request, "Scrum auto-work request")
	if err != nil || projectID != 14 {
		t.Fatalf("projectID=%d error=%v", projectID, err)
	}
	for _, target := range []string{
		"/v1/scrum",
		"/v1/scrum?project_id=%2B14",
		"/v1/scrum?project_id=14&project_id=15",
		"/v1/scrum?project_id=14&mode=agent",
	} {
		request := httptest.NewRequest("PATCH", target, nil)
		if _, err := decodeScrumMutationProjectID(request, "Scrum auto-work request"); err == nil {
			t.Fatalf("target %q unexpectedly accepted", target)
		}
	}
}
