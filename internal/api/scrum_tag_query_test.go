package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestDecodeScrumTagCatalogQueryPreservesExactSearch(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/scrum/tags?project_id=14&q=API%20client&limit=40", nil)
	query, err := decodeScrumTagCatalogQuery(request)
	if err != nil {
		t.Fatalf("decode exact Scrum tag query: %v", err)
	}
	if query.ProjectID != 14 || query.Search != "API client" || query.Limit != 40 {
		t.Fatalf("decoded query=%+v, expected exact search and limit", query)
	}
}

func TestDecodeScrumTagCatalogQueryRejectsAmbiguousOrInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		message string
	}{
		{name: "unknown", target: "/v1/scrum/tags?project_id=14&model=x", message: "unknown query field"},
		{name: "missing project", target: "/v1/scrum/tags", message: "requires project_id"},
		{name: "duplicate search", target: "/v1/scrum/tags?project_id=14&q=a&q=b", message: "exactly once"},
		{name: "duplicate project", target: "/v1/scrum/tags?project_id=14&project_id=15", message: "exactly once"},
		{name: "noncanonical project", target: "/v1/scrum/tags?project_id=014", message: "canonical positive integer"},
		{name: "zero project", target: "/v1/scrum/tags?project_id=0", message: "canonical positive integer"},
		{name: "empty limit", target: "/v1/scrum/tags?project_id=14&limit=", message: "canonical integer"},
		{name: "leading-zero limit", target: "/v1/scrum/tags?project_id=14&limit=040", message: "canonical integer"},
		{name: "zero limit", target: "/v1/scrum/tags?project_id=14&limit=0", message: "between 1 and 100"},
		{name: "large limit", target: "/v1/scrum/tags?project_id=14&limit=101", message: "between 1 and 100"},
		{name: "NUL search", target: "/v1/scrum/tags?project_id=14&q=bad%00tag", message: "NUL"},
		{name: "invalid UTF-8", target: "/v1/scrum/tags?project_id=14&q=%ff", message: "UTF-8"},
		{name: "oversized search", target: "/v1/scrum/tags?project_id=14&q=" + strings.Repeat("x", 257), message: "256-byte"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.target, nil)
			_, err := decodeScrumTagCatalogQuery(request)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v, expected message containing %q", err, test.message)
			}
		})
	}
	request := &http.Request{URL: &url.URL{Path: "/v1/scrum/tags", RawQuery: "project_id=14&q=%zz"}}
	if _, err := decodeScrumTagCatalogQuery(request); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("malformed escape error=%v, expected explicit decode failure", err)
	}
}

func TestDecodeScrumTagCatalogQueryUsesBoundedOmittedDefaultsOnly(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/scrum/tags?project_id=14", nil)
	query, err := decodeScrumTagCatalogQuery(request)
	if err != nil {
		t.Fatalf("decode omitted optional query: %v", err)
	}
	if query.ProjectID != 14 || query.Search != "" || query.Limit != 40 {
		t.Fatalf("query=%+v, expected registered omitted-field defaults", query)
	}
}

func TestScrumTagCatalogHasNoSilentQueryNormalizationFallback(t *testing.T) {
	files := map[string][]string{
		"scrum_tags.go":                 {`parsePositiveInt(`, `Query().Get("q")`, `strings.ToLower(strings.TrimSpace(query))`},
		"../queue/scrum_tag_catalog.go": {`query = strings.ToLower`, `strings.TrimSpace(query)`},
		"web/src/lib/scrum_api.ts":      {`query.trim()`, `if (limit > 0)`},
	}
	for path, forbidden := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("%s retains silent tag-query behavior %q", path, token)
			}
		}
	}
}
