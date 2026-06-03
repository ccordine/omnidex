package api

import (
	"encoding/json"
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
		`href="/ui/styles.css"`,
		`data-controller="shell gx chat projects admin scrum"`,
		`data-panel-name="projects"`,
		`data-projects-target="list"`,
		`id="gx-global-loading-indicator"`,
		`data-chat-target="progress"`,
		`data-admin-target="tabPanel"`,
		`data-recyclr-sink="admin-tab-panel"`,
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

func TestUIPanelReturnsSingleServerFragment(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/ui/panel?panel=jobs", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Panel string `json:"panel"`
		HTML  string `json:"html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Panel != "jobs" {
		t.Fatalf("panel=%q want jobs", payload.Panel)
	}
	if !strings.Contains(payload.HTML, `data-panel-name="jobs"`) {
		t.Fatalf("jobs fragment missing panel marker: %s", payload.HTML)
	}
	if strings.Contains(payload.HTML, `data-panel-name="chat"`) || strings.Contains(payload.HTML, `data-panel-name="memory"`) {
		t.Fatalf("fragment should only contain requested panel")
	}
	if rec.Result().Cookies()[0].Name != uiSessionCookieName {
		t.Fatalf("ui session cookie missing")
	}
}

func TestUISessionMergesQueryStateAndPatch(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/ui/session?panel=projects&scrum_column=todo", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload uiSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.State["panel"] != "projects" || payload.State["scrum_column"] != "todo" {
		t.Fatalf("unexpected state: %#v", payload.State)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("ui session cookie missing")
	}

	req = httptest.NewRequest(http.MethodPatch, "/v1/ui/session", strings.NewReader(`{"state":{"panel":"metrics","admin_tab":"health"}}`))
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid patch JSON: %v", err)
	}
	if payload.State["panel"] != "metrics" || payload.State["admin_tab"] != "health" {
		t.Fatalf("unexpected patched state: %#v", payload.State)
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
