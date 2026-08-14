package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestRemovedScrumInferenceActionsReturnGone(t *testing.T) {
	server := &Server{repo: &queue.Repository{}}
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(http.ResponseWriter, *http.Request)
	}{
		{name: "coach", path: "/v1/scrum/cards/card_1/coach?project_id=1", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrumCardByID(w, r)
		}},
		{name: "coach config", path: "/v1/scrum/cards/card_1/coach-config?project_id=1", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrumCardByID(w, r)
		}},
		{name: "tag inference", path: "/v1/scrum/cards/card_1/tags-suggest?project_id=1", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrumCardByID(w, r)
		}},
		{name: "project planning generation", path: "/v1/projects/1/planning-chat", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleProjectByID(w, r)
		}},
		{name: "project planning history", method: http.MethodGet, path: "/v1/projects/1/planning-chat", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleProjectByID(w, r)
		}},
		{name: "project planning draft promotion", path: "/v1/projects/1/planning-chat/drafts", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleProjectByID(w, r)
		}},
		{name: "project debugger", path: "/v1/projects/1/debugger/run", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleProjectByID(w, r)
		}},
		{name: "auto-review automation", method: http.MethodPatch, path: "/v1/scrum?project_id=1", body: `{"auto_review":{}}`, call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrum(w, r)
		}},
		{name: "create-ticket automation", method: http.MethodPatch, path: "/v1/scrum?project_id=1", body: `{"create_ticket":{}}`, call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrum(w, r)
		}},
		{name: "create-card ticket automation", path: "/v1/scrum/cards?project_id=1", body: `{"title":"card","create_ticket":true}`, call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrumCards(w, r)
		}},
		{name: "unpaginated Scrum file inventory", method: http.MethodGet, path: "/v1/scrum/files?project_id=1", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleScrumFiles(w, r)
		}},
		{name: "data source natural language", path: "/removed", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleDataSourceAsk(w, r, "source_1")
		}},
		{name: "data source exploration", path: "/removed", call: func(w http.ResponseWriter, r *http.Request) {
			server.handleDataSourceExplore(w, r, "source_1")
		}},
		{name: "public data source natural language", path: "/removed", call: func(w http.ResponseWriter, r *http.Request) {
			server.handlePublicDataSourceAsk(w, r, "source_1")
		}},
		{name: "data source channel inference", path: "/removed", call: func(w http.ResponseWriter, r *http.Request) {
			server.postDataSourceChannelMessage(w, r, "source_1", "channel_1")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			body := test.body
			if body == "" {
				body = "{}"
			}
			method := test.method
			if method == "" {
				method = http.MethodPost
			}
			request := httptest.NewRequest(method, test.path, strings.NewReader(body))
			test.call(response, request)
			if response.Code != http.StatusGone {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRemovedScrumInferenceHTTPRoutesAreGoneWithoutRepository(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil)
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "coach", method: http.MethodPost, path: "/v1/scrum/cards/card_1/coach?project_id=1", body: `{}`},
		{name: "coach config", method: http.MethodPost, path: "/v1/scrum/cards/card_1/coach-config?project_id=1", body: `{}`},
		{name: "tag inference", method: http.MethodPost, path: "/v1/scrum/cards/card_1/tags-suggest?project_id=1", body: `{}`},
		{name: "project planning generation", method: http.MethodPost, path: "/v1/projects/1/planning-chat", body: `{}`},
		{name: "project planning history", method: http.MethodGet, path: "/v1/projects/1/planning-chat"},
		{name: "project planning draft promotion", method: http.MethodPost, path: "/v1/projects/1/planning-chat/drafts", body: `{}`},
		{name: "project debugger", method: http.MethodPost, path: "/v1/projects/1/debugger/run", body: `{}`},
		{name: "board auto review", method: http.MethodPatch, path: "/v1/scrum?project_id=1", body: `{"auto_review":{}}`},
		{name: "board ticket generation", method: http.MethodPatch, path: "/v1/scrum?project_id=1", body: `{"create_ticket":{}}`},
		{name: "card ticket generation", method: http.MethodPost, path: "/v1/scrum/cards?project_id=1", body: `{"title":"card","description":"","column":"ready","create_ticket":true}`},
		{name: "card ticket generation config", method: http.MethodPost, path: "/v1/scrum/cards?project_id=1", body: `{"title":"card","description":"","column":"ready","create_ticket_config":{}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusGone {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestUnavailableRepositoryDoesNotMaskUnknownOrLiveScrumAndProjectRoutes(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "unknown card action", method: http.MethodPost, path: "/v1/scrum/cards/card_1/not-registered?project_id=1", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "malformed card path", method: http.MethodPost, path: "/v1/scrum/cards/card%20/coach?project_id=1", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "unknown project action", method: http.MethodPost, path: "/v1/projects/1/not-registered", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "malformed project path", method: http.MethodPost, path: "/v1/projects/01/debugger", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "live card action", method: http.MethodPost, path: "/v1/scrum/cards/card_1/play?project_id=1", body: `{\"expected_updated_at\":\"2026-08-13T00:00:00Z\"}`, wantStatus: http.StatusServiceUnavailable},
		{name: "live project action", method: http.MethodPost, path: "/v1/projects/1/play", body: `{}`, wantStatus: http.StatusServiceUnavailable},
		{name: "live board mutation", method: http.MethodPatch, path: "/v1/scrum?project_id=1", body: `{"auto_work":{"enabled":false,"source_columns":["assigned"]}}`, wantStatus: http.StatusServiceUnavailable},
		{name: "live card create", method: http.MethodPost, path: "/v1/scrum/cards?project_id=1", body: `{"title":"card","description":"","column":"ready"}`, wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}
