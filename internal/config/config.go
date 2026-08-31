package config

import (
	"fmt"
	"os"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/modelconfig"
)

type Config struct {
	ListenAddr                string
	CoreURL                   string
	HostAgentURL              string
	HostAgentToken            string
	IntegrationAPIToken       string
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
	ModelAuthority            modelconfig.Authority
	EmbeddingModel            string
	WorkerCount               int
	CodingFragmentConcurrency int
	WorkerPollInterval        time.Duration
	RequestTimeout            time.Duration
	RealtimeStreamMaxAge      time.Duration
	RealtimeHeartbeat         time.Duration
	RealtimeWriteTimeout      time.Duration
	RedisURL                  string
	UISessionTTL              time.Duration
	InferenceContextTokens    int
}

// Load parses the environment and preserves provider selections and credentials
// without resolving them; provider validation belongs to the first actual
// provider operation.
func Load() (Config, error) {
	provider, embeddingProvider := loadProviderSelection()
	compatibleProviders := loadCompatibleProviderConfigs()
	providerModels := loadProviderModelConfigs()
	modelAuthority, err := modelconfig.LoadEnvironment()
	if err != nil {
		return Config{}, fmt.Errorf("load model routing authority: %w", err)
	}
	databaseSchema, err := loadDatabaseSchema()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:                getenv("LISTEN_ADDR", "0.0.0.0:8090"),
		HostAgentURL:              getenv("HOST_AGENT_URL", ""),
		HostAgentToken:            getenv("HOST_AGENT_TOKEN", ""),
		IntegrationAPIToken:       os.Getenv("OMNIDEX_INTEGRATION_API_TOKEN"),
		CoreURL:                   os.Getenv("CORE_URL"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		DatabaseSchema:            databaseSchema,
		LLMProvider:               provider,
		EmbeddingProvider:         embeddingProvider,
		ProviderModels:            providerModels,
		OllamaBaseURL:             getenv("OLLAMA_BASE_URL", ""),
		CompatibleProviders:       compatibleProviders,
		AzureAIBaseURL:            os.Getenv("AZURE_AI_BASE_URL"),
		AzureAIAPIKey:             os.Getenv("AZURE_AI_API_KEY"),
		AzureAIAPIVersion:         os.Getenv("AZURE_AI_API_VERSION"),
		AzureAIAPIStyle:           os.Getenv("AZURE_AI_API_STYLE"),
		GoogleBaseURL:             getenv("GOOGLE_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		GoogleAPIKey:              os.Getenv("GOOGLE_API_KEY"),
		HuggingFaceBaseURL:        getenv("HUGGINGFACE_BASE_URL", "https://router.huggingface.co"),
		HuggingFaceAPIKey:         os.Getenv("HUGGINGFACE_API_KEY"),
		ModelAuthority:            modelAuthority,
		EmbeddingModel:            embeddingModelForProvider(embeddingProvider, providerModels),
		WorkerCount:               getenvInt("WORKER_COUNT", 2),
		CodingFragmentConcurrency: getenvInt("CODING_FRAGMENT_CONCURRENCY", defaultCodingFragmentConcurrency(provider)),
		WorkerPollInterval:        getenvDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		RequestTimeout:            getenvDuration("REQUEST_TIMEOUT", 10*time.Minute),
		RealtimeStreamMaxAge:      getenvDuration("REALTIME_STREAM_MAX_AGE", 10*time.Minute),
		RealtimeHeartbeat:         getenvDuration("REALTIME_HEARTBEAT", 25*time.Second),
		RealtimeWriteTimeout:      getenvDuration("REALTIME_WRITE_TIMEOUT", 10*time.Second),
		RedisURL:                  getenv("REDIS_URL", ""),
		UISessionTTL:              getenvDuration("UI_SESSION_TTL", 30*time.Minute),
		InferenceContextTokens:    getenvInt("INFERENCE_CONTEXT_TOKENS", llm.DefaultInferenceContextTokens),
	}

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
	if provider == "ollama" {
		return 1
	}
	return 4
}
