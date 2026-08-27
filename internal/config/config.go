package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

type Config struct {
	ListenAddr                string
	CoreURL                   string
	HostAgentURL              string
	HostAgentToken            string
	IntegrationAPIToken       string
	WrapperOnly               bool
	DatabaseURL               string
	DatabaseSchema            string
	LLMProvider               string
	EmbeddingProvider         string
	ProviderModels            map[string]ProviderModelConfig
	OllamaBaseURL             string
	CompatibleProviders       map[string]CompatibleProviderConfig
	AzureAIBaseURL            string
	AzureAIAPIKey             string
	AzureAIAPIVersion         string
	AzureAIAPIStyle           string
	GoogleBaseURL             string
	GoogleAPIKey              string
	HuggingFaceBaseURL        string
	HuggingFaceAPIKey         string
	StationModels             map[station.ID]string
	RoleplaySemanticModel     string
	ContextRelevanceProvider  string
	EmbeddingModel            string
	WebSearchProviders        []string
	WebSearchTimeout          time.Duration
	WebSearchPerSourceBudget  int
	WebSearchTotalBudget      int
	WorkspaceRoot             string
	WorkspaceHostRoot         string
	DeploymentKeyFile         string
	DeploymentBindAddress     string
	DeploymentAdvertisedHost  string
	DeploymentProbeHost       string
	WorkerCount               int
	CodingFragmentConcurrency int
	WorkerPollInterval        time.Duration
	RequestTimeout            time.Duration
	RealtimeMaxClients        int
	RealtimeStreamMaxAge      time.Duration
	RealtimeHeartbeat         time.Duration
	RealtimeWriteTimeout      time.Duration
	RedisURL                  string
	UIRedisRequired           bool
	UISessionTTL              time.Duration
	InferenceContextTokens    int
	MigrateOnStartup          bool
}

// Load parses the environment and validates provider-independent runtime
// structure. It preserves provider selections and credentials without resolving
// them; provider validation belongs to the first actual provider operation.
func Load() (Config, error) {
	if err := validateTypedEnvironment(); err != nil {
		return Config{}, err
	}
	provider, embeddingProvider := loadProviderSelection()
	compatibleProviders := loadCompatibleProviderConfigs()
	providerModels := loadProviderModelConfigs()
	databaseSchema, err := loadDatabaseSchema()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:                getenv("LISTEN_ADDR", "0.0.0.0:8090"),
		HostAgentURL:              getenv("HOST_AGENT_URL", ""),
		HostAgentToken:            getenv("HOST_AGENT_TOKEN", ""),
		IntegrationAPIToken:       os.Getenv("OMNIDEX_INTEGRATION_API_TOKEN"),
		CoreURL:                   getenv("CORE_URL", "http://192.168.1.102:8090"),
		WrapperOnly:               getenvBool("WRAPPER_ONLY", false),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		DatabaseSchema:            databaseSchema,
		LLMProvider:               provider,
		EmbeddingProvider:         embeddingProvider,
		ProviderModels:            providerModels,
		OllamaBaseURL:             getenv("OLLAMA_BASE_URL", ""),
		CompatibleProviders:       compatibleProviders,
		AzureAIBaseURL:            firstNonEmptyEnv([]string{"AZURE_AI_BASE_URL", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_BASE_URL"}, ""),
		AzureAIAPIKey:             firstEnv("AZURE_AI_API_KEY", "AZURE_OPENAI_API_KEY"),
		AzureAIAPIVersion:         getenv("AZURE_AI_API_VERSION", getenv("AZURE_OPENAI_API_VERSION", "")),
		AzureAIAPIStyle:           getenv("AZURE_AI_API_STYLE", getenv("AZURE_OPENAI_API_STYLE", "")),
		GoogleBaseURL:             getenv("GOOGLE_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		GoogleAPIKey:              firstEnv("GOOGLE_API_KEY", "GEMINI_API_KEY"),
		HuggingFaceBaseURL:        getenv("HUGGINGFACE_BASE_URL", "https://router.huggingface.co"),
		HuggingFaceAPIKey:         firstEnv("HUGGINGFACE_API_KEY", "HF_TOKEN"),
		EmbeddingModel:            embeddingModelForProvider(embeddingProvider),
		RoleplaySemanticModel:     getenv("OMNI_ROLEPLAY_SEMANTIC_MODEL", ""),
		ContextRelevanceProvider:  strings.ToLower(getenv("OMNI_CONTEXT_RELEVANCE_PROVIDER", ContextRelevanceProviderServer)),
		WebSearchProviders:        getenvCSV("WEB_SEARCH_PROVIDERS", []string{"brave", "google", "reddit"}),
		WebSearchTimeout:          getenvDuration("WEB_SEARCH_TIMEOUT", 15*time.Second),
		WebSearchPerSourceBudget:  getenvInt("WEB_SEARCH_PER_SOURCE_BUDGET", 3000),
		WebSearchTotalBudget:      getenvInt("WEB_SEARCH_TOTAL_BUDGET", 6000),
		WorkspaceRoot:             getenv("WORKSPACE_ROOT", ""),
		WorkspaceHostRoot:         getenv("HOST_WORKSPACE_PATH", ""),
		DeploymentKeyFile:         getenv("OMNIDEX_DEPLOYMENT_KEY_FILE", "/var/lib/omnidex-deployment/key"),
		DeploymentBindAddress:     getenv("OMNIDEX_DEPLOYMENT_BIND_ADDRESS", ""),
		DeploymentAdvertisedHost:  getenv("OMNIDEX_DEPLOYMENT_ADVERTISED_HOST", ""),
		DeploymentProbeHost:       getenv("OMNIDEX_DEPLOYMENT_PROBE_HOST", ""),
		WorkerCount:               getenvInt("WORKER_COUNT", 2),
		CodingFragmentConcurrency: getenvInt("CODING_FRAGMENT_CONCURRENCY", defaultCodingFragmentConcurrency(provider)),
		WorkerPollInterval:        getenvDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		RequestTimeout:            getenvDuration("REQUEST_TIMEOUT", 10*time.Minute),
		RealtimeMaxClients:        getenvInt("REALTIME_MAX_CLIENTS", 512),
		RealtimeStreamMaxAge:      getenvDuration("REALTIME_STREAM_MAX_AGE", 10*time.Minute),
		RealtimeHeartbeat:         getenvDuration("REALTIME_HEARTBEAT", 25*time.Second),
		RealtimeWriteTimeout:      getenvDuration("REALTIME_WRITE_TIMEOUT", 10*time.Second),
		RedisURL:                  getenv("REDIS_URL", ""),
		UIRedisRequired:           getenvBool("UI_REDIS_REQUIRED", false),
		UISessionTTL:              getenvDuration("UI_SESSION_TTL", 30*time.Minute),
		InferenceContextTokens:    getenvInt("INFERENCE_CONTEXT_TOKENS", llm.DefaultInferenceContextTokens),
		MigrateOnStartup:          getenvBool("MIGRATE_ON_STARTUP", true),
	}

	if err := validateConfigStructure(cfg); err != nil {
		return Config{}, err
	}

	cfg.StationModels = loadStationModels(cfg)

	return cfg, nil
}

func loadDatabaseSchema() (string, error) {
	value, configured := os.LookupEnv("DATABASE_SCHEMA")
	if !configured {
		return db.DefaultRuntimeSchema, nil
	}
	if value == "" {
		return "", fmt.Errorf("DATABASE_SCHEMA is explicitly empty")
	}
	if err := db.ValidateRuntimeSchemaName(value); err != nil {
		return "", fmt.Errorf("DATABASE_SCHEMA: %w", err)
	}
	return value, nil
}

func defaultCodingFragmentConcurrency(provider string) int {
	if strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return 1
	}
	return 4
}

// Validate checks provider-independent runtime structure. Provider authority is
// deliberately resolved and validated only at the first provider operation.
func Validate(cfg Config) error {
	return validateConfigStructure(cfg)
}

func validateConfigStructure(cfg Config) error {
	if err := validateIntegrationAPIToken(cfg.IntegrationAPIToken); err != nil {
		return err
	}
	if !cfg.WrapperOnly && strings.TrimSpace(cfg.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if !cfg.WrapperOnly {
		if err := db.ValidateRuntimeSchemaName(cfg.DatabaseSchema); err != nil {
			return fmt.Errorf("DATABASE_SCHEMA: %w", err)
		}
	}
	return validateRuntimeConfig(cfg)
}

func validateIntegrationAPIToken(token string) error {
	if token == "" {
		return nil
	}
	if len(token) < 32 || len(token) > 4096 {
		return fmt.Errorf("OMNIDEX_INTEGRATION_API_TOKEN must contain 32..4096 exact ASCII bytes")
	}
	for _, character := range []byte(token) {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("OMNIDEX_INTEGRATION_API_TOKEN must contain only exact visible ASCII bytes")
		}
	}
	return nil
}
