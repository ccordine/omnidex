package config

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/specialist"
)

func TestLoadChineseOpenAICompatibleProviders(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		canonical string
		apiKeyEnv string
		modelEnv  string
		baseURL   string
	}{
		{name: "deepseek", provider: "deep-seek", canonical: "deepseek", apiKeyEnv: "DEEPSEEK_API_KEY", modelEnv: "DEEPSEEK_MODEL", baseURL: "https://api.deepseek.com"},
		{name: "qwen", provider: "dashscope", canonical: "qwen", apiKeyEnv: "DASHSCOPE_API_KEY", modelEnv: "DASHSCOPE_MODEL", baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{name: "moonshot", provider: "kimi", canonical: "moonshot", apiKeyEnv: "MOONSHOT_API_KEY", modelEnv: "MOONSHOT_MODEL", baseURL: "https://api.moonshot.ai/v1"},
		{name: "zhipu", provider: "glm", canonical: "zhipu", apiKeyEnv: "ZHIPU_API_KEY", modelEnv: "ZHIPU_MODEL", baseURL: "https://open.bigmodel.cn/api/paas/v4"},
		{name: "zai", provider: "z-ai", canonical: "zai", apiKeyEnv: "ZAI_API_KEY", modelEnv: "ZAI_MODEL", baseURL: "https://api.z.ai/api/paas/v4"},
		{name: "minimax", provider: "mini-max", canonical: "minimax", apiKeyEnv: "MINIMAX_API_KEY", modelEnv: "MINIMAX_MODEL", baseURL: "https://api.minimax.io/v1"},
		{name: "qianfan", provider: "ernie", canonical: "qianfan", apiKeyEnv: "QIANFAN_API_KEY", modelEnv: "QIANFAN_MODEL", baseURL: "https://qianfan.baidubce.com/v2"},
		{name: "hunyuan", provider: "tencent", canonical: "hunyuan", apiKeyEnv: "HUNYUAN_API_KEY", modelEnv: "HUNYUAN_MODEL", baseURL: "https://api.hunyuan.cloud.tencent.com/v1"},
		{name: "doubao", provider: "ark", canonical: "doubao", apiKeyEnv: "ARK_API_KEY", modelEnv: "ARK_MODEL", baseURL: "https://ark.cn-beijing.volces.com/api/v3"},
		{name: "stepfun", provider: "step", canonical: "stepfun", apiKeyEnv: "STEPFUN_API_KEY", modelEnv: "STEPFUN_MODEL", baseURL: "https://api.stepfun.com/v1"},
		{name: "yi", provider: "01-ai", canonical: "yi", apiKeyEnv: "YI_API_KEY", modelEnv: "YI_MODEL", baseURL: "https://api.01.ai/v1"},
		{name: "baichuan", provider: "baichuan-ai", canonical: "baichuan", apiKeyEnv: "BAICHUAN_API_KEY", modelEnv: "BAICHUAN_MODEL", baseURL: "https://api.baichuan-ai.com/v1"},
		{name: "spark", provider: "iflytek", canonical: "spark", apiKeyEnv: "SPARK_API_KEY", modelEnv: "SPARK_MODEL", baseURL: "https://spark-api-open.xf-yun.com/v1"},
		{name: "siliconflow", provider: "silicon-flow", canonical: "siliconflow", apiKeyEnv: "SILICONFLOW_API_KEY", modelEnv: "SILICONFLOW_MODEL", baseURL: "https://api.siliconflow.cn/v1"},
		{name: "modelscope", provider: "model-scope", canonical: "modelscope", apiKeyEnv: "MODELSCOPE_API_KEY", modelEnv: "MODELSCOPE_MODEL", baseURL: "https://api-inference.modelscope.cn/v1"},
		{name: "modelarts", provider: "huawei-maas", canonical: "modelarts", apiKeyEnv: "MODELARTS_API_KEY", modelEnv: "MODELARTS_MODEL", baseURL: "https://api.modelarts-maas.com/openai/v1"},
		{name: "mimo", provider: "xiaomi", canonical: "mimo", apiKeyEnv: "MIMO_API_KEY", modelEnv: "MIMO_MODEL", baseURL: "https://api.xiaomimimo.com/v1"},
		{name: "longcat", provider: "meituan", canonical: "longcat", apiKeyEnv: "LONGCAT_API_KEY", modelEnv: "LONGCAT_MODEL", baseURL: "https://api.longcat.chat/openai/v1"},
		{name: "antling", provider: "inclusion-ai", canonical: "antling", apiKeyEnv: "ANTLING_API_KEY", modelEnv: "ANTLING_MODEL", baseURL: "https://api.ant-ling.com/v1"},
		{name: "tokenhub", provider: "tencent-tokenhub", canonical: "tokenhub", apiKeyEnv: "TOKENHUB_API_KEY", modelEnv: "TOKENHUB_MODEL", baseURL: "https://tokenhub.tencentmaas.com/v1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRemoteProviderTestEnvironment(t)
			t.Setenv("LLM_PROVIDER", test.provider)
			t.Setenv(test.apiKeyEnv, "provider-test-key")
			t.Setenv(test.modelEnv, "provider-test-model")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.LLMProvider != test.canonical {
				t.Fatalf("LLMProvider=%q want %q", cfg.LLMProvider, test.canonical)
			}
			if cfg.DefaultModel != "provider-test-model" {
				t.Fatalf("DefaultModel=%q want provider-test-model", cfg.DefaultModel)
			}
			providerConfig, ok := cfg.CompatibleProviders[test.canonical]
			if !ok {
				t.Fatalf("CompatibleProviders missing %q", test.canonical)
			}
			if providerConfig.APIKey != "provider-test-key" {
				t.Fatalf("APIKey not loaded for %q", test.canonical)
			}
			if providerConfig.BaseURL != test.baseURL {
				t.Fatalf("BaseURL=%q want %q", providerConfig.BaseURL, test.baseURL)
			}
			if cfg.ProviderModels[test.canonical].Default != "provider-test-model" {
				t.Fatalf("provider DefaultModel=%q", cfg.ProviderModels[test.canonical].Default)
			}
			if cfg.CodingFragmentConcurrency != 4 {
				t.Fatalf("remote fragment concurrency=%d want 4", cfg.CodingFragmentConcurrency)
			}
		})
	}
}

func TestLoadChineseProviderRequiresCurrentModelID(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "deepseek")
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_MODEL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DEEPSEEK_MODEL") {
		t.Fatalf("Load() error=%v, want explicit DEEPSEEK_MODEL failure", err)
	}
}

func TestLoadDedicatedSubtaskExecutorModel(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL", "qwen2.5-coder:14b")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_SUBTASK_EXECUTOR", "qwen3-coder:30b")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.SpecialistModels[specialist.RoleSubtaskExecutorSpecialist]; got != "qwen3-coder:30b" {
		t.Fatalf("subtask executor model=%q, want dedicated configured model", got)
	}
}

func TestLoadDedicatedCodingAssemblyModels(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL", "qwen2.5-coder:14b")
	t.Setenv("OLLAMA_MODEL_GLUE", "qwen2.5-coder:3b")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_SURFACE", "qwen3:4b-thinking")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_PRODUCT_IDENTITY", "qwen2.5-coder:14b-identity")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_PARTITION", "qwen2.5-coder:7b-partition")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER", "deepseek-r1:8b")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_SPLIT", "qwen2.5-coder:7b-split")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_ARTIFACT_HANDLING", "qwen2.5:3b-artifact")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_CAPABILITY_RELATION", "qwen3:4b-relation")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_SKILL_SELECTION", "qwen3:4b-skill-selection")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_SKILL_PROCEDURE", "qwen2.5-coder:7b-skill-procedure")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT", "qwen3-coder:30b")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT_CORRECTION", "qwen2.5-coder:14b-correction")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingSurfaceStation]; got != "qwen3:4b-thinking" {
		t.Fatalf("coding surface model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingProductIdentityStation]; got != "qwen2.5-coder:14b-identity" {
		t.Fatalf("coding product identity model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingRequirementPartitionStation]; got != "qwen2.5-coder:7b-partition" {
		t.Fatalf("coding requirement partition model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingRequirementAdviserStation]; got != "deepseek-r1:8b" {
		t.Fatalf("coding requirement adviser model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingRequirementSplitStation]; got != "qwen2.5-coder:7b-split" {
		t.Fatalf("coding requirement split model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingArtifactHandlingStation]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact handling model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingCapabilityRelationStation]; got != "qwen3:4b-relation" {
		t.Fatalf("coding capability relation model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingSkillSelectionStation]; got != "qwen3:4b-skill-selection" {
		t.Fatalf("coding skill selection model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingSkillProcedureStation]; got != "qwen2.5-coder:7b-skill-procedure" {
		t.Fatalf("coding skill procedure model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingFragmentStation]; got != "qwen3-coder:30b" {
		t.Fatalf("coding fragment model=%q want dedicated override", got)
	}
	if got := cfg.SpecialistModels[specialist.RoleCodingFragmentCorrectionStation]; got != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding fragment correction model=%q want dedicated override", got)
	}
	if cfg.CodingFragmentConcurrency != 1 {
		t.Fatalf("local fragment concurrency=%d want 1", cfg.CodingFragmentConcurrency)
	}
}

func TestLoadRejectsUnsafeCodingFragmentConcurrency(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL", "qwen2.5-coder:7b")
	t.Setenv("CODING_FRAGMENT_CONCURRENCY", "5")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CODING_FRAGMENT_CONCURRENCY") {
		t.Fatalf("Load() error=%v, want fragment concurrency bound", err)
	}
}

func TestLoadGenerationOnlyProviderRequiresEmbeddingProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "deepseek")
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_MODEL", "current-model")
	t.Setenv("EMBEDDING_PROVIDER", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "EMBEDDING_PROVIDER is required") {
		t.Fatalf("Load() error=%v, want explicit EMBEDDING_PROVIDER failure", err)
	}
}

func TestLoadCustomCompatibleProviderRequiresCompleteEndpoint(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "openai-compatible")
	t.Setenv("COMPATIBLE_API_KEY", "test-key")
	t.Setenv("COMPATIBLE_MODEL", "custom-model")
	t.Setenv("COMPATIBLE_BASE_URL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "COMPATIBLE_BASE_URL") {
		t.Fatalf("Load() error=%v, want COMPATIBLE_BASE_URL failure", err)
	}
}

func TestLoadRejectsMalformedCompatibleProviderURL(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "deepseek")
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_MODEL", "current-model")
	t.Setenv("DEEPSEEK_BASE_URL", "api.deepseek.com/v1")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DEEPSEEK_BASE_URL") {
		t.Fatalf("Load() error=%v, want absolute URL failure", err)
	}
}

func TestLoadCompatibleEmbeddingProviderRequiresEmbeddingModel(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "qwen")
	t.Setenv("EMBEDDING_PROVIDER", "qwen")
	t.Setenv("QWEN_API_KEY", "test-key")
	t.Setenv("QWEN_MODEL", "current-chat-model")
	t.Setenv("QWEN_EMBEDDING_MODEL", "")
	t.Setenv("EMBEDDING_MODEL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "QWEN_EMBEDDING_MODEL") {
		t.Fatalf("Load() error=%v, want explicit QWEN_EMBEDDING_MODEL failure", err)
	}
}

func setRemoteProviderTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")
}

func TestLoadOpenAIRequiresAPIKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() structural validation error: %v", err)
	}
	err = Validate(cfg)
	if err == nil {
		t.Fatalf("expected credential validation error when OPENAI_API_KEY is missing")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY error, got: %v", err)
	}
}

func TestLoadAllowsDatabaseCredentialOverlayBeforeFinalValidation(t *testing.T) {
	setRemoteProviderTestEnvironment(t)
	t.Setenv("LLM_PROVIDER", "qwen")
	t.Setenv("QWEN_API_KEY", "")
	t.Setenv("QWEN_MODEL", "qwen-current")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() must permit the database secret overlay phase: %v", err)
	}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "QWEN_API_KEY") {
		t.Fatalf("Validate() before secret overlay error=%v", err)
	}
	provider := cfg.CompatibleProviders["qwen"]
	provider.APIKey = "database-qwen-key"
	cfg.CompatibleProviders["qwen"] = provider
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() after database secret overlay: %v", err)
	}
}

func TestLoadOpenAIUsesOpenAIModelDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_MODEL_FAST", "")
	t.Setenv("OPENAI_MODEL_REASONING", "")
	t.Setenv("EMBEDDING_MODEL", "legacy-embed")
	t.Setenv("OLLAMA_MODEL", "llama3.2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DefaultModel != "gpt-4.1-mini" {
		t.Fatalf("DefaultModel=%q want %q", cfg.DefaultModel, "gpt-4.1-mini")
	}
	if cfg.FastModel != "gpt-4.1-mini" {
		t.Fatalf("FastModel=%q want %q", cfg.FastModel, "gpt-4.1-mini")
	}
	if cfg.ReasoningModel != "gpt-4.1-mini" {
		t.Fatalf("ReasoningModel=%q want %q", cfg.ReasoningModel, "gpt-4.1-mini")
	}
	if cfg.EmbeddingModel != "legacy-embed" {
		t.Fatalf("EmbeddingModel=%q want %q", cfg.EmbeddingModel, "legacy-embed")
	}
}

func TestLoadGoogleProviderUsesGoogleModelsAndEmbeddings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_MODEL", "gemini-test")
	t.Setenv("GEMINI_MODEL_FAST", "gemini-fast")
	t.Setenv("GEMINI_EMBEDDING_MODEL", "text-embedding-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "google" {
		t.Fatalf("LLMProvider=%q want google", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "gemini-test" {
		t.Fatalf("DefaultModel=%q want gemini-test", cfg.DefaultModel)
	}
	if cfg.FastModel != "gemini-fast" {
		t.Fatalf("FastModel=%q want gemini-fast", cfg.FastModel)
	}
	if cfg.EmbeddingProvider != "google" {
		t.Fatalf("EmbeddingProvider=%q want google", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "text-embedding-test" {
		t.Fatalf("EmbeddingModel=%q want text-embedding-test", cfg.EmbeddingModel)
	}
}

func TestLoadAnthropicUsesExplicitOllamaEmbeddingProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "claude")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "claude-test")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "anthropic" {
		t.Fatalf("LLMProvider=%q want anthropic", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "claude-test" {
		t.Fatalf("DefaultModel=%q want claude-test", cfg.DefaultModel)
	}
	if cfg.EmbeddingProvider != "ollama" {
		t.Fatalf("EmbeddingProvider=%q want ollama", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-test" {
		t.Fatalf("EmbeddingModel=%q want nomic-test", cfg.EmbeddingModel)
	}
}

func TestLoadHuggingFaceProviderUsesHFTokenAndModel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "hf")
	t.Setenv("HF_TOKEN", "test-token")
	t.Setenv("HF_MODEL", "org/model:fastest")
	t.Setenv("HF_EMBEDDING_MODEL", "sentence-transformers/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "huggingface" {
		t.Fatalf("LLMProvider=%q want huggingface", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "org/model:fastest" {
		t.Fatalf("DefaultModel=%q want org/model:fastest", cfg.DefaultModel)
	}
	if cfg.HuggingFaceAPIKey != "test-token" {
		t.Fatalf("HuggingFaceAPIKey not loaded from HF_TOKEN")
	}
	if cfg.EmbeddingModel != "sentence-transformers/test" {
		t.Fatalf("EmbeddingModel=%q want sentence-transformers/test", cfg.EmbeddingModel)
	}
}

func TestLoadXAIProviderUsesGrokAliasesAndExplicitOllamaEmbeddings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "grock")
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("GROK_API_KEY", "xai-test-key")
	t.Setenv("GROK_MODEL", "grok-test")
	t.Setenv("GROK_MODEL_FAST", "grok-fast")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "xai" {
		t.Fatalf("LLMProvider=%q want xai", cfg.LLMProvider)
	}
	if cfg.DefaultModel != "grok-test" {
		t.Fatalf("DefaultModel=%q want grok-test", cfg.DefaultModel)
	}
	if cfg.FastModel != "grok-fast" {
		t.Fatalf("FastModel=%q want grok-fast", cfg.FastModel)
	}
	providerConfig, ok := cfg.CompatibleProviders["xai"]
	if !ok {
		t.Fatal("CompatibleProviders missing xai")
	}
	if providerConfig.APIKey != "xai-test-key" {
		t.Fatalf("xAI API key not loaded from GROK_API_KEY")
	}
	if providerConfig.BaseURL != "https://api.x.ai/v1" {
		t.Fatalf("xAI BaseURL=%q want default", providerConfig.BaseURL)
	}
	if cfg.EmbeddingProvider != "ollama" {
		t.Fatalf("EmbeddingProvider=%q want ollama", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-test" {
		t.Fatalf("EmbeddingModel=%q want nomic-test", cfg.EmbeddingModel)
	}
}

func TestLoadAzureProviderUsesMicrosoftAliases(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "windows-ai")
	t.Setenv("AZURE_OPENAI_ENDPOINT", "https://example.openai.azure.com")
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-test-key")
	t.Setenv("AZURE_OPENAI_DEPLOYMENT", "chat-deployment")
	t.Setenv("AZURE_OPENAI_EMBEDDING_DEPLOYMENT", "embed-deployment")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMProvider != "azure" {
		t.Fatalf("LLMProvider=%q want azure", cfg.LLMProvider)
	}
	if cfg.AzureAIBaseURL != "https://example.openai.azure.com" {
		t.Fatalf("AzureAIBaseURL=%q", cfg.AzureAIBaseURL)
	}
	if cfg.AzureAIAPIKey != "azure-test-key" {
		t.Fatalf("AzureAIAPIKey not loaded from AZURE_OPENAI_API_KEY")
	}
	if cfg.DefaultModel != "chat-deployment" {
		t.Fatalf("DefaultModel=%q want chat-deployment", cfg.DefaultModel)
	}
	if cfg.EmbeddingProvider != "azure" {
		t.Fatalf("EmbeddingProvider=%q want azure", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "embed-deployment" {
		t.Fatalf("EmbeddingModel=%q want embed-deployment", cfg.EmbeddingModel)
	}
}

func TestLoadAnthropicCanUseGoogleEmbeddingProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")
	t.Setenv("EMBEDDING_PROVIDER", "google")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	t.Setenv("GOOGLE_EMBEDDING_MODEL", "text-embedding-004")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EmbeddingProvider != "google" {
		t.Fatalf("EmbeddingProvider=%q want google", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "text-embedding-004" {
		t.Fatalf("EmbeddingModel=%q want text-embedding-004", cfg.EmbeddingModel)
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "something-else")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "LLM_PROVIDER") {
		t.Fatalf("expected LLM_PROVIDER error, got: %v", err)
	}
}

func TestLoadWrapperOnlyAllowsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.WrapperOnly {
		t.Fatalf("WrapperOnly=%v want true", cfg.WrapperOnly)
	}
}

func TestLoadRealtimeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("REALTIME_MAX_CLIENTS", "")
	t.Setenv("REALTIME_STREAM_MAX_AGE", "")
	t.Setenv("REALTIME_HEARTBEAT", "")
	t.Setenv("REALTIME_WRITE_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.RealtimeMaxClients != 512 {
		t.Fatalf("RealtimeMaxClients=%d want 512", cfg.RealtimeMaxClients)
	}
	if cfg.RealtimeStreamMaxAge != 10*time.Minute {
		t.Fatalf("RealtimeStreamMaxAge=%s want 10m", cfg.RealtimeStreamMaxAge)
	}
	if cfg.RealtimeHeartbeat != 25*time.Second {
		t.Fatalf("RealtimeHeartbeat=%s want 25s", cfg.RealtimeHeartbeat)
	}
	if cfg.RealtimeWriteTimeout != 10*time.Second {
		t.Fatalf("RealtimeWriteTimeout=%s want 10s", cfg.RealtimeWriteTimeout)
	}
}

func TestLoadRejectsMalformedTypedEnvironment(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "WORKER_COUNT", value: "many"},
		{key: "INFERENCE_CONTEXT_TOKENS", value: "wide"},
		{key: "WRAPPER_ONLY", value: "perhaps"},
		{key: "REQUEST_TIMEOUT", value: "soon"},
		{key: "WEB_SEARCH_PROVIDERS", value: ",,,"},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error=nil, want %s validation failure", test.key)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error=%v, want %s context", err, test.key)
			}
		})
	}
}

func TestLoadRejectsOutOfRangeRuntimeSettings(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "WORKER_COUNT", value: "0"},
		{key: "WORKER_POLL_INTERVAL", value: "0s"},
		{key: "RETRIEVAL_LIMIT", value: "65"},
		{key: "CONTEXT_CHAR_BUDGET", value: "0"},
		{key: "INFERENCE_CONTEXT_TOKENS", value: "4095"},
		{key: "WORKSPACE_MAX_FILES", value: "0"},
		{key: "WORKSPACE_CONTEXT_BUDGET", value: "0"},
		{key: "REALTIME_MAX_CLIENTS", value: "0"},
		{key: "REALTIME_STREAM_MAX_AGE", value: "10s"},
		{key: "REALTIME_HEARTBEAT", value: "1s"},
		{key: "REALTIME_WRITE_TIMEOUT", value: "100ms"},
		{key: "UI_SESSION_TTL", value: "30s"},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error=nil, want %s validation failure", test.key)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error=%v, want %s context", err, test.key)
			}
		})
	}
}

func TestLoadRejectsRemovedDecisionEngineSettings(t *testing.T) {
	for _, key := range removedEnvironmentKeys {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(key, "legacy-value")

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "was removed") {
				t.Fatalf("Load() error=%v, want explicit %s removal failure", err, key)
			}
		})
	}
}
