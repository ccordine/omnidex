package config

import (
	"os"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/modelconfig"
)

type Config struct {
	ListenAddr              string
	CoreURL                 string
	HostAgentURL            string
	HostAgentToken          string
	HostDirectoryAccessRoot string
	IntegrationAPIToken     string
	DatabaseURL             string
	DatabaseSchema          string
	LLMProvider             string
	EmbeddingProvider       string
	ProviderModels          map[string]ProviderModelConfig
	OllamaBaseURL           string
	CompatibleProviders     map[string]CompatibleProviderConfig
	AzureAIBaseURL          string
	AzureAIAPIKey           string
	AzureAIAPIVersion       string
	AzureAIAPIStyle         string
	GoogleBaseURL           string
	GoogleAPIKey            string
	HuggingFaceBaseURL      string
	HuggingFaceAPIKey       string
	ModelAuthority          modelconfig.Authority
	EmbeddingModel          string
	WorkerPollInterval      string
	RequestTimeout          string
	RealtimeStreamMaxAge    string
	RealtimeHeartbeat       string
	RealtimeWriteTimeout    string
	RedisURL                string
	UISessionTTL            string
	InferenceContextTokens  string
}

// Load parses the environment and preserves provider selections and credentials
// without resolving them; provider validation belongs to the first actual
// provider operation.
func Load() Config {
	provider, embeddingProvider := loadProviderSelection()
	compatibleProviders := loadCompatibleProviderConfigs()
	providerModels := loadProviderModelConfigs()
	modelAuthority := modelconfig.LoadEnvironment()
	cfg := Config{
		ListenAddr:              getenv("LISTEN_ADDR", "0.0.0.0:8090"),
		HostAgentURL:            getenv("HOST_AGENT_URL", ""),
		HostAgentToken:          getenv("HOST_AGENT_TOKEN", ""),
		HostDirectoryAccessRoot: os.Getenv("HOST_DIRECTORY_ACCESS_ROOT"),
		IntegrationAPIToken:     os.Getenv("OMNIDEX_INTEGRATION_API_TOKEN"),
		CoreURL:                 os.Getenv("CORE_URL"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		DatabaseSchema:          getenv("DATABASE_SCHEMA", db.DefaultRuntimeSchema),
		LLMProvider:             provider,
		EmbeddingProvider:       embeddingProvider,
		ProviderModels:          providerModels,
		OllamaBaseURL:           getenv("OLLAMA_BASE_URL", ""),
		CompatibleProviders:     compatibleProviders,
		AzureAIBaseURL:          os.Getenv("AZURE_AI_BASE_URL"),
		AzureAIAPIKey:           os.Getenv("AZURE_AI_API_KEY"),
		AzureAIAPIVersion:       os.Getenv("AZURE_AI_API_VERSION"),
		AzureAIAPIStyle:         os.Getenv("AZURE_AI_API_STYLE"),
		GoogleBaseURL:           getenv("GOOGLE_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		GoogleAPIKey:            os.Getenv("GOOGLE_API_KEY"),
		HuggingFaceBaseURL:      getenv("HUGGINGFACE_BASE_URL", "https://router.huggingface.co"),
		HuggingFaceAPIKey:       os.Getenv("HUGGINGFACE_API_KEY"),
		ModelAuthority:          modelAuthority,
		EmbeddingModel:          embeddingModelForProvider(embeddingProvider, providerModels),
		WorkerPollInterval:      getenv("WORKER_POLL_INTERVAL", "2s"),
		RequestTimeout:          getenv("REQUEST_TIMEOUT", "10m"),
		RealtimeStreamMaxAge:    getenv("REALTIME_STREAM_MAX_AGE", "10m"),
		RealtimeHeartbeat:       getenv("REALTIME_HEARTBEAT", "25s"),
		RealtimeWriteTimeout:    getenv("REALTIME_WRITE_TIMEOUT", "10s"),
		RedisURL:                getenv("REDIS_URL", ""),
		UISessionTTL:            getenv("UI_SESSION_TTL", "30m"),
		InferenceContextTokens:  getenv("INFERENCE_CONTEXT_TOKENS", "8192"),
	}

	return cfg
}
