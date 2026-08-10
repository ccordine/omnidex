package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/specialist"
)

type Config struct {
	AppEnv                       string
	ListenAddr                   string
	CoreURL                      string
	HostAgentURL                 string
	HostAgentToken               string
	WrapperOnly                  bool
	DatabaseURL                  string
	LLMProvider                  string
	EmbeddingProvider            string
	ProviderModels               map[string]ProviderModelConfig
	OllamaBaseURL                string
	CompatibleProviders          map[string]CompatibleProviderConfig
	AzureAIBaseURL               string
	AzureAIAPIKey                string
	AzureAIAPIVersion            string
	AzureAIAPIStyle              string
	GoogleBaseURL                string
	GoogleAPIKey                 string
	AnthropicBaseURL             string
	AnthropicAPIKey              string
	AnthropicVersion             string
	AnthropicMaxTokens           int
	HuggingFaceBaseURL           string
	HuggingFaceAPIKey            string
	DefaultModel                 string
	FastModel                    string
	GlueModel                    string
	ReasoningModel               string
	TaggingModel                 string
	PlanModel                    string
	AnalyzeModel                 string
	ResponseModel                string
	SearchModel                  string
	MemoryModel                  string
	SpecialistModels             map[string]string
	EmbeddingModel               string
	WebSearchEnabled             bool
	WebSearchProviders           []string
	WebSearchTimeout             time.Duration
	WebSearchPerSourceBudget     int
	WebSearchTotalBudget         int
	WorkspaceScanEnabled         bool
	WorkspaceRoot                string
	WorkspaceHostRoot            string
	WorkspaceMaxFiles            int
	WorkspaceContextBudget       int
	WorkerCount                  int
	CodingFragmentConcurrency    int
	WorkerPollInterval           time.Duration
	RequestTimeout               time.Duration
	RealtimeMaxClients           int
	RealtimeStreamMaxAge         time.Duration
	RealtimeHeartbeat            time.Duration
	RealtimeWriteTimeout         time.Duration
	RedisURL                     string
	UIRedisRequired              bool
	UISessionTTL                 time.Duration
	RetrievalLimit               int
	ContextCharBudget            int
	InferenceContextTokens       int
	CognitionModelDigest         string
	CognitionModelQuantization   string
	CognitionBackendVersion      string
	CognitionHardware            string
	CognitionContextCeilingBytes int
	CognitionMaxOutputTokens     int
	MigrateOnStartup             bool
	SkillsRoot                   string
}

// Load parses the environment and validates all non-secret configuration.
// Call Validate after applying the configured durable secret store.
func Load() (Config, error) {
	if err := validateTypedEnvironment(); err != nil {
		return Config{}, err
	}
	provider, embeddingProvider, err := loadProviderSelection()
	if err != nil {
		return Config{}, err
	}
	compatibleProviders := loadCompatibleProviderConfigs()
	providerModels := loadProviderModelConfigs()
	cognitionContextCeilingBytes, err := requiredPositiveEnvInt("COGNITION_CONTEXT_CEILING_BYTES")
	if err != nil {
		return Config{}, err
	}
	cognitionMaxOutputTokens, err := requiredPositiveEnvInt("COGNITION_MAX_OUTPUT_TOKENS")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:                       getenv("APP_ENV", "development"),
		ListenAddr:                   getenv("LISTEN_ADDR", "0.0.0.0:8090"),
		HostAgentURL:                 getenv("HOST_AGENT_URL", ""),
		HostAgentToken:               getenv("HOST_AGENT_TOKEN", ""),
		CoreURL:                      getenv("CORE_URL", "http://192.168.1.102:8090"),
		WrapperOnly:                  getenvBool("WRAPPER_ONLY", false),
		DatabaseURL:                  os.Getenv("DATABASE_URL"),
		LLMProvider:                  provider,
		EmbeddingProvider:            embeddingProvider,
		ProviderModels:               providerModels,
		OllamaBaseURL:                getenv("OLLAMA_BASE_URL", "http://host.docker.internal:11434"),
		CompatibleProviders:          compatibleProviders,
		AzureAIBaseURL:               firstNonEmptyEnv([]string{"AZURE_AI_BASE_URL", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_BASE_URL"}, ""),
		AzureAIAPIKey:                firstEnv("AZURE_AI_API_KEY", "AZURE_OPENAI_API_KEY"),
		AzureAIAPIVersion:            getenv("AZURE_AI_API_VERSION", getenv("AZURE_OPENAI_API_VERSION", "")),
		AzureAIAPIStyle:              getenv("AZURE_AI_API_STYLE", getenv("AZURE_OPENAI_API_STYLE", "")),
		GoogleBaseURL:                getenv("GOOGLE_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		GoogleAPIKey:                 firstEnv("GOOGLE_API_KEY", "GEMINI_API_KEY"),
		AnthropicBaseURL:             getenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"),
		AnthropicAPIKey:              strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AnthropicVersion:             getenv("ANTHROPIC_VERSION", "2023-06-01"),
		AnthropicMaxTokens:           getenvInt("ANTHROPIC_MAX_TOKENS", 4096),
		HuggingFaceBaseURL:           getenv("HUGGINGFACE_BASE_URL", "https://router.huggingface.co"),
		HuggingFaceAPIKey:            firstEnv("HUGGINGFACE_API_KEY", "HF_TOKEN"),
		DefaultModel:                 providerModels[provider].Default,
		FastModel:                    getenvProvider(provider, "MODEL_FAST", ""),
		GlueModel:                    getenvProvider(provider, "MODEL_GLUE", ""),
		ReasoningModel:               getenvProvider(provider, "MODEL_REASONING", ""),
		TaggingModel:                 getenvProvider(provider, "MODEL_TAGGER", ""),
		PlanModel:                    getenvProvider(provider, "MODEL_PLANNER", ""),
		AnalyzeModel:                 getenvProvider(provider, "MODEL_ANALYZER", ""),
		ResponseModel:                getenvProvider(provider, "MODEL_RESPONDER", ""),
		SearchModel:                  getenvProvider(provider, "MODEL_SEARCH", ""),
		MemoryModel:                  getenvProvider(provider, "MODEL_MEMORY", ""),
		EmbeddingModel:               embeddingModelForProvider(embeddingProvider),
		WebSearchEnabled:             getenvBool("WEB_SEARCH_ENABLED", true),
		WebSearchProviders:           getenvCSV("WEB_SEARCH_PROVIDERS", []string{"duckduckgo", "google", "reddit"}),
		WebSearchTimeout:             getenvDuration("WEB_SEARCH_TIMEOUT", 15*time.Second),
		WebSearchPerSourceBudget:     getenvInt("WEB_SEARCH_PER_SOURCE_BUDGET", 3000),
		WebSearchTotalBudget:         getenvInt("WEB_SEARCH_TOTAL_BUDGET", 6000),
		WorkspaceScanEnabled:         getenvBool("WORKSPACE_SCAN_ENABLED", true),
		WorkspaceRoot:                getenv("WORKSPACE_ROOT", ""),
		WorkspaceHostRoot:            getenv("HOST_WORKSPACE_PATH", ""),
		WorkspaceMaxFiles:            getenvInt("WORKSPACE_MAX_FILES", 5000),
		WorkspaceContextBudget:       getenvInt("WORKSPACE_CONTEXT_BUDGET", 6000),
		WorkerCount:                  getenvInt("WORKER_COUNT", 2),
		CodingFragmentConcurrency:    getenvInt("CODING_FRAGMENT_CONCURRENCY", defaultCodingFragmentConcurrency(provider)),
		WorkerPollInterval:           getenvDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		RequestTimeout:               getenvDuration("REQUEST_TIMEOUT", 180*time.Second),
		RealtimeMaxClients:           getenvInt("REALTIME_MAX_CLIENTS", 512),
		RealtimeStreamMaxAge:         getenvDuration("REALTIME_STREAM_MAX_AGE", 10*time.Minute),
		RealtimeHeartbeat:            getenvDuration("REALTIME_HEARTBEAT", 25*time.Second),
		RealtimeWriteTimeout:         getenvDuration("REALTIME_WRITE_TIMEOUT", 10*time.Second),
		RedisURL:                     getenv("REDIS_URL", ""),
		UIRedisRequired:              getenvBool("UI_REDIS_REQUIRED", false),
		UISessionTTL:                 getenvDuration("UI_SESSION_TTL", 30*time.Minute),
		RetrievalLimit:               getenvInt("RETRIEVAL_LIMIT", 8),
		ContextCharBudget:            getenvInt("CONTEXT_CHAR_BUDGET", 4000),
		InferenceContextTokens:       getenvInt("INFERENCE_CONTEXT_TOKENS", llm.DefaultInferenceContextTokens),
		CognitionModelDigest:         os.Getenv("COGNITION_MODEL_SHA256"),
		CognitionModelQuantization:   os.Getenv("COGNITION_MODEL_QUANTIZATION"),
		CognitionBackendVersion:      os.Getenv("COGNITION_BACKEND_VERSION"),
		CognitionHardware:            os.Getenv("COGNITION_HARDWARE"),
		CognitionContextCeilingBytes: cognitionContextCeilingBytes,
		CognitionMaxOutputTokens:     cognitionMaxOutputTokens,
		MigrateOnStartup:             getenvBool("MIGRATE_ON_STARTUP", true),
		SkillsRoot:                   getenv("OMNIDEX_SKILLS_ROOT", "skills"),
	}

	if err := validateConfigStructure(cfg); err != nil {
		return Config{}, err
	}

	if cfg.FastModel == "" {
		cfg.FastModel = cfg.DefaultModel
	}
	if cfg.GlueModel == "" {
		cfg.GlueModel = cfg.FastModel
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
		return getenvProvider(cfg.LLMProvider, strings.TrimPrefix(legacy, "OLLAMA_"), fallback)
	}
	executorModel := roleEnv(specialist.RoleSubtaskExecutorSpecialist, cfg.ReasoningModel)
	cfg.SpecialistModels = map[string]string{
		specialist.RolePlannerSpecialist:                 roleEnv(specialist.RolePlannerSpecialist, cfg.PlanModel),
		specialist.RoleToolingSpecialist:                 roleEnv(specialist.RoleToolingSpecialist, cfg.AnalyzeModel),
		specialist.RoleFilesystemResearchSpecialist:      roleEnv(specialist.RoleFilesystemResearchSpecialist, cfg.AnalyzeModel),
		specialist.RoleIntentTaggingSpecialist:           roleEnv(specialist.RoleIntentTaggingSpecialist, cfg.TaggingModel),
		specialist.RoleMemoryRetrievalSpecialist:         roleEnv(specialist.RoleMemoryRetrievalSpecialist, cfg.MemoryModel),
		specialist.RoleWebResearchSpecialist:             roleEnv(specialist.RoleWebResearchSpecialist, cfg.SearchModel),
		specialist.RoleSubtaskExecutorSpecialist:         executorModel,
		specialist.RoleAnalysisSpecialist:                roleEnv(specialist.RoleAnalysisSpecialist, cfg.AnalyzeModel),
		specialist.RoleResponseSpecialist:                roleEnv(specialist.RoleResponseSpecialist, cfg.ResponseModel),
		specialist.RoleReviewVerificationSpecialist:      roleEnv(specialist.RoleReviewVerificationSpecialist, cfg.AnalyzeModel),
		specialist.RoleMediaControlSpecialist:            roleEnv(specialist.RoleMediaControlSpecialist, cfg.ResponseModel),
		specialist.RoleBrowserInspectionSpecialist:       roleEnv(specialist.RoleBrowserInspectionSpecialist, cfg.ResponseModel),
		specialist.RoleScreenVisionSpecialist:            roleEnv(specialist.RoleScreenVisionSpecialist, cfg.ResponseModel),
		specialist.RoleShellExecutionSpecialist:          roleEnv(specialist.RoleShellExecutionSpecialist, cfg.PlanModel),
		specialist.RoleAudioNotesSpecialist:              roleEnv(specialist.RoleAudioNotesSpecialist, cfg.ResponseModel),
		specialist.RoleCodingSurfaceStation:              roleEnv(specialist.RoleCodingSurfaceStation, cfg.TaggingModel),
		specialist.RoleCodingProductIdentityStation:      roleEnv(specialist.RoleCodingProductIdentityStation, cfg.GlueModel),
		specialist.RoleCodingRequirementPartitionStation: roleEnv(specialist.RoleCodingRequirementPartitionStation, cfg.GlueModel),
		specialist.RoleCodingArtifactHandlingStation:     roleEnv(specialist.RoleCodingArtifactHandlingStation, cfg.GlueModel),
		specialist.RoleCodingCapabilityRelationStation:   roleEnv(specialist.RoleCodingCapabilityRelationStation, cfg.ReasoningModel),
		specialist.RoleCodingSkillSelectionStation:       roleEnv(specialist.RoleCodingSkillSelectionStation, cfg.ReasoningModel),
		specialist.RoleCodingSkillProcedureStation:       roleEnv(specialist.RoleCodingSkillProcedureStation, cfg.GlueModel),
		specialist.RoleCodingFragmentStation:             roleEnv(specialist.RoleCodingFragmentStation, executorModel),
		specialist.RoleCodingFragmentCorrectionStation:   roleEnv(specialist.RoleCodingFragmentCorrectionStation, executorModel),
	}

	return cfg, nil
}

func defaultCodingFragmentConcurrency(provider string) int {
	if strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return 1
	}
	return 4
}

// Validate performs final configuration validation, including credentials.
// It must run after database-backed secrets have been overlaid.
func Validate(cfg Config) error {
	if err := validateConfigStructure(cfg); err != nil {
		return err
	}
	if err := validateSelectedProviderCredential(cfg.LLMProvider, cfg, "LLM_PROVIDER"); err != nil {
		return err
	}
	return validateSelectedProviderCredential(cfg.EmbeddingProvider, cfg, "EMBEDDING_PROVIDER")
}

func validateConfigStructure(cfg Config) error {
	if !cfg.WrapperOnly && strings.TrimSpace(cfg.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if err := validateSelectedProviderEndpoint(cfg.LLMProvider, cfg, "LLM_PROVIDER"); err != nil {
		return err
	}
	if err := validateSelectedProviderEndpoint(cfg.EmbeddingProvider, cfg, "EMBEDDING_PROVIDER"); err != nil {
		return err
	}
	if err := validateSelectedProviderModels(cfg); err != nil {
		return err
	}
	return validateRuntimeConfig(cfg)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
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

func requiredPositiveEnvInt(key string) (int, error) {
	raw, exists := os.LookupEnv(key)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	if raw != strings.TrimSpace(raw) {
		return 0, fmt.Errorf("%s must be one exact positive integer", key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be one exact positive integer, received %q", key, raw)
	}
	return value, nil
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
