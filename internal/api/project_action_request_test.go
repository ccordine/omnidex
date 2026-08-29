package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestProjectPlayPauseRequestAcceptsOnlyOneExactEmptyObject(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/v1/projects/7/play", strings.NewReader("{}"))
	if err := decodeProjectAutoWorkActionRequest(httptest.NewRecorder(), request, "project play"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		"", "null", "[]", `{"agent":"legacy"}`, `{"mode":"auto"}`,
		`{"x":1,"x":2}`, `{} {}`, "{\"x\":\"bad\u0000value\"}",
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/projects/7/play", strings.NewReader(body))
		if err := decodeProjectAutoWorkActionRequest(httptest.NewRecorder(), request, "project play"); err == nil {
			t.Fatalf("inexact project action body was accepted: %q", body)
		}
	}
}

func TestProjectPlayPauseRejectInexactBodyAndQueryBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := NewServer(&queue.Repository{}, nil)
	for _, path := range []string{
		"/v1/projects/7/play?mode=auto",
		"/v1/projects/7/pause?project_id=7",
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	for _, action := range []string{"play", "pause"} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(
			http.MethodPost, "/v1/projects/7/"+action, strings.NewReader(`{"execution_agent":"codex"}`),
		))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("action=%s status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
}

func TestProjectSurveyAndMapScanRequireExactEmptyObject(t *testing.T) {
	t.Parallel()
	server := NewServer(&queue.Repository{}, nil)
	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/v1/projects/7/survey", body: `{"planning":true}`},
		{path: "/v1/projects/7/map/scan", body: `{} {}`},
		{path: "/v1/projects/7/map/scan?mode=agent", body: `{}`},
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}
