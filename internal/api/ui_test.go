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
		`data-controller="shell gx chat projects admin scrum"`,
		`data-panel-name="projects"`,
		`data-projects-target="list"`,
		`id="gx-global-loading-indicator"`,
		`data-chat-target="progress"`,
		`data-chat-target="researchStatusOutput"`,
		`data-chat-target="metricsOutput"`,
		`data-panel-name="data"`,
		`click->scrum#stopCardClick`,
		`data-chat-target="memoryList"`,
		`data-recyclr-sink="modal"`,
		"/ui/assets/",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat shell missing %q", want)
		}
	}
}

func TestUIServesStaticAssets(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUIServesBuiltBundle(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	const marker = `src="/ui/assets/`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatal("built bundle script tag missing")
	}
	rest := body[start+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("malformed script tag")
	}
	assetPath := "/ui/assets/" + rest[:end]
	req = httptest.NewRequest(http.MethodGet, assetPath, nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status=%d path=%s", rec.Code, assetPath)
	}
	bundle := rec.Body.String()
	for _, want := range []string{
		"Omni UI is ready",
		"chat#openTimelineItem",
		"renderProgress",
		"loadGlobalActivity",
		"evt_",
	} {
		if !strings.Contains(bundle, want) {
			t.Fatalf("bundle missing %q", want)
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
