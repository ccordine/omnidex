package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/specialist"
)

type Config struct {
	AppEnv                   string
	ListenAddr               string
	WrapperOnly              bool
	DatabaseURL              string
	LLMProvider              string
	OllamaBaseURL            string
	OpenAIBaseURL            string
	OpenAIAPIKey             string
	OpenAIOrganization       string
	OpenAIProject            string
	OllamaRestartCommand     string
	OllamaRestartTimeout     time.Duration
	DefaultModel             string
	FastModel                string
	ReasoningModel           string
	TaggingModel             string
	PlanModel                string
	AnalyzeModel             string
	ResponseModel            string
	SearchModel              string
	MemoryModel              string
	SpecialistModels         map[string]string
	EmbeddingModel           string
	WebSearchEnabled         bool
	WebSearchProviders       []string
	WebSearchTimeout         time.Duration
	WebSearchPerSourceBudget int
	WebSearchTotalBudget     int
	WorkspaceScanEnabled     bool
	WorkspaceRoot            string
	WorkspaceMaxFiles        int
	WorkspaceContextBudget   int
	StopOnSufficientContext  bool
	SufficientContextChars   int
	MemoryInferenceEnabled   bool
	MemoryInferenceMaxItems  int
	TournamentEnabled        bool
	TournamentChunkChars     int
	TournamentSummaryChars   int
	TournamentMaxRounds      int
	TournamentVerify         bool
	WorkerCount              int
	WorkerPollInterval       time.Duration
	RequestTimeout           time.Duration
	RetrievalLimit           int
	ContextCharBudget        int
	HallucinationRetryLimit  int
	MigrateOnStartup         bool
	V3Enabled                bool
	SkillsRoot               string
}

func Load() (Config, error) {
	provider := strings.ToLower(strings.TrimSpace(getenv("LLM_PROVIDER", "ollama")))
	if provider == "" {
		provider = "ollama"
	}
	if provider != "ollama" && provider != "openai" {
		return Config{}, fmt.Errorf("LLM_PROVIDER must be one of: ollama, openai")
	}

	cfg := Config{
		AppEnv:                   getenv("APP_ENV", "development"),
		ListenAddr:               getenv("LISTEN_ADDR", ":8090"),
		WrapperOnly:              getenvBool("WRAPPER_ONLY", false),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		LLMProvider:              provider,
		OllamaBaseURL:            getenv("OLLAMA_BASE_URL", "http://host.docker.internal:11434"),
		OpenAIBaseURL:            getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:             strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIOrganization:       getenv("OPENAI_ORGANIZATION", ""),
		OpenAIProject:            getenv("OPENAI_PROJECT", ""),
		OllamaRestartCommand:     getenv("OLLAMA_RESTART_COMMAND", ""),
		OllamaRestartTimeout:     getenvDuration("OLLAMA_RESTART_TIMEOUT", 20*time.Second),
		DefaultModel:             getenvProvider(provider, "OPENAI_MODEL", "OLLAMA_MODEL", defaultModelForProvider(provider)),
		FastModel:                getenvProvider(provider, "OPENAI_MODEL_FAST", "OLLAMA_MODEL_FAST", ""),
		ReasoningModel:           getenvProvider(provider, "OPENAI_MODEL_REASONING", "OLLAMA_MODEL_REASONING", ""),
		TaggingModel:             getenvProvider(provider, "OPENAI_MODEL_TAGGER", "OLLAMA_MODEL_TAGGER", ""),
		PlanModel:                getenvProvider(provider, "OPENAI_MODEL_PLANNER", "OLLAMA_MODEL_PLANNER", ""),
		AnalyzeModel:             getenvProvider(provider, "OPENAI_MODEL_ANALYZER", "OLLAMA_MODEL_ANALYZER", ""),
		ResponseModel:            getenvProvider(provider, "OPENAI_MODEL_RESPONDER", "OLLAMA_MODEL_RESPONDER", ""),
		SearchModel:              getenvProvider(provider, "OPENAI_MODEL_SEARCH", "OLLAMA_MODEL_SEARCH", ""),
		MemoryModel:              getenvProvider(provider, "OPENAI_MODEL_MEMORY", "OLLAMA_MODEL_MEMORY", ""),
		EmbeddingModel:           embeddingModelForProvider(provider),
		WebSearchEnabled:         getenvBool("WEB_SEARCH_ENABLED", true),
		WebSearchProviders:       getenvCSV("WEB_SEARCH_PROVIDERS", []string{"yahoo", "google", "reddit"}),
		WebSearchTimeout:         getenvDuration("WEB_SEARCH_TIMEOUT", 15*time.Second),
		WebSearchPerSourceBudget: getenvInt("WEB_SEARCH_PER_SOURCE_BUDGET", 3000),
		WebSearchTotalBudget:     getenvInt("WEB_SEARCH_TOTAL_BUDGET", 6000),
		WorkspaceScanEnabled:     getenvBool("WORKSPACE_SCAN_ENABLED", true),
		WorkspaceRoot:            getenv("WORKSPACE_ROOT", ""),
		WorkspaceMaxFiles:        getenvInt("WORKSPACE_MAX_FILES", 5000),
		WorkspaceContextBudget:   getenvInt("WORKSPACE_CONTEXT_BUDGET", 6000),
		StopOnSufficientContext:  getenvBool("STOP_ON_SUFFICIENT_CONTEXT", true),
		SufficientContextChars:   getenvInt("SUFFICIENT_CONTEXT_CHARS", 1400),
		MemoryInferenceEnabled:   getenvBool("MEMORY_INFERENCE_ENABLED", true),
		MemoryInferenceMaxItems:  getenvInt("MEMORY_INFERENCE_MAX_ITEMS", 3),
		TournamentEnabled:        getenvBool("TOURNAMENT_ENABLED", true),
		TournamentChunkChars:     getenvInt("TOURNAMENT_CHUNK_CHARS", 2200),
		TournamentSummaryChars:   getenvInt("TOURNAMENT_SUMMARY_CHARS", 750),
		TournamentMaxRounds:      getenvInt("TOURNAMENT_MAX_ROUNDS", 4),
		TournamentVerify:         getenvBool("TOURNAMENT_VERIFY_RELEVANCE", true),
		WorkerCount:              getenvInt("WORKER_COUNT", 2),
		WorkerPollInterval:       getenvDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		RequestTimeout:           getenvDuration("REQUEST_TIMEOUT", 90*time.Second),
		RetrievalLimit:           getenvInt("RETRIEVAL_LIMIT", 8),
		ContextCharBudget:        getenvInt("CONTEXT_CHAR_BUDGET", 4000),
		HallucinationRetryLimit:  getenvInt("HALLUCINATION_RETRY_LIMIT", 2),
		MigrateOnStartup:         getenvBool("MIGRATE_ON_STARTUP", true),
		V3Enabled:                getenvBool("OMNIDEX_V3_ENABLED", true),
		SkillsRoot:               getenv("OMNIDEX_SKILLS_ROOT", "skills"),
	}

	if !cfg.WrapperOnly && cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.LLMProvider == "openai" && cfg.OpenAIAPIKey == "" {
		return Config{}, fmt.Errorf("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
	}

	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 1
	}
	if cfg.SufficientContextChars < 1 {
		cfg.SufficientContextChars = 1400
	}
	if cfg.MemoryInferenceMaxItems < 0 {
		cfg.MemoryInferenceMaxItems = 0
	}
	if cfg.TournamentChunkChars < 500 {
		cfg.TournamentChunkChars = 2200
	}
	if cfg.TournamentSummaryChars < 120 {
		cfg.TournamentSummaryChars = 750
	}
	if cfg.TournamentMaxRounds < 1 {
		cfg.TournamentMaxRounds = 4
	}
	if cfg.TournamentMaxRounds > 8 {
		cfg.TournamentMaxRounds = 8
	}
	if cfg.WorkspaceMaxFiles < 1 {
		cfg.WorkspaceMaxFiles = 5000
	}
	if cfg.WorkspaceContextBudget < 1 {
		cfg.WorkspaceContextBudget = 6000
	}
	if cfg.OllamaRestartTimeout <= 0 {
		cfg.OllamaRestartTimeout = 20 * time.Second
	}
	if cfg.HallucinationRetryLimit < 1 {
		cfg.HallucinationRetryLimit = 1
	}
	if cfg.HallucinationRetryLimit > 6 {
		cfg.HallucinationRetryLimit = 6
	}

	if cfg.FastModel == "" {
		cfg.FastModel = cfg.DefaultModel
	}
	if cfg.ReasoningModel == "" {
		cfg.ReasoningModel = cfg.DefaultModel
	}
	if cfg.TaggingModel == "" {
		cfg.TaggingModel = cfg.FastModel
	}
	if cfg.AnalyzeModel == "" {
		cfg.AnalyzeModel = cfg.ReasoningModel
	}
	if cfg.PlanModel == "" {
		cfg.PlanModel = cfg.ReasoningModel
	}
	if cfg.ResponseModel == "" {
		cfg.ResponseModel = cfg.ReasoningModel
	}
	if cfg.SearchModel == "" {
		cfg.SearchModel = cfg.FastModel
	}
	if cfg.MemoryModel == "" {
		cfg.MemoryModel = cfg.FastModel
	}

	roleEnv := func(roleID string, fallback string) string {
		legacy := specialist.EnvVarForRoleID(roleID)
		openAIKey := openAIEnvAlias(legacy)
		return getenvProvider(cfg.LLMProvider, openAIKey, legacy, fallback)
	}
	cfg.SpecialistModels = map[string]string{
		specialist.RolePlannerSpecialist:            roleEnv(specialist.RolePlannerSpecialist, cfg.PlanModel),
		specialist.RoleToolingSpecialist:            roleEnv(specialist.RoleToolingSpecialist, cfg.AnalyzeModel),
		specialist.RoleFilesystemResearchSpecialist: roleEnv(specialist.RoleFilesystemResearchSpecialist, cfg.AnalyzeModel),
		specialist.RoleIntentTaggingSpecialist:      roleEnv(specialist.RoleIntentTaggingSpecialist, cfg.TaggingModel),
		specialist.RoleMemoryRetrievalSpecialist:    roleEnv(specialist.RoleMemoryRetrievalSpecialist, cfg.MemoryModel),
		specialist.RoleWebResearchSpecialist:        roleEnv(specialist.RoleWebResearchSpecialist, cfg.SearchModel),
		specialist.RoleAnalysisSpecialist:           roleEnv(specialist.RoleAnalysisSpecialist, cfg.AnalyzeModel),
		specialist.RoleResponseSpecialist:           roleEnv(specialist.RoleResponseSpecialist, cfg.ResponseModel),
		specialist.RoleReviewVerificationSpecialist: roleEnv(specialist.RoleReviewVerificationSpecialist, cfg.AnalyzeModel),
		specialist.RoleMediaControlSpecialist:       roleEnv(specialist.RoleMediaControlSpecialist, cfg.ResponseModel),
		specialist.RoleBrowserInspectionSpecialist:  roleEnv(specialist.RoleBrowserInspectionSpecialist, cfg.ResponseModel),
		specialist.RoleScreenVisionSpecialist:       roleEnv(specialist.RoleScreenVisionSpecialist, cfg.ResponseModel),
		specialist.RoleShellExecutionSpecialist:     roleEnv(specialist.RoleShellExecutionSpecialist, cfg.PlanModel),
		specialist.RoleAudioNotesSpecialist:         roleEnv(specialist.RoleAudioNotesSpecialist, cfg.ResponseModel),
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		parsed, err := time.ParseDuration(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func normalizeLLMProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai":
		return "openai"
	default:
		return "ollama"
	}
}

func defaultModelForProvider(provider string) string {
	if normalizeLLMProvider(provider) == "openai" {
		return "gpt-4.1-mini"
	}
	return "llama3.2"
}

func embeddingModelForProvider(provider string) string {
	if normalizeLLMProvider(provider) == "openai" {
		return getenv("OPENAI_EMBEDDING_MODEL", getenv("EMBEDDING_MODEL", "text-embedding-3-small"))
	}
	return getenv("EMBEDDING_MODEL", "nomic-embed-text")
}

func getenvProvider(provider string, openaiKey, legacyKey, fallback string) string {
	if normalizeLLMProvider(provider) == "openai" {
		return getenv(openaiKey, fallback)
	}
	return getenv(legacyKey, fallback)
}

func openAIEnvAlias(legacy string) string {
	legacy = strings.TrimSpace(legacy)
	if legacy == "" {
		return ""
	}
	if strings.HasPrefix(legacy, "OLLAMA_") {
		return "OPENAI_" + strings.TrimPrefix(legacy, "OLLAMA_")
	}
	return "OPENAI_" + legacy
}
