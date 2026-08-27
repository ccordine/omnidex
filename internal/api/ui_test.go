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
		`<link rel="icon" href="data:image/svg+xml,`,
		`data-controller="shell recyclr chat projects scrum browser-inference"`,
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

func TestChatPanelOffersFrictionlessNeutralComposerAndOptionalCreationSettings(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/panel?panel=chat", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		HTML chatComponentHTML `json:"html"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-action="chat#newThread"`,
		`New chat`,
		`>Options</summary>`,
		`data-chat-target="newChannelModeSelect"`,
		`<option value="assistant" selected>Assistant</option>`,
		`data-chat-target="newChannelDataSourceSelect"`,
		`data-recyclr-sink="new-channel-data-source-options"`,
		`>Data connection</span>`,
		`<option value="" selected>No data</option>`,
		`<option value="" disabled selected>New conversation</option>`,
		`Sending your first message creates and selects a new conversation automatically.`,
		`data-chat-target="typingIndicator"`,
		`aria-hidden="true"`,
		`Omni is responding`,
		`chat-typing-dot`,
		`data-action="input->chat#composerInput keydown->chat#slashCommandKeydown keydown->chat#composerKeydown"`,
		`Enter to send · Shift+Enter for a new line`,
	} {
		if !strings.Contains(payload.HTML.Bundle, expected) {
			t.Errorf("chat panel lacks %q: %s", expected, payload.HTML.Bundle)
		}
	}
	for _, forbidden := range []string{
		`channel-options-pagination`, `new-channel-data-source-pagination`,
		`Load more channels`, `New conversation evidence source`, `data-action="chat#createChannel"`,
		`<option value="roleplay">`, `data-chat-target="newChannelRoleplayFields"`,
		`data-recyclr-sink="roleplay-simulation"`,
	} {
		if strings.Contains(payload.HTML.Bundle, forbidden) {
			t.Errorf("chat panel retains obsolete bulky control %q: %s", forbidden, payload.HTML.Bundle)
		}
	}
}

func TestRoleplayPanelProvidesDedicatedWorldLibraryAndSimulationWorkspace(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/panel?panel=roleplay", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		HTML chatComponentHTML `json:"html"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-panel-name="roleplay"`,
		`class="chat-panel flex h-full min-h-0 flex-col`,
		`class="chat-content grid min-h-0 flex-1`,
		`class="flex min-w-0 flex-wrap items-center gap-2"`,
		`data-chat-target="roleplayPanel"`,
		`data-chat-target="roleplayLoading"`,
		`data-chat-target="roleplayWorkspaceLoading"`,
		`data-chat-target="roleplayWorldDialog"`,
		`data-chat-target="roleplayCharacterDialog"`,
		`data-chat-target="roleplaySetupDialog"`,
		`data-roleplay-dialog="worlds"`,
		`data-roleplay-dialog="characters"`,
		`data-roleplay-dialog="setup"`,
		`class="flex h-full min-h-0 min-w-0 flex-col"`,
		`data-action="chat#openRoleplayWorldBrowser"`,
		`data-action="chat#openRoleplayWorldSetup"`,
		`aria-label="Open scene and cast controls"`,
		`>Scene &amp; cast</button>`,
		`data-action="chat#openRoleplayCharacterLibrary"`,
		`data-recyclr-sink="roleplay-simulation"`,
		`data-recyclr-sink="roleplay-world-list"`,
		`data-recyclr-sink="roleplay-library-list"`,
		`data-action="submit->chat#createRoleplayWorld"`,
		`data-action="submit->chat#createRoleplayLibraryCharacter"`,
		`data-recyclr-sink="roleplay-composer-authority"`,
		`data-chat-target="roleplayDraftParts"`,
		`data-chat-target="roleplayDraftPartPool"`,
		`data-action="chat#addRoleplayDraftPart"`,
		`data-recyclr-sink="roleplay-cast-sidebar"`,
		`aria-label="Active world"`,
		`>+ Message</button>`,
		`>+ Action</button>`,
		`>+ Event</button>`,
		`aria-labelledby="roleplay-world-setup-title"`,
	} {
		if !strings.Contains(payload.HTML.Bundle, expected) {
			t.Errorf("roleplay panel lacks workspace host %q: %s", expected, payload.HTML.Bundle)
		}
	}
	if strings.Count(payload.HTML.Bundle, `data-recyclr-sink="roleplay-simulation"`) != 1 {
		t.Fatalf("roleplay panel must contain one roleplay simulation sink: %s", payload.HTML.Bundle)
	}
	composer := strings.Index(payload.HTML.Bundle, `class="chat-composer `)
	authority := strings.Index(payload.HTML.Bundle, `data-recyclr-sink="roleplay-composer-authority"`)
	input := strings.Index(payload.HTML.Bundle, `aria-label="Story message"`)
	if composer < 0 || authority <= composer || input <= authority ||
		strings.Count(payload.HTML.Bundle, `data-recyclr-sink="roleplay-composer-authority"`) != 1 {
		t.Fatalf("roleplay persona authority must render once inside the composer directly before its input: %s", payload.HTML.Bundle)
	}
	for _, obsolete := range []string{
		`data-action="chat#continueRoleplayScene"`,
		`aria-label="Worlds and character library"`,
		`aria-label="World controls"`,
		`data-roleplay-collection-dialog=`,
		`xl:grid-cols-[18rem_minmax(24rem,1fr)_22rem]`,
		`xl:grid-cols-[minmax(24rem,1fr)_22rem]`,
		`lg:grid-cols-[minmax(0,1fr)_14rem]`,
	} {
		if strings.Contains(payload.HTML.Bundle, obsolete) {
			t.Fatalf("roleplay panel keeps permanently visible collection UI %q: %s", obsolete, payload.HTML.Bundle)
		}
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
		HTML chatComponentHTML `json:"html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if strings.Contains(rec.Body.String(), `"component"`) {
		t.Fatalf("duplicate legacy panel component field remains: %s", rec.Body.String())
	}
	if !strings.Contains(payload.HTML.Bundle, `data-controller="admin"`) {
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
	var payload uiPanelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Panel != "jobs" {
		t.Fatalf("panel=%q want jobs", payload.Panel)
	}
	if !strings.Contains(payload.HTML.Bundle, `data-panel-name="jobs"`) {
		t.Fatalf("jobs fragment missing panel marker: %s", payload.HTML.Bundle)
	}
	if strings.Contains(payload.HTML.Bundle, `data-panel-name="chat"`) || strings.Contains(payload.HTML.Bundle, `data-panel-name="memory"`) {
		t.Fatalf("fragment should only contain requested panel")
	}
	if !strings.Contains(payload.HTML.Bundle, `data-recyclr-target="app-panel"`) {
		t.Fatalf("panel response lacks its server-rendered component bundle: %s", payload.HTML.Bundle)
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
		"/v1/ui/chat/channels",
		"/v1/ui/chat/data-sources",
		"/v1/ui/chat/jobs",
		"/v1/ui/chat/memory",
		"/v1/ui/chat/metrics",
		"/v1/ui/chat/roleplay",
		"/v1/ui/chat/timeline",
		"/web-research",
		"/messages?",
	} {
		if !strings.Contains(bundle, want) {
			t.Fatalf("bundle missing %q", want)
		}
	}
	for _, forbidden := range []string{"renderChatMessages", "renderJobsPanel", "renderMetricsDashboard", "chat#openTimelineItem"} {
		if strings.Contains(bundle, forbidden) {
			t.Fatalf("built bundle retains browser component authority %q", forbidden)
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
