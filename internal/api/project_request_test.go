package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestProjectCreateRequestPreservesExactDescription(t *testing.T) {
	body := `{"name":"Omnidex","location":"/srv/omnidex","description":"  exact description\nwith tab:\t "}`
	request := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(body))
	decoded, err := decodeProjectCreateRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "Omnidex" || decoded.Location != "/srv/omnidex" ||
		decoded.Description != "  exact description\nwith tab:\t " {
		t.Fatalf("decoded request changed accepted bytes: %#v", decoded)
	}
}

func TestProjectPatchRequestAcceptsOnlyTypedProjectFields(t *testing.T) {
	body := `{"expected_updated_at":"2026-08-13T12:00:00.123456Z","description":"  exact patch\n","model_config":{"conversation_response_model":"qwen"}}`
	request := httptest.NewRequest(http.MethodPatch, "/v1/projects/7", strings.NewReader(body))
	decoded, err := decodeProjectPatchRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ExpectedUpdatedAt.Format(time.RFC3339Nano) != "2026-08-13T12:00:00.123456Z" ||
		!decoded.Description.Present || decoded.Description.Value != "  exact patch\n" ||
		!decoded.ModelConfig.Present {
		t.Fatalf("decoded patch changed typed authority: %#v", decoded)
	}
}

func TestProjectDeleteRequestRequiresOneCanonicalRevision(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodDelete, "/v1/projects/7",
		strings.NewReader(`{"expected_updated_at":"2026-08-13T12:00:00.123456Z"}`),
	)
	revision, err := decodeProjectDeleteRequest(httptest.NewRecorder(), request)
	if err != nil || revision.Format(time.RFC3339Nano) != "2026-08-13T12:00:00.123456Z" {
		t.Fatalf("delete revision=%s error=%v", revision, err)
	}
	for _, body := range []string{
		`{}`, `{"expected_updated_at":null}`,
		`{"expected_updated_at":"2026-08-13T08:00:00-04:00"}`,
		`{"expected_updated_at":"2026-08-13T12:00:00Z","cascade":true}`,
		`{"expected_updated_at":"2026-08-13T12:00:00Z","expected_updated_at":"2026-08-13T12:00:01Z"}`,
	} {
		request := httptest.NewRequest(http.MethodDelete, "/v1/projects/7", strings.NewReader(body))
		if _, err := decodeProjectDeleteRequest(httptest.NewRecorder(), request); err == nil {
			t.Fatalf("invalid delete body was accepted: %s", body)
		}
	}
}

func TestProjectRequestsRejectInexactAndLegacyAuthority(t *testing.T) {
	tests := []struct {
		name  string
		patch bool
		body  string
	}{
		{name: "duplicate", body: `{"name":"one","name":"two","location":"/srv/x","description":""}`},
		{name: "unknown", body: `{"name":"one","location":"/srv/x","description":"","extra":true}`},
		{name: "inexact case", body: `{"Name":"one","location":"/srv/x","description":""}`},
		{name: "trailing JSON", body: `{"name":"one","location":"/srv/x","description":""}{}`},
		{name: "NUL", body: `{"name":"one","location":"/srv/x","description":"bad\u0000text"}`},
		{name: "null", body: `{"name":"one","location":"/srv/x","description":null}`},
		{name: "recipe id", body: `{"name":"one","location":"/srv/x","description":"","recipe_id":"legacy"}`},
		{name: "recipe body", body: `{"name":"one","location":"/srv/x","description":"","recipe":{}}`},
		{name: "patch missing revision", patch: true, body: `{"description":"changed"}`},
		{name: "patch noncanonical revision", patch: true, body: `{"expected_updated_at":"2026-08-13T08:00:00-04:00","description":"changed"}`},
		{name: "patch raw settings", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","settings":{}}`},
		{name: "patch project state", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","project_state":"existing"}`},
		{name: "patch recipe id", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","recipe_id":"legacy"}`},
		{name: "patch recipe body", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","recipe":{}}`},
		{name: "patch agent config", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","agent_config":{"agent_system":"cursor"}}`},
		{name: "patch duplicate model", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","model_config":{"conversation_response_model":"one","conversation_response_model":"two"}}`},
		{name: "patch unknown model", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","model_config":{"whole_file_agent":"forbidden"}}`},
		{name: "patch noncanonical model", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","model_config":{"conversation_response_model":" qwen "}}`},
		{name: "patch model NUL", patch: true, body: `{"expected_updated_at":"2026-08-13T12:00:00Z","model_config":{"conversation_response_model":"bad\u0000model"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method, path := http.MethodPost, "/v1/projects"
			if test.patch {
				method, path = http.MethodPatch, "/v1/projects/7"
			}
			request := httptest.NewRequest(method, path, strings.NewReader(test.body))
			var err error
			if test.patch {
				_, err = decodeProjectPatchRequest(httptest.NewRecorder(), request)
			} else {
				_, err = decodeProjectCreateRequest(httptest.NewRecorder(), request)
			}
			if err == nil {
				t.Fatalf("forbidden request was accepted: %s", test.body)
			}
		})
	}
}

func TestDecodeProjectCollectionQueryIsExactAndBounded(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/projects?limit=20&offset=40", nil)
	query, err := decodeProjectCollectionQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query.Limit != 20 || query.Offset != 40 {
		t.Fatalf("decoded query=%+v", query)
	}
	for _, raw := range []string{
		"limit=0", "limit=101", "limit=01", "limit=+1", "limit=x", "offset=-1",
		"offset=01", "offset=+1", "offset=1000001", "offset=x", "limit=20&limit=30", "unknown=1",
	} {
		t.Run(raw, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/projects?"+raw, nil)
			if _, err := decodeProjectCollectionQuery(request); err == nil {
				t.Fatalf("inexact query was accepted: %q", raw)
			}
		})
	}
}

func TestCommittedProjectMutationUsesTypedDegradedOutcome(t *testing.T) {
	response := httptest.NewRecorder()
	writeCommittedProjectMutation(response, model.Project{ID: 17, Name: "Created"}, "state survey", errors.New("survey unavailable"))
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload projectMutationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CommitState != projectMutationCommittedDegraded || payload.Project["id"] != float64(17) ||
		!strings.Contains(payload.OperationError, "was committed") {
		t.Fatalf("degraded outcome=%+v", payload)
	}
}

func TestProjectEndpointsRejectRetiredFieldsBeforeRepositoryAccess(t *testing.T) {
	server := NewServer(&queue.Repository{}, nil)
	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/v1/projects", body: `{"name":"one","location":"/srv/x","description":"","recipe_id":"legacy"}`},
		{method: http.MethodPatch, path: "/v1/projects/7", body: `{"agent_config":{"agent_system":"cursor"}}`},
		{method: http.MethodPatch, path: "/v1/projects/7", body: `{"settings":{"agent_config":{}}}`},
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}
