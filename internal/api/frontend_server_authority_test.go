package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/admin?tab=advanced", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var component uiOperationalComponent
	if err := json.Unmarshal(response.Body.Bytes(), &component); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`data-recyclr-target="admin-tab-panel"`, "Destructive maintenance"} {
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
	fields := []map[string]any{{"key": "agent_system", "label": "Agent", "value": "omnidex", "options": []string{"omnidex", "codex"}}}
	settings := uiProjectConfigSection("Agent overrides", "project-agent", "projects#saveAgentConfig", "projects#clearAgentConfig", fields, map[string]string{"agent_system": "codex"})
	if !strings.Contains(settings, `<option value="codex" selected>`) || strings.Contains(settings, `<option value="omnidex" selected>`) {
		t.Fatalf("project component did not distinguish explicit override from inherited value: %s", settings)
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
		"../queue/projects.go":             {"ListScrumCards("},
		"../queue/data_source_channels.go": {"ListDataSourceChannelMessages("},
		"../ollama/client.go":              {"ListModels("},
		"../ollama/model_page.go":          {"json.Unmarshal("},
		"../omni/recipe.go":                {"func LoadRecipes(", "os.ReadDir(root)"},
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
