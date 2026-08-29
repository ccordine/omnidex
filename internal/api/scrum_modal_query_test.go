package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeScrumModalQueryAcceptsOnlyExactTypedState(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		projectID  int64
		tab        scrumModalTab
		filePath   string
		fileOffset int
	}{
		{name: "omitted tab is the one default", target: "/modal?project_id=14", projectID: 14, tab: scrumModalTabCard},
		{name: "explicit root", target: "/modal?project_id=14&tab=files&file_path=&file_offset=0", projectID: 14, tab: scrumModalTabFiles},
		{name: "exact relative path bytes", target: "/modal?project_id=14&tab=files&file_path=%20pkg%20&file_offset=50", projectID: 14, tab: scrumModalTabFiles, filePath: " pkg ", fileOffset: 50},
		{name: "channel", target: "/modal?project_id=14&tab=channel", projectID: 14, tab: scrumModalTabChannel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			query, err := decodeScrumModalQuery(request)
			if err != nil {
				t.Fatal(err)
			}
			if query.ProjectID != test.projectID || query.Tab != test.tab ||
				query.FilePath != test.filePath || query.FileOffset != test.fileOffset {
				t.Fatalf("query=%+v", query)
			}
		})
	}
}

func TestScrumModalAndFileEndpointsRejectInexactQueryBeforeRepositoryAccess(t *testing.T) {
	server := &Server{repo: &queue.Repository{}}
	for _, target := range []string{
		"/v1/scrum/cards/card-7/modal?project_id=14&tab=FILES&file_path=",
		"/v1/scrum/cards/card-7/files?project_id=14&file_path=&file_offset=00",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		server.handleScrumCardByID(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%q status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}

func TestDecodeScrumModalQueryRejectsAliasesAmbiguityAndMalformedValues(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "missing project", target: "/modal?tab=card"},
		{name: "noncanonical project", target: "/modal?project_id=014"},
		{name: "duplicate project", target: "/modal?project_id=14&project_id=15"},
		{name: "explicit empty tab", target: "/modal?project_id=14&tab="},
		{name: "case alias", target: "/modal?project_id=14&tab=FILES&file_path="},
		{name: "whitespace alias", target: "/modal?project_id=14&tab=%20files&file_path="},
		{name: "retired tab", target: "/modal?project_id=14&tab=config"},
		{name: "duplicate tab", target: "/modal?project_id=14&tab=card&tab=files"},
		{name: "unknown field", target: "/modal?project_id=14&mode=files"},
		{name: "files without explicit path", target: "/modal?project_id=14&tab=files"},
		{name: "path on card tab", target: "/modal?project_id=14&tab=card&file_path="},
		{name: "offset on card tab", target: "/modal?project_id=14&tab=card&file_offset=0"},
		{name: "invalid utf8 path", target: "/modal?project_id=14&tab=files&file_path=%FF"},
		{name: "NUL path", target: "/modal?project_id=14&tab=files&file_path=pkg%00file"},
		{name: "absolute path", target: "/modal?project_id=14&tab=files&file_path=%2Fworkspace"},
		{name: "dot path", target: "/modal?project_id=14&tab=files&file_path=pkg%2F..%2Fother"},
		{name: "inexact offset", target: "/modal?project_id=14&tab=files&file_path=&file_offset=01"},
		{name: "negative offset", target: "/modal?project_id=14&tab=files&file_path=&file_offset=-1"},
		{name: "oversized offset", target: "/modal?project_id=14&tab=files&file_path=&file_offset=1000001"},
		{name: "oversized query", target: "/modal?project_id=14&tab=files&file_path=" + strings.Repeat("x", maxScrumModalRawQuerySize)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			if _, err := decodeScrumModalQuery(request); err == nil {
				t.Fatalf("accepted inexact query %q", request.URL.RawQuery)
			}
		})
	}
}

func TestDecodeScrumFilePageQueryRequiresExplicitRootAndOffset(t *testing.T) {
	accepted := httptest.NewRequest(
		http.MethodGet, "/files?project_id=14&file_path=&file_offset=0", nil,
	)
	query, err := decodeScrumFilePageQuery(accepted)
	if err != nil {
		t.Fatal(err)
	}
	if query.ProjectID != 14 || query.FilePath != "" || query.FileOffset != 0 {
		t.Fatalf("query=%+v", query)
	}

	for _, target := range []string{
		"/files?project_id=14&file_offset=0",
		"/files?project_id=14&file_path=",
		"/files?project_id=14&file_path=&file_offset=0&tab=files",
		"/files?project_id=14&file_path=&file_offset=+0",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if _, err := decodeScrumFilePageQuery(request); err == nil {
			t.Fatalf("accepted inexact file-page query %q", request.URL.RawQuery)
		}
	}
}
