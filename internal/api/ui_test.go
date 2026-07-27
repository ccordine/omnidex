package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		`data-controller="shell recyclr chat projects scrum"`,
		`data-recyclr-scope-value="page"`,
		`data-recyclr-target="status"`,
		`data-recyclr-sink="app-panel"`,
		`Loading workspace`,
		`id="recyclr-global-loading-indicator"`,
		`data-chat-target="progress"`,
		`click->scrum#stopCardClick`,
		`data-recyclr-sink="modal"`,
		`data-shell-target="leftDrawer"`,
		`data-shell-target="rightDrawer"`,
		"/ui/assets/",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat shell missing %q", want)
		}
	}
	if strings.Contains(body, `data-controller="shell gx`) {
		t.Fatal("legacy gx controller remains on the page")
	}
	if strings.Contains(body, `href="/ui/styles.css"`) {
		t.Fatal("legacy unbundled stylesheet remains in the page shell")
	}
}

func TestLegacyUnbundledStylesheetRouteIsRemoved(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/ui/styles.css", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy stylesheet status=%d want=%d", rec.Code, http.StatusNotFound)
	}
	source, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatalf("read ui.go: %v", err)
	}
	if strings.Contains(string(source), "web/styles.css") || strings.Contains(string(source), `HandleFunc("/ui/styles.css"`) {
		t.Fatal("legacy unbundled stylesheet embed or route remains")
	}
}

func TestAdminPanelOwnsItsDeferredController(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/ui/panel?panel=admin", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(payload.HTML, `data-controller="admin"`) {
		t.Fatal("admin panel must own its deferred controller")
	}
}

func TestLegacyRealtimeSSERouteIsRemoved(t *testing.T) {
	for _, name := range []string{"server.go", "realtime_handlers.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(source), "realtime/sse") || strings.Contains(string(source), "handleRealtimeSSE") {
			t.Fatalf("legacy SSE realtime path remains in %s", name)
		}
	}
}

func TestRealtimeFrontendHasNoPollingFallbacks(t *testing.T) {
	for _, name := range []string{
		"web/src/controllers/scrum_controller.ts",
		"web/src/controllers/chat_controller.ts",
		"web/src/controllers/data_controller.ts",
		"web/src/controllers/admin_controller.ts",
		"web/src/controllers/projects_controller.ts",
	} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{
			"set" + "Interval(",
			"fetchScrum" + "Health",
			"poll" + "Job(",
			"pollDataSource" + "Job",
			"debugger" + "Poll",
			"keep " + "polling",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("polling fallback %q remains in %s", forbidden, name)
			}
		}
	}
	if _, err := os.Stat("web/src/controllers/chat_component_controller.ts"); !os.IsNotExist(err) {
		t.Fatalf("legacy chat component controller still exists: %v", err)
	}
}

func TestLegacyScrumModalAndBoardRenderFallbacksAreRemoved(t *testing.T) {
	controller, err := os.ReadFile("web/src/controllers/scrum_controller.ts")
	if err != nil {
		t.Fatalf("read scrum controller: %v", err)
	}
	for _, forbidden := range []string{
		"isReactCardModalOpen",
		"applyModalFromServer",
		"card-modal-spa-refresh",
		"data-scrum-tab-panel",
		"data-scrum-channel-messages",
		"renderScrumBoard(",
		"renderScrumColumn(",
	} {
		if strings.Contains(string(controller), forbidden) {
			t.Fatalf("legacy client-rendered Scrum path %q remains", forbidden)
		}
	}
	if _, err := os.Stat("scrum_config_render.go"); !os.IsNotExist(err) {
		t.Fatalf("legacy server HTML card-modal renderer still exists: %v", err)
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
