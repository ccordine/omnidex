package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeScrumBoardQueryAcceptsOnlyExactRegisteredViewport(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(
		"GET", "/v1/scrum?project_id=14&column=in_progress&card_offset=40", nil,
	)
	query, err := decodeScrumBoardQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query.ProjectID != 14 || query.Column != "in_progress" || query.CardOffset != 40 {
		t.Fatalf("decoded query=%+v", query)
	}

	defaults := httptest.NewRequest("GET", "/v1/scrum?project_id=14", nil)
	query, err = decodeScrumBoardQuery(defaults)
	if err != nil {
		t.Fatal(err)
	}
	if query.Column != "assigned" || query.CardOffset != 0 {
		t.Fatalf("default query=%+v", query)
	}
}

func TestScrumBoardGETRejectsInexactViewportBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	request := httptest.NewRequest(
		http.MethodGet, "/v1/scrum?project_id=14&column=ready&column=done", nil,
	)
	response := httptest.NewRecorder()
	server.handleScrum(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "exactly once") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDecodeScrumBoardQueryRejectsAliasesAndUnboundedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		target  string
		message string
	}{
		{name: "missing project", target: "/v1/scrum", message: "requires project_id"},
		{name: "unknown", target: "/v1/scrum?project_id=14&model=x", message: "unknown query field"},
		{name: "duplicate project", target: "/v1/scrum?project_id=14&project_id=15", message: "exactly once"},
		{name: "noncanonical project", target: "/v1/scrum?project_id=014", message: "canonical positive integer"},
		{name: "positive alias", target: "/v1/scrum?project_id=%2B14", message: "canonical positive integer"},
		{name: "zero project", target: "/v1/scrum?project_id=0", message: "canonical positive integer"},
		{name: "empty column", target: "/v1/scrum?project_id=14&column=", message: "not registered"},
		{name: "column whitespace", target: "/v1/scrum?project_id=14&column=%20assigned", message: "not registered"},
		{name: "unknown column", target: "/v1/scrum?project_id=14&column=todo", message: "not registered"},
		{name: "duplicate column", target: "/v1/scrum?project_id=14&column=ready&column=done", message: "exactly once"},
		{name: "empty offset", target: "/v1/scrum?project_id=14&card_offset=", message: "canonical integer"},
		{name: "leading-zero offset", target: "/v1/scrum?project_id=14&card_offset=040", message: "canonical integer"},
		{name: "negative offset", target: "/v1/scrum?project_id=14&card_offset=-1", message: "between 0"},
		{name: "large offset", target: "/v1/scrum?project_id=14&card_offset=1000001", message: "between 0"},
		{name: "duplicate offset", target: "/v1/scrum?project_id=14&card_offset=1&card_offset=2", message: "exactly once"},
		{name: "oversized raw query", target: "/v1/scrum?project_id=14&" + strings.Repeat("x", 4096), message: "4096-byte"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.target, nil)
			_, err := decodeScrumBoardQuery(request)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want containing %q", err, test.message)
			}
		})
	}
}
