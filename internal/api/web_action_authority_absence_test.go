package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestCheckedInRecipeActionSubsystemIsAbsent(t *testing.T) {
	for _, path := range []string{
		"../../recipes/frontend.stimulus-tailwind-recyclr.json",
		"../../docs/RECIPES.md",
		"../omni/recipe.go",
		"../omni/recipe_page.go",
		"recipe_handlers.go",
		"web/src/react/card-modal/RecipeTab.tsx",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired checked-in recipe surface remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired recipe surface %s: %v", path, err)
		}
	}
	if entries, err := os.ReadDir("../../recipes"); err == nil {
		if len(entries) != 0 {
			t.Errorf("checked-in recipe directory contains %d retired manifests", len(entries))
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect checked-in recipe directory: %v", err)
	}

	server := NewServer(nil, nil)
	for _, path := range []string{"/v1/recipes", "/v1/recipes/frontend.stimulus-tailwind-recyclr"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("retired recipe route %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	for path, forbidden := range map[string][]string{
		"server.go":                                   {`HandleFunc("/v1/recipes`},
		"ui_project_modal_component.go":               {"Recipe", "recipe_id", "LoadRecipe"},
		"ui_project_detail_component.go":              {`"recipe"`, `{"recipe", "Recipe"}`},
		"ui_project_tabs.go":                          {`case "recipe"`, "renderUIProjectRecipe"},
		"ui_project_inspection_component.go":          {"renderUIProjectRecipe", "LoadRecipe"},
		"web/src/controllers/projects_controller.ts":  {`"recipe"`, "saveRecipe", "loadRecipePage", "loadCreateRecipePage"},
		"web/src/lib/project_api.ts":                  {"/v1/recipes", "RecipeCatalogItem", "recipe_id", "recipe:"},
		"web/src/lib/project_browser_coordinator.ts":  {"createRecipe", "recipe_id", "loadCreateRecipePage"},
		"web/src/lib/project_mutation_coordinator.ts": {"saveRecipe", "recipeJson", "recipeId"},
		"web/src/lib/project_types.ts":                {"RecipeCatalogItem", "recipe_id", "recipe?:"},
		"web/src/lib/panel_routing.ts":                {`"recipe"`},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains retired checked-in recipe authority %q", path, fragment)
			}
		}
	}
	routingSource := readFrontendOrAPISource(t, "../modelconfig/routing.go")
	for _, fragment := range []string{"func Resolve(", "func ResolveRouting("} {
		if strings.Contains(routingSource, fragment) {
			t.Errorf("model routing retains dead card-precedence authority %q", fragment)
		}
	}
}

func TestScrumSurfacesHaveNoAgentSelectionAuthority(t *testing.T) {
	if _, err := os.Stat("agent_config_service.go"); err == nil {
		t.Fatal("retired agent-configuration API runtime remains")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect retired agent-configuration API runtime: %v", err)
	}
	server := NewServer(nil, nil)
	for _, path := range []string{"/v1/settings/agents", "/v1/agents/resolved"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("retired agent-selection route %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	serverSource := readFrontendOrAPISource(t, "server.go")
	for _, forbidden := range []string{`HandleFunc("/v1/settings/agents`, `HandleFunc("/v1/agents/resolved`} {
		if strings.Contains(serverSource, forbidden) {
			t.Errorf("server retains retired agent-selection route %q", forbidden)
		}
	}
	for path, forbidden := range map[string][]string{
		"web/index.html":                              {"agent cockpit"},
		"web/panels/admin.html":                       {"models, agents", "Models &amp; agents"},
		"web/locales/en.json":                         {"models, agents", "Models & agents", "transport.direct", "transport.queue"},
		"web/locales/es.json":                         {"modelos, agentes", "Modelos y agentes"},
		"web/locales/ja.json":                         {"エージェント"},
		"web/locales/ru.json":                         {"агенты"},
		"web/locales/zh-Hans.json":                    {"智能体"},
		"ui_admin_settings_component.go":              {"renderUIAgentFields", "saveGlobalAgents", "workspace agent settings"},
		"ui_project_settings_component.go":            {"Project agent overrides", "projectAgentConfig", "resolvedAgentsForProjectCard"},
		"web/src/controllers/projects_controller.ts":  {"saveAgentConfig", "clearAgentConfig"},
		"web/src/lib/project_mutation_coordinator.ts": {"saveAgentConfig", "clearAgentConfig", `"agent_config"`},
		"web/src/lib/project_api.ts":                  {"agent_config:"},
		"web/src/lib/project_types.ts":                {"agent_config?:"},
		"web/src/lib/scrum_api.ts":                    {"agentConfig", `body.agent_config`},
		"web/src/lib/scrum_types.ts":                  {"agent_config?:", "agent_fields?:", "agent_system?:", "agent_overrides?:"},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains Scrum agent-selection authority %q", path, fragment)
			}
		}
	}
}

func TestWebScrumRuntimeHasNoExternalAgentStreamOrEventDictionary(t *testing.T) {
	for _, path := range []string{
		"scrum_agent_stream.go",
		"scrum_external_agent_outcome.go",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired Scrum external-agent runtime remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired Scrum external-agent runtime %s: %v", path, err)
		}
	}
	for path, forbidden := range map[string][]string{
		"scrum_channel_activity.go":       {`strings.Contains(eventType, "external_agent")`},
		"scrum_channel_activity_merge.go": {"Agent event"},
		"chat_job_progress_stations.go":   {"summarizeChatExternalAgent", "External coding runtime"},
		"chat_job_progress_summary.go":    {"external_agent_started", "external_agent_failed", "external_agent_unavailable"},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains retired external-agent presentation authority %q", path, fragment)
			}
		}
	}
}

func TestRetiredScrumSyncRailHasNoRouteHandlerOrBrowserClient(t *testing.T) {
	server := NewServer(&queue.Repository{}, nil)
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/v1/scrum/cards/sync?project_id=1", wantStatus: http.StatusMethodNotAllowed},
		{path: "/v1/scrum/cards/card_1/sync?project_id=1", wantStatus: http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{}`))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.wantStatus {
			t.Errorf("retired sync route %s status=%d want=%d body=%s", test.path, response.Code, test.wantStatus, response.Body.String())
		}
	}
	for path, forbidden := range map[string][]string{
		"server.go":                    {`HandleFunc("/v1/scrum/cards/sync"`},
		"scrum_handlers.go":            {`case "sync"`, "handleScrumCardSync"},
		"scrum_card_state_handlers.go": {"handleScrumCardSync"},
		"web/src/lib/scrum_api.ts":     {"syncScrumBoard", "syncScrumCard", "/v1/scrum/cards/sync", `cardURL(cardID, "sync"`},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains retired Scrum sync authority %q", path, fragment)
			}
		}
	}
}

func TestScrumRealtimeProgressUsesOneCodeOwnedReason(t *testing.T) {
	for _, path := range []string{
		"scrum_play_autorun.go",
		"web/src/controllers/scrum_controller.ts",
	} {
		source := readFrontendOrAPISource(t, path)
		if strings.Contains(source, "agent output") || strings.Contains(source, "Agent produced new output") {
			t.Errorf("%s retains stale external-agent realtime vocabulary", path)
		}
	}
	producer := readFrontendOrAPISource(t, "scrum_play_autorun.go")
	if !strings.Contains(producer, `scrumCardRealtimeJobProgress scrumCardRealtimeReason = "job_progress"`) ||
		!strings.Contains(producer, "string(scrumCardRealtimeJobProgress)") {
		t.Error("Scrum job progress producer lacks its closed code-owned realtime reason")
	}
	controller := readFrontendOrAPISource(t, "web/src/controllers/scrum_controller.ts")
	if !strings.Contains(controller, "SCRUM_CARD_REALTIME_REASON.jobProgress") {
		t.Error("Scrum controller does not consume the shared closed job-progress reason")
	}
}

func TestScrumBrowserHasNoExternalAgentOrOptionalProjectVocabulary(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"web/src/react/card-modal/ChannelTab.tsx": {
			"agentWorking", "Agent working", "Steer this card", "waiting agent",
		},
		"web/styles.css": {"channel-agent-reply"},
		"web/src/lib/scrum_api.ts": {
			"projectID?: number", "projectID: number | null", "projectID: number | null | undefined",
			`query.set("before", before.trim())`, "JSON.stringify({})", "updateScrumBoard", `method: "PUT"`,
		},
		"web/src/lib/scrum_ticket_api.ts": {"projectID?: number", "projectID: number | null"},
		"web/src/lib/project_api.ts":      {"projectID?: number", `return ""`},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains optional/agent browser authority %q", path, fragment)
			}
		}
	}
	operation := readFrontendOrAPISource(t, "scrum_channel_operation.go")
	if strings.Contains(operation, "waiting agent") {
		t.Error("Scrum channel result retains external-agent vocabulary")
	}
}

func TestScrumCardHasNoWriteOnlyModelConfigurationControl(t *testing.T) {
	for _, path := range []string{
		"web/src/react/card-modal/ConfigTab.tsx",
		"web/src/lib/model_config_api.ts",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired card-level model configuration surface remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired card-level model configuration surface %s: %v", path, err)
		}
	}
	server := NewServer(nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/models/resolved?project_id=1&card_id=card_1", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("retired card model route status=%d body=%s", response.Code, response.Body.String())
	}
	for path, forbidden := range map[string][]string{
		"server.go":                                 {`HandleFunc("/v1/models/resolved`},
		"scrum_card_edit.go":                        {`json:"model_config"`, "ModelConfig"},
		"scrum_modal_context.go":                    {`case "config"`, "populateScrumModalConfigContext", "ModelOverrides"},
		"scrum_modal_handlers.go":                   {`"model_fields"`, `"model_source"`, `"model_overrides"`},
		"web/src/lib/scrum_api.ts":                  {`| "model_config"`},
		"web/src/lib/scrum_types.ts":                {"ScrumConfigField", "model_overrides?:"},
		"web/src/react/card-modal/CardModalApp.tsx": {"ConfigTab", `activeTab === "config"`},
		"web/src/react/card-modal/types.ts":         {`"config"`},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains write-only card model authority %q", path, fragment)
			}
		}
	}
}

func TestDataPanelDoesNotAdvertiseRetiredNaturalLanguageInference(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"web/panels/data.html":     {"Chat with your databases", "natural-language queries"},
		"web/locales/en.json":      {"Chat with your databases", "natural-language queries"},
		"web/locales/es.json":      {"lenguaje natural"},
		"web/locales/ja.json":      {"自然言語クエリ"},
		"web/locales/ru.json":      {"естественном языке"},
		"web/locales/zh-Hans.json": {"自然语言查询"},
	} {
		source := readFrontendOrAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s advertises retired data inference %q", path, fragment)
			}
		}
	}
}

func readFrontendOrAPISource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
