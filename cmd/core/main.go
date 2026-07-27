package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/secrets"
	"github.com/gryph/omnidex/internal/version"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/worker"
)

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
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config validation error: %v", err)
	}

	if shouldResolveOllamaEndpoint(cfg) {
		resolveCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		resolved, err := ollama.ResolveReachableBaseURL(resolveCtx, cfg.OllamaBaseURL, 4*time.Second)
		cancel()
		if err != nil {
			log.Printf("ollama startup probe failed (jobs may fail until OLLAMA_BASE_URL is reachable): %v", err)
		} else if resolved != strings.TrimSpace(cfg.OllamaBaseURL) {
			log.Printf("ollama endpoint resolved %s -> %s", cfg.OllamaBaseURL, resolved)
			cfg.OllamaBaseURL = resolved
		} else {
			log.Printf("ollama endpoint reachable at %s", resolved)
		}
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

	httpServer := api.NewServerWithOptions(repo, llmClient, api.ServerOptions{
		LifecycleContext:     ctx,
		ProviderConfig:       cfg,
		RequestTimeout:       cfg.RequestTimeout,
		V3Enabled:            cfg.V3Enabled,
		WebSearchEnabled:     cfg.WebSearchEnabled,
		WebSearchProviders:   cfg.WebSearchProviders,
		WebSearchTimeout:     cfg.WebSearchTimeout,
		CoreURL:              cfg.CoreURL,
		ListenAddr:           cfg.ListenAddr,
		HostAgentURL:         cfg.HostAgentURL,
		HostAgentToken:       cfg.HostAgentToken,
		RealtimeMaxClients:   cfg.RealtimeMaxClients,
		RealtimeStreamMaxAge: cfg.RealtimeStreamMaxAge,
		RealtimeHeartbeat:    cfg.RealtimeHeartbeat,
		RealtimeWriteTimeout: cfg.RealtimeWriteTimeout,
		RedisURL:             cfg.RedisURL,
		UIRedisRequired:      cfg.UIRedisRequired,
		UISessionTTL:         cfg.UISessionTTL,
	})
	if !cfg.WrapperOnly {
		workerService, err := worker.New(
			repo,
			llmClient,
			webSearchService,
			worker.Options{
				WorkerCount:            cfg.WorkerCount,
				PollInterval:           cfg.WorkerPollInterval,
				RetrievalLimit:         cfg.RetrievalLimit,
				ContextBudget:          cfg.ContextCharBudget,
				InferenceContextTokens: cfg.InferenceContextTokens,
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
				OllamaBaseURL:           cfg.OllamaBaseURL,
				V3Enabled:               cfg.V3Enabled,
				SkillsRoot:              cfg.SkillsRoot,
				Logger:                  log.Default(),
				OnJobFinished:           httpServer.OnJobFinishedAsync,
				OnJobOutput:             httpServer.OnJobOutputAsync,
			},
		)
		if err != nil {
			log.Fatalf("configure worker: %v", err)
		}
		go workerService.Start(ctx)
		if err := httpServer.StartScrumAutoWorkLoop(ctx); err != nil {
			log.Fatalf("start Scrum auto-work loop: %v", err)
		}
	}
	log.Printf("core listening on %s core_url=%s llm_provider=%s wrapper_only=%t", cfg.ListenAddr, cfg.CoreURL, cfg.LLMProvider, cfg.WrapperOnly)
	if err := api.Run(ctx, cfg.ListenAddr, httpServer.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func shouldResolveOllamaEndpoint(cfg config.Config) bool {
	provider := strings.ToLower(strings.TrimSpace(cfg.LLMProvider))
	embedding := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	return provider == "ollama" || embedding == "ollama"
}
