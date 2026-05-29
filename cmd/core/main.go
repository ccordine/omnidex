package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/secrets"
	"github.com/gryph/omnidex/internal/version"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/worker"
)

func envOrFallback(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	log.Printf("omnidex core %s", version.Full())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var repo *queue.Repository
	if !cfg.WrapperOnly {
		pool, err := db.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("database connection error: %v", err)
		}
		defer pool.Close()

		repo = queue.New(pool)
		if cfg.MigrateOnStartup {
			if err := repo.EnsureSchema(ctx); err != nil {
				log.Fatalf("schema migration error: %v", err)
			}
		}
		secretResolver := secrets.NewResolver(repo)
		secrets.SetGlobal(secretResolver)
		secrets.OverlayConfig(&cfg, secretResolver)
	}

	llmClient, err := llmprovider.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("llm provider error: %v", err)
	}
	var webSearchService *websearch.Service
	if cfg.WebSearchEnabled {
		webSearchService = websearch.New(
			cfg.WebSearchProviders,
			cfg.WebSearchTimeout,
			cfg.WebSearchPerSourceBudget,
			cfg.WebSearchTotalBudget,
		)
	}

	if !cfg.WrapperOnly {
		workerService := worker.New(
			repo,
			llmClient,
			webSearchService,
			worker.Options{
				WorkerCount:    cfg.WorkerCount,
				PollInterval:   cfg.WorkerPollInterval,
				RetrievalLimit: cfg.RetrievalLimit,
				ContextBudget:  cfg.ContextCharBudget,
				Models: worker.ModelRouting{
					Default:    cfg.DefaultModel,
					Fast:       cfg.FastModel,
					Reasoning:  cfg.ReasoningModel,
					Tagging:    cfg.TaggingModel,
					Plan:       cfg.PlanModel,
					Analyze:    cfg.AnalyzeModel,
					Response:   cfg.ResponseModel,
					Search:     cfg.SearchModel,
					Memory:     cfg.MemoryModel,
					Specialist: cfg.SpecialistModels,
				},
				Cognition: worker.CognitionSettings{
					StopOnSufficientContext: cfg.StopOnSufficientContext,
					SufficientContextChars:  cfg.SufficientContextChars,
					MemoryInferenceEnabled:  cfg.MemoryInferenceEnabled,
					MemoryInferenceMaxItems: cfg.MemoryInferenceMaxItems,
				},
				Tournament: worker.TournamentSettings{
					Enabled:       cfg.TournamentEnabled,
					ChunkChars:    cfg.TournamentChunkChars,
					SummaryChars:  cfg.TournamentSummaryChars,
					MaxRounds:     cfg.TournamentMaxRounds,
					VerifySupport: cfg.TournamentVerify,
				},
				Workspace: worker.WorkspaceSettings{
					Enabled:       cfg.WorkspaceScanEnabled,
					Root:          cfg.WorkspaceRoot,
					MaxFiles:      cfg.WorkspaceMaxFiles,
					ContextBudget: cfg.WorkspaceContextBudget,
				},
				HallucinationRetryLimit: cfg.HallucinationRetryLimit,
				OllamaRestartCommand:    cfg.OllamaRestartCommand,
				OllamaRestartTimeout:    cfg.OllamaRestartTimeout,
				V3Enabled:               cfg.V3Enabled,
				SkillsRoot:              cfg.SkillsRoot,
				Logger:                  log.Default(),
			},
		)
		go workerService.Start(ctx)
	}

	ollamaDefaultModel := envOrFallback("OLLAMA_MODEL", "")
	openAIDefaultModel := envOrFallback("OPENAI_MODEL", "")
	azureAIDefaultModel := envOrFallback("AZURE_AI_MODEL", envOrFallback("AZURE_OPENAI_DEPLOYMENT", ""))
	xAIDefaultModel := envOrFallback("XAI_MODEL", envOrFallback("GROK_MODEL", ""))
	googleDefaultModel := envOrFallback("GOOGLE_MODEL", envOrFallback("GEMINI_MODEL", ""))
	anthropicDefaultModel := envOrFallback("ANTHROPIC_MODEL", envOrFallback("CLAUDE_MODEL", ""))
	huggingFaceDefaultModel := envOrFallback("HUGGINGFACE_MODEL", envOrFallback("HF_MODEL", ""))
	ollamaEmbeddingModel := envOrFallback("OLLAMA_EMBEDDING_MODEL", "")
	openAIEmbeddingModel := envOrFallback("OPENAI_EMBEDDING_MODEL", "")
	azureAIEmbeddingModel := envOrFallback("AZURE_AI_EMBEDDING_MODEL", envOrFallback("AZURE_OPENAI_EMBEDDING_DEPLOYMENT", ""))
	googleEmbeddingModel := envOrFallback("GOOGLE_EMBEDDING_MODEL", envOrFallback("GEMINI_EMBEDDING_MODEL", ""))
	huggingFaceEmbeddingModel := envOrFallback("HUGGINGFACE_EMBEDDING_MODEL", envOrFallback("HF_EMBEDDING_MODEL", ""))

	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "ollama") {
		ollamaDefaultModel = envOrFallback("OLLAMA_MODEL", cfg.DefaultModel)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "openai") {
		openAIDefaultModel = envOrFallback("OPENAI_MODEL", cfg.DefaultModel)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "azure") {
		azureAIDefaultModel = envOrFallback("AZURE_AI_MODEL", envOrFallback("AZURE_OPENAI_DEPLOYMENT", cfg.DefaultModel))
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "xai") {
		xAIDefaultModel = envOrFallback("XAI_MODEL", envOrFallback("GROK_MODEL", cfg.DefaultModel))
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "google") {
		googleDefaultModel = envOrFallback("GOOGLE_MODEL", envOrFallback("GEMINI_MODEL", cfg.DefaultModel))
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "anthropic") {
		anthropicDefaultModel = envOrFallback("ANTHROPIC_MODEL", envOrFallback("CLAUDE_MODEL", cfg.DefaultModel))
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLMProvider), "huggingface") {
		huggingFaceDefaultModel = envOrFallback("HUGGINGFACE_MODEL", envOrFallback("HF_MODEL", cfg.DefaultModel))
	}
	switch strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider)) {
	case "ollama":
		ollamaEmbeddingModel = envOrFallback("OLLAMA_EMBEDDING_MODEL", cfg.EmbeddingModel)
	case "openai":
		openAIEmbeddingModel = envOrFallback("OPENAI_EMBEDDING_MODEL", cfg.EmbeddingModel)
	case "azure":
		azureAIEmbeddingModel = envOrFallback("AZURE_AI_EMBEDDING_MODEL", envOrFallback("AZURE_OPENAI_EMBEDDING_DEPLOYMENT", cfg.EmbeddingModel))
	case "google":
		googleEmbeddingModel = envOrFallback("GOOGLE_EMBEDDING_MODEL", envOrFallback("GEMINI_EMBEDDING_MODEL", cfg.EmbeddingModel))
	case "huggingface":
		huggingFaceEmbeddingModel = envOrFallback("HUGGINGFACE_EMBEDDING_MODEL", envOrFallback("HF_EMBEDDING_MODEL", cfg.EmbeddingModel))
	}

	httpServer := api.NewServerWithOptions(repo, llmClient, api.ServerOptions{
		DefaultProvider:           cfg.LLMProvider,
		RequestTimeout:            cfg.RequestTimeout,
		V3Enabled:                 cfg.V3Enabled,
		OllamaBaseURL:             cfg.OllamaBaseURL,
		OllamaDefaultModel:        ollamaDefaultModel,
		OllamaEmbeddingModel:      ollamaEmbeddingModel,
		OpenAIBaseURL:             cfg.OpenAIBaseURL,
		OpenAIAPIKey:              cfg.OpenAIAPIKey,
		OpenAIOrganization:        cfg.OpenAIOrganization,
		OpenAIProject:             cfg.OpenAIProject,
		OpenAIDefaultModel:        openAIDefaultModel,
		OpenAIEmbeddingModel:      openAIEmbeddingModel,
		AzureAIBaseURL:            cfg.AzureAIBaseURL,
		AzureAIAPIKey:             cfg.AzureAIAPIKey,
		AzureAIAPIVersion:         cfg.AzureAIAPIVersion,
		AzureAIAPIStyle:           cfg.AzureAIAPIStyle,
		AzureAIDefaultModel:       azureAIDefaultModel,
		AzureAIEmbeddingModel:     azureAIEmbeddingModel,
		XAIBaseURL:                cfg.XAIBaseURL,
		XAIAPIKey:                 cfg.XAIAPIKey,
		XAIDefaultModel:           xAIDefaultModel,
		GoogleBaseURL:             cfg.GoogleBaseURL,
		GoogleAPIKey:              cfg.GoogleAPIKey,
		GoogleDefaultModel:        googleDefaultModel,
		GoogleEmbeddingModel:      googleEmbeddingModel,
		AnthropicBaseURL:          cfg.AnthropicBaseURL,
		AnthropicAPIKey:           cfg.AnthropicAPIKey,
		AnthropicVersion:          cfg.AnthropicVersion,
		AnthropicMaxTokens:        cfg.AnthropicMaxTokens,
		AnthropicDefaultModel:     anthropicDefaultModel,
		HuggingFaceBaseURL:        cfg.HuggingFaceBaseURL,
		HuggingFaceAPIKey:         cfg.HuggingFaceAPIKey,
		HuggingFaceDefaultModel:   huggingFaceDefaultModel,
		HuggingFaceEmbeddingModel: huggingFaceEmbeddingModel,
		WebSearchEnabled:          cfg.WebSearchEnabled,
		WebSearchProviders:        cfg.WebSearchProviders,
		WebSearchTimeout:          cfg.WebSearchTimeout,
		CoreURL:                   cfg.CoreURL,
		ListenAddr:                cfg.ListenAddr,
	})
	log.Printf("core listening on %s core_url=%s llm_provider=%s wrapper_only=%t", cfg.ListenAddr, cfg.CoreURL, cfg.LLMProvider, cfg.WrapperOnly)
	if err := api.Run(ctx, cfg.ListenAddr, httpServer.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
