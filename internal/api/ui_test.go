package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIServesChatShell(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Omni Chat",
		"tailwindcss.com",
		`data-controller="gx chat"`,
		`id="gx-global-loading-indicator"`,
		`data-chat-target="progress"`,
		`data-chat-target="researchStatusOutput"`,
		`data-chat-target="metricsOutput"`,
		`data-recyclr-sink="modal"`,
		"/ui/app.js",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat shell missing %q", want)
		}
	}
}

func TestUIServesStaticAssets(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Application.start",
		"recyclrjs",
		"GXController",
		"data-recyclr-target",
		"openTimelineItem",
		"renderProgress",
		"renderResearchStatus",
		"renderMetricsDashboard",
		"evt_",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("app.js missing %q", want)
		}
	}
}

func TestUIRouteDoesNotMaskAPINotFound(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusNotFound)
	}
}
