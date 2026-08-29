package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProductionFrontendComponentRenderersAreServerOwned(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"web/src/lib/admin_render.ts",
		"web/src/lib/agent_config_render.ts",
		"web/src/lib/channel_render.ts",
		"web/src/lib/data_render.ts",
		"web/src/lib/data_sources_render.ts",
		"web/src/lib/model_config_render.ts",
		"web/src/lib/project_git_render.ts",
		"web/src/lib/project_render.ts",
		"web/src/lib/screen_render.ts",
		"web/src/lib/scrum_modal_render.ts",
		"web/src/lib/scrum_render.ts",
		"web/src/lib/terminal_render.ts",
		"web/src/lib/data_chart.ts",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("browser component renderer remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", path, err)
		}
	}
}

func TestOperationalAdminComponentIsServerRendered(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/admin?tab=health", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var component uiOperationalComponent
	if err := json.Unmarshal(response.Body.Bytes(), &component); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`data-recyclr-target="admin-tab-panel"`, "Core health"} {
		if !strings.Contains(component.HTML.Bundle, required) {
			t.Errorf("server admin component is missing %q", required)
		}
	}
}

func TestOperationalAdminComponentRejectsUnknownTab(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/admin?tab=legacy", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestServerRenderersPreserveBoundedInteractionInputs(t *testing.T) {
	modal := renderUIScrumCreateCard("ready")
	if !strings.Contains(modal, `<option value="ready" selected>`) || !strings.Contains(modal, `data-action="submit->scrum#createCard"`) {
		t.Fatalf("create-card component lacks its bounded column or typed interaction: %s", modal)
	}
	fields := []uiProjectModelField{{
		Key: "coding_fragment_model", Label: "Coding fragment model",
		Value: "qwen3-coder:30b", Options: []string{"qwen3-coder:30b", "qwen2.5-coder:14b"},
	}}
	settings := uiProjectConfigSection(
		7, time.Date(2026, 8, 13, 12, 0, 0, 123456000, time.UTC),
		"Model overrides", "project-model", "projects#saveModelConfig", "projects#clearModelConfig",
		fields, map[string]string{"coding_fragment_model": "qwen2.5-coder:14b"},
	)
	if !strings.Contains(settings, `<option value="qwen2.5-coder:14b" selected>`) || strings.Contains(settings, `<option value="qwen3-coder:30b" selected>`) {
		t.Fatalf("project component did not distinguish explicit override from inherited value: %s", settings)
	}
	if strings.Count(settings, `data-project-id="7"`) != 2 {
		t.Fatalf("project configuration mutation controls lack exact server project authority: %s", settings)
	}
	if strings.Count(settings, `data-project-updated-at="2026-08-13T12:00:00.123456Z"`) != 2 {
		t.Fatalf("project configuration controls lack exact server revision authority: %s", settings)
	}
}

func TestServerDataPaginationControlsAreExplicit(t *testing.T) {
	if controls := renderUIDataPagination("data#loadDataPage", "source", 0, 20, false); controls != "" {
		t.Fatalf("terminal page rendered controls: %s", controls)
	}
	controls := renderUIDataPagination("data#loadDataPage", "source", 20, 20, true)
	for _, required := range []string{`data-page-offset="0"`, `data-page-offset="40"`, `data-page-kind="source"`} {
		if !strings.Contains(controls, required) {
			t.Errorf("pagination controls are missing %q: %s", required, controls)
		}
	}
}

func TestProductionFrontendDoesNotConstructComponentMarkup(t *testing.T) {
	t.Parallel()
	var paths []string
	err := filepath.WalkDir("web/src", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == "web/src/react" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx") {
			continue
		}
		if strings.HasSuffix(path, "card_modal_spa_controller.tsx") || strings.HasSuffix(path, "scrum_card_modal_host.ts") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"insertAdjacentHTML(",
			"buildRecyclrBundle(",
			"renderRecyclrBundle(",
			"<div", "<section", "<article", "<form", "<button", "<input", "<select", "<option", "<table", "<template",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("production browser component construction %q remains in %s", forbidden, path)
			}
		}
	}
}

func TestOperationalControllersRequireServerComponentBundles(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"web/src/controllers/admin_controller.ts":              {"fetchAdminComponent(", "renderServerBundle("},
		"web/src/controllers/admin_data_sources_controller.ts": {"fetchAdminDataSourcesComponent(", "renderServerBundle("},
		"web/src/controllers/data_controller.ts":               {"fetchDataComponent(", "renderServerBundle("},
		"web/src/controllers/projects_controller.ts":           {"fetchProjectsComponent(", "fetchProjectDetailComponent(", "renderServerBundle("},
		"web/src/controllers/screen_controller.ts":             {"fetchScreenMonitorsComponent(", "renderServerBundle("},
	}
	for path, required := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range required {
			if !strings.Contains(string(raw), token) {
				t.Errorf("%s does not require server component path %q", path, token)
			}
		}
	}
}

func TestRecyclrClientAcceptsServerBundlesOnly(t *testing.T) {
	t.Parallel()
	path := "web/src/lib/recyclr.ts"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{"buildRecyclrBundle", "template.innerHTML", "template.outerHTML"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("generic client Recyclr HTML construction %q remains", forbidden)
		}
	}
}

func TestGrowingOperationalCatalogsHaveNoUnboundedProductionLoader(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"../queue/projects.go":                  {"ListScrumCards("},
		"scrum_card_pages.go":                   {"loadAllScrumCards", "for page.HasMore"},
		"scrum_global_autowork.go":              {"ListProjects(", "sort.SliceStable(candidates"},
		"scrum_reorder.go":                      {"placeScrumCard(", "board.Cards"},
		"../queue/data_source_channels.go":      {"ListDataSourceChannelMessages("},
		"../ollama/client.go":                   {"ListModels("},
		"../ollama/model_page.go":               {"json.Unmarshal("},
		"../queue/scrum_card_messages.go":       {"jsonb_array_elements"},
		"scrum_channel_chat.go":                 {"displayScrumChannelMessages", "hydrateCardChannelChat", "sortScrumChatChronological"},
		"scrum_channel_presentation.go":         {"displayScrumChannelMessages", "hydrateCardChannelChat", "sortScrumChatChronological"},
		"scrum_modal_context.go":                {"refreshScrumPlayQueue("},
		"scrum_service.go":                      {"refreshScrumPlayQueue("},
		"web/src/lib/scrum_api.ts":              {"fetchScrumFiles", "fetchScrumCard(", "fetchScrumCardPayload"},
		"web/src/react/card-modal/FilesTab.tsx": {"fetchScrumCardModal"},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("unbounded production loader %q remains in %s", token, path)
			}
		}
	}
}

func TestReactCardItemsCannotOwnDurableRowsOrIdentities(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"web/src/react/card-modal/CardTab.tsx",
		"web/src/react/card-modal/TestsTab.tsx",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		if !strings.Contains(source, "mutateScrumCardItem(") {
			t.Errorf("%s does not use the typed server-owned item mutation", path)
		}
		for _, forbidden := range []string{"Date.now()", "patchChecklist(", "patchTests(", "{ checklist:", "{ test_criteria:"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("client durable item authority %q remains in %s", forbidden, path)
			}
		}
	}
	raw, err := os.ReadFile("web/src/lib/scrum_api.ts")
	if err != nil {
		t.Fatal(err)
	}
	edit := string(raw)
	start := strings.Index(edit, "export type ScrumCardEdit")
	end := strings.Index(edit, "export type ScrumCardItemMutation")
	if start < 0 || end <= start {
		t.Fatal("Scrum card edit and item mutation types are missing")
	}
	for _, forbidden := range []string{`"checklist"`, `"test_criteria"`} {
		if strings.Contains(edit[start:end], forbidden) {
			t.Errorf("generic Scrum card patch still grants %s authority", forbidden)
		}
	}
}

func TestScrumModalTabLoadersDoNotCrossFetchOperationalState(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("scrum_modal_context.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	checks := map[string]struct {
		required  string
		forbidden []string
	}{
		"populateScrumModalChannelContext": {"scrumChannelPage(", []string{"resolvedModelsForProject", "LoadRecipePage", "populateScrumModalFileContext"}},
	}
	for name, check := range checks {
		body := exactSourceFunction(t, source, name)
		if !strings.Contains(body, check.required) {
			t.Errorf("%s does not load its exact typed authority %q", name, check.required)
		}
		for _, forbidden := range check.forbidden {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s cross-fetches unrelated modal authority %q", name, forbidden)
			}
		}
	}
}

func exactSourceFunction(t *testing.T, source, name string) string {
	t.Helper()
	start := strings.Index(source, "func "+name+"(")
	if start < 0 {
		start = strings.Index(source, "func (s *Server) "+name+"(")
	}
	if start < 0 {
		t.Fatalf("function %s is missing", name)
	}
	end := strings.Index(source[start+1:], "\nfunc ")
	if end < 0 {
		return source[start:]
	}
	return source[start : start+1+end]
}
