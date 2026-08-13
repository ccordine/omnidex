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
