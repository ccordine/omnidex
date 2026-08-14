package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDecodeScrumChannelQueryAcceptsOnlyExactTypedFields(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/scrum/cards/card/channel?project_id=14&limit=25&before=scrumchat_v1_a", nil)
	query, err := decodeScrumChannelQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query.ProjectID != 14 || query.Limit != 25 || query.Before != "scrumchat_v1_a" {
		t.Fatalf("query=%+v", query)
	}
	omitted := httptest.NewRequest("GET", "/v1/scrum/cards/card/channel?project_id=14", nil)
	query, err = decodeScrumChannelQuery(omitted)
	if err != nil || query.Limit != scrumChannelDefaultPageSize || query.Before != "" {
		t.Fatalf("omitted query=%+v error=%v", query, err)
	}
}

func TestDecodeScrumChannelQueryRejectsAmbiguousOrForbiddenInput(t *testing.T) {
	tests := []struct{ name, target, message string }{
		{"unknown", "/channel?project_id=14&model=x", "unknown query field"},
		{"duplicate", "/channel?project_id=14&limit=2&limit=3", "exactly once"},
		{"missing project", "/channel?limit=2", "requires project_id"},
		{"project alias", "/channel?project_id=014", "canonical positive"},
		{"limit alias", "/channel?project_id=14&limit=025", "canonical integer"},
		{"blank before", "/channel?project_id=14&before=", "malformed"},
		{"before alias", "/channel?project_id=14&before=scrumchat_v1_01", "malformed"},
		{"oversized", "/channel?project_id=14&tool=" + strings.Repeat("x", maxScrumChannelRawQuerySize), "byte bound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeScrumChannelQuery(httptest.NewRequest("GET", test.target, nil))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want %q", err, test.message)
			}
		})
	}
	request := &http.Request{URL: &url.URL{Path: "/channel", RawQuery: "project_id=14&before=%zz"}}
	if _, err := decodeScrumChannelQuery(request); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("malformed escape error=%v", err)
	}
}
