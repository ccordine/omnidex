package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectedDirectInferenceImplementationsAreAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{
		"internal/api/scrum_coach.go",
		"internal/api/scrum_coach_config.go",
		"internal/api/scrum_card_llm_enqueue.go",
		"internal/api/project_debugger.go",
		"internal/worker/data_source_llm.go",
		"internal/worker/data_source_query.go",
		"internal/worker/data_source_explore.go",
		"internal/worker/project_debugger.go",
		"internal/worker/scrum_card_llm.go",
		"internal/projectdebugger/scan.go",
		"internal/scrumcardllm/runner.go",
		"internal/modelconfig/resolve.go",
		"internal/research/research.go",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Fatalf("rejected direct-inference implementation remains: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", name, err)
		}
	}
}

func TestStaticBootstrapSkillsAndPersonaResearchAreAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/queue/worker_skills.go": {
			"SyncBootstrapSkills", "syncBootstrapSkill", "bootstrap_contract",
		},
		"internal/specialists/version.go": {
			"SkillSourceBootstrap", "SkillKindBootstrapSpecialist", `"bootstrap_specialist"`,
		},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("static bootstrap authority %q remains in %s", token, name)
			}
		}
	}
	walkDirectInferenceProductionGo(t, filepath.Join(root, "internal"), func(path, source string) {
		if strings.Contains(source, `internal/research`) {
			t.Errorf("production source %s imports the retired persona research layer", path)
		}
	})
}

func TestRetiredInferenceClientAndDocumentationSurfacesAreAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{
		"docs/CHAT_WIRING_AUDIT.md",
		"docs/DOCUMENTATION_SPECIALIST.md",
		"docs/LOCAL_SERVICE_CHANNELS.md",
		"docs/SCRUM_PLANNER.md",
		"internal/api/web/src/controllers/project_chat_controller.ts",
		"internal/api/web/src/lib/project_chat_api.ts",
		"internal/api/web/src/lib/project_chat_render.ts",
		"internal/api/web/src/lib/project_debugger_coordinator.ts",
		"internal/api/web/src/lib/project_debugger_render.ts",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("retired inference client/documentation surface remains: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", name, err)
		}
	}
	aliases, err := os.ReadFile(filepath.Join(root, "agent_aliases.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"--profile architect", "--web auto", "--workspace auto", "--reasoning",
		"enqueue --pipeline", "acont()", `_agent_cli_cmd continue`,
	} {
		if strings.Contains(string(aliases), token) {
			t.Errorf("retired CLI inference control %q remains in agent_aliases.sh", token)
		}
	}
	documentChecks := map[string][]string{
		"docs/PATCH_MODE.md": {
			"Structured planner payloads", `"tool": "patch.apply"`,
			"a model proposes a unified diff", "omni patch apply",
		},
		"docs/CODEBASE_MAP.md":      {"This helps the planner"},
		"docs/DEVELOPMENT_LOOPS.md": {"The planner should express proof work"},
		"docs/ROADMAP.md":           {"## Planner & Scrum Track (Venusaur)"},
	}
	for name, forbidden := range documentChecks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("retired inference documentation %q remains in %s", token, name)
			}
		}
	}
	walkDirectInferenceProductionText(t, filepath.Join(root, "internal", "api", "web", "src"), func(path, source string) {
		for _, token := range []string{
			"project_chat_controller", "ProjectDebuggerCoordinator", "coachScrumCard",
			"suggestScrumTags", "create_ticket_config", "/project-planning/chat",
		} {
			if strings.Contains(source, token) {
				t.Errorf("retired inference client token %q remains in %s", token, path)
			}
		}
	})
}

func TestProductionAPIAndWorkerInferenceUsesOnlyExactBoundary(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, directory := range []string{"internal/api", "internal/worker"} {
		walkDirectInferenceProductionGo(t, filepath.Join(root, directory), func(path, source string) {
			for _, forbidden := range []string{
				".Generate(", ".GeneratePrepared(", ".GeneratePreparedStream(", "PrepareContextModel(",
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("production source %s bypasses the exact station boundary with %q", path, forbidden)
				}
			}
			if strings.Contains(source, "llm.Client") {
				t.Errorf("production API/worker source %s retains broad llm.Client authority", path)
			}
		})
	}
}

func TestBroadModelRolesAndLegacyQueueActionsAreAbsent(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/modelconfig/config.go": {
			`Key: "default_model"`, `Key: "fast_model"`, `Key: "glue_model"`,
			`Key: "reasoning_model"`, `Key: "planner_model"`, `Key: "analyzer_model"`,
			`Key: "responder_model"`, `Key: "tagger_model"`, `Key: "search_model"`, `Key: "memory_model"`,
		},
		"internal/modelconfig/routing.go": {
			"Default string", "Fast string", "Glue string", "Reasoning string", "Tagging string",
			"Plan string", "Analyze string", "Response string", "Search string", "Memory string",
		},
		"internal/config/station_models.go": {"fallback", "DefaultModel"},
		"internal/config/config.go": {
			"DefaultModel", "AnthropicBaseURL", "AnthropicAPIKey", "AnthropicVersion", "AnthropicMaxTokens",
			`getenv("OLLAMA_BASE_URL", "http://host.docker.internal:11434")`,
		},
		"internal/llmprovider/catalog/definitions.go": {
			`DefaultBaseURL: "http://host.docker.internal:11434"`,
		},
		"internal/llmprovider/factory.go": {"resolvedGenerationModel", "definition.DefaultModel", "cfg.DefaultModel"},
		"internal/llmprovider/catalog/catalog.go": {
			"\n\tModelEnvironmentKeys", "\n\tDefaultModel", "SupportsGeneration", "GenerationProviderIDs",
		},
		"internal/api/provider_catalog.go": {
			"generation_provider", "supports_generation", "selected_for_generation", "generation_configured",
		},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("rejected broad inference token %q remains in %s", token, name)
			}
		}
	}
	for _, directory := range []string{"internal/model", "internal/queue", "internal/worker"} {
		walkDirectInferenceProductionGo(t, filepath.Join(root, directory), func(path, source string) {
			for _, token := range []string{
				"PipelineDataQuery", "PipelineDataExplore", "PipelineProjectDebugger", "PipelineScrumCardLLM",
				`action: "data_source_query"`, `action: "data_source_explore"`, `action: "project_debugger"`, `action: "scrum_card_llm"`,
			} {
				if strings.Contains(source, token) {
					t.Errorf("removed queue authority %q remains in %s", token, path)
				}
			}
		})
	}
}

func TestJobModelMetadataHasNoWriteOnlySourceLabel(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{
		"internal/api/model_config_service.go",
		"internal/worker/model_override.go",
	} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range []string{"model_config_source", "modelConfigSource"} {
			if strings.Contains(string(raw), token) {
				t.Errorf("write-only model metadata %q remains in %s", token, name)
			}
		}
	}
}

func TestShippedEnvironmentTemplatesExcludeRemovedModelRoles(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{".env.example", "default.env", "docker-compose.yml"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, suffix := range []string{
			"MODEL_FAST", "MODEL_GLUE", "MODEL_REASONING", "MODEL_TAGGER", "MODEL_PLANNER",
			"MODEL_ANALYZER", "MODEL_RESPONDER", "MODEL_SEARCH", "MODEL_MEMORY", "OLLAMA_FALLBACK_URLS",
			"OLLAMA_MODEL=", "OPENAI_MODEL=", "GOOGLE_MODEL=", "GEMINI_MODEL=", "ANTHROPIC_MODEL=",
			"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "XAI_API_KEY", "XAI_BASE_URL", "GROK_API_KEY",
			"DEEPSEEK_API_KEY", "MOONSHOT_API_KEY", "DOUBAO_API_KEY", "MODELSCOPE_API_KEY",
		} {
			if strings.Contains(string(raw), suffix) {
				t.Errorf("shipped configuration %s retains removed key suffix %s", name, suffix)
			}
		}
	}
}

func walkDirectInferenceProductionGo(t *testing.T, root string, inspect func(string, string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inspect(path, string(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func walkDirectInferenceProductionText(t *testing.T, root string, inspect func(string, string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.Contains(entry.Name(), ".test.") {
			return nil
		}
		extension := filepath.Ext(path)
		if extension != ".ts" && extension != ".tsx" && extension != ".html" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inspect(path, string(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
