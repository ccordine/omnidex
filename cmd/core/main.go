package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/secrets"
	"github.com/gryph/omnidex/internal/version"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/worker"
)

func main() {
	if len(os.Args) > 1 {
		switch {
		case len(os.Args) == 3 && os.Args[1] == "config:validate-file":
			if err := validateCoreEnvironmentFile(os.Args[2]); err != nil {
				log.Fatalf("config validation error: %v", err)
			}
			log.Printf("configuration is valid")
			return
		case len(os.Args) == 2 && os.Args[1] == "database:preserve-legacy-public":
			if err := runLegacyPublicPreservationCommand(); err != nil {
				log.Fatalf("legacy public preservation error: %v", err)
			}
			return
		default:
			log.Fatalf("unsupported core command")
		}
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	log.Printf("omnidex core %s", version.Full())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var repo *queue.Repository
	var roleplaySimulation *roleplay.Store
	var migrationBundle queue.MigrationBundle
	if !cfg.WrapperOnly {
		migrationBundle, err = loadCoreMigrationBundle()
		if err != nil {
			log.Fatalf("migration authority error: %v", err)
		}
		pool, err := db.ConnectRuntime(ctx, cfg.DatabaseURL, cfg.DatabaseSchema)
		if err != nil {
			log.Fatalf("database connection error: %v", err)
		}
		defer pool.Close()

		repo = queue.New(pool)
		roleplaySimulation, err = roleplay.NewStore(pool)
		if err != nil {
			log.Fatalf("roleplay simulation store error: %v", err)
		}
		if cfg.MigrateOnStartup {
			if err := repo.EnsureSchema(ctx, migrationBundle); err != nil {
				log.Fatalf("schema migration error: %v", err)
			}
		}
		if err := repo.ValidateRuntimeAuthority(ctx); err != nil {
			log.Fatalf("runtime authority error: %v", err)
		}
		secretResolver := secrets.NewResolver(repo)
		secrets.SetGlobal(secretResolver)
		secrets.OverlayConfig(&cfg, secretResolver)
	}
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("config validation error: %v", err)
	}

	llmTransports := llmprovider.NewLazyFromConfig(cfg)
	var webSearchService *websearch.Service
	providers := make([]websearch.ProviderID, len(cfg.WebSearchProviders))
	for index, provider := range cfg.WebSearchProviders {
		providers[index] = websearch.ProviderID(provider)
	}
	webSearchService, err = websearch.New(runtimeWebSearchConfig(cfg, providers))
	if err != nil {
		log.Fatalf("web search configuration error: %v", err)
	}
	browserContextRelevance, browserContextModel, err := runtimeBrowserContextRelevance(cfg)
	if err != nil {
		log.Fatalf("browser context relevance configuration error: %v", err)
	}

	httpServer := api.NewServerWithOptions(repo, llmTransports.Embeddings, api.ServerOptions{
		LifecycleContext:        ctx,
		MigrationBundle:         migrationBundle,
		ProviderConfig:          cfg,
		RequestTimeout:          cfg.RequestTimeout,
		WebSearchProviders:      cfg.WebSearchProviders,
		CoreURL:                 cfg.CoreURL,
		ListenAddr:              cfg.ListenAddr,
		HostAgentURL:            cfg.HostAgentURL,
		HostAgentToken:          cfg.HostAgentToken,
		IntegrationAPIToken:     cfg.IntegrationAPIToken,
		RealtimeMaxClients:      cfg.RealtimeMaxClients,
		RealtimeStreamMaxAge:    cfg.RealtimeStreamMaxAge,
		RealtimeHeartbeat:       cfg.RealtimeHeartbeat,
		RealtimeWriteTimeout:    cfg.RealtimeWriteTimeout,
		RedisURL:                cfg.RedisURL,
		UIRedisRequired:         cfg.UIRedisRequired,
		UISessionTTL:            cfg.UISessionTTL,
		RoleplaySimulation:      roleplaySimulation,
		BrowserContextRelevance: browserContextRelevance,
		BrowserContextModel:     browserContextModel,
	})
	if !cfg.WrapperOnly {
		workerService, err := worker.New(
			repo,
			llmTransports.Stations,
			llmTransports.Embeddings,
			webSearchService,
			worker.Options{
				WorkerCount:            cfg.WorkerCount,
				FragmentConcurrency:    cfg.CodingFragmentConcurrency,
				PollInterval:           cfg.WorkerPollInterval,
				InferenceContextTokens: cfg.InferenceContextTokens,
				InferenceProvider:      cfg.LLMProvider,
				EmbeddingProvider:      cfg.EmbeddingProvider,
				EmbeddingModel:         cfg.EmbeddingModel,
				Models: worker.ModelRouting{
					Stations: cfg.StationModels,
				},
				Workspace: worker.WorkspaceSettings{
					Root:     cfg.WorkspaceRoot,
					HostRoot: cfg.WorkspaceHostRoot,
				},
				Logger:                  log.Default(),
				OnJobFinished:           httpServer.OnJobFinishedAsync,
				OnJobOutput:             httpServer.OnJobOutputAsync,
				BrowserContextRelevance: browserContextRelevance,
			},
		)
		if err != nil {
			log.Fatalf("configure worker: %v", err)
		}
		go func() {
			if err := workerService.Start(ctx); err != nil {
				log.Printf("worker service stopped: %v", err)
				cancel()
			}
		}()
		if err := httpServer.StartScrumAutoWorkLoop(ctx); err != nil {
			log.Fatalf("start Scrum auto-work loop: %v", err)
		}
	}
	log.Printf("core listening on %s core_url=%s llm_provider=%s wrapper_only=%t", cfg.ListenAddr, cfg.CoreURL, cfg.LLMProvider, cfg.WrapperOnly)
	if err := api.Run(ctx, cfg.ListenAddr, httpServer.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func runtimeWebSearchConfig(cfg config.Config, providers []websearch.ProviderID) websearch.Config {
	return websearch.Config{
		Providers:                append([]websearch.ProviderID(nil), providers...),
		Timeout:                  cfg.WebSearchTimeout,
		PerDocumentBytes:         cfg.WebSearchPerSourceBudget,
		TotalDocumentBytes:       cfg.WebSearchTotalBudget,
		MaxCandidatesPerProvider: 8,
		MaxCandidates:            16,
		MaxDocuments:             2,
		MaxResponseBytes:         1 << 20,
	}
}
