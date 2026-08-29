package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/secrets"
)

type Server struct {
	lifecycleContext           context.Context
	repo                       *queue.Repository
	channelStore               channelStore
	roleplaySimulation         RoleplaySimulationStore
	enqueueChannelTurn         enqueueChannelTurnFunc
	enqueueRoleplayChannelTurn enqueueRoleplayChannelTurnFunc
	embeddingClient            llm.EmbeddingClient
	mux                        *http.ServeMux
	providerConfig             config.Config
	defaultProvider            string
	requestTimeout             time.Duration
	ollamaBaseURL              string
	ollamaEmbeddingModel       string
	ollamaModelAuthority       OllamaModelAuthority
	ollamaModelLifecycle       OllamaModelLifecycleAuthority
	ollamaDownloads            OllamaDownloadStore
	ollamaCatalog              OllamaCatalogAuthority
	ollamaDownloadMu           sync.Mutex
	ollamaDownloadRunning      map[string]struct{}
	ollamaDownloadSlots        chan struct{}
	webSearchProviders         []string
	secretsResolver            *secrets.Resolver
	coreURLDefault             string
	listenAddr                 string
	realtimeMaxClients         int
	realtimeStreamMaxAge       time.Duration
	realtimeHeartbeat          time.Duration
	realtimeWriteTimeout       time.Duration
	redisURL                   string
	uiRedisRequired            bool
	uiSessionTTL               time.Duration
	uiRedis                    *uiRedisClient
	uiRedisInitError           string
	uiMemoryMu                 sync.RWMutex
	uiMemorySessions           map[string]uiMemorySessionRecord
	roleplaySceneDraftMu       sync.Mutex
	ollamaURLMu                sync.RWMutex
	hostAgentURL               string
	hostAgentToken             string
	integrationAPIToken        string
	realtimeHub                *RealtimeHub
	jobOutputOnce              sync.Once
	jobOutputCoalescer         *jobOutputCoalescer
	telemetryRealtimeOnce      sync.Once
	telemetryRealtime          *telemetryRealtimeCoalescer
	scrumAutoWorkMu            sync.Mutex
	scrumAutoWorkAsyncMu       sync.Mutex
	scrumAutoWorkAsyncRunning  bool
	scrumAutoWorkAsyncPending  bool
}

type ServerOptions struct {
	LifecycleContext     context.Context
	ProviderConfig       config.Config
	RequestTimeout       time.Duration
	WebSearchProviders   []string
	CoreURL              string
	ListenAddr           string
	HostAgentURL         string
	HostAgentToken       string
	IntegrationAPIToken  string
	RealtimeMaxClients   int
	RealtimeStreamMaxAge time.Duration
	RealtimeHeartbeat    time.Duration
	RealtimeWriteTimeout time.Duration
	RedisURL             string
	UIRedisRequired      bool
	UISessionTTL         time.Duration
	RoleplaySimulation   RoleplaySimulationStore
	OllamaModelAuthority OllamaModelAuthority
	OllamaModelLifecycle OllamaModelLifecycleAuthority
	OllamaDownloads      OllamaDownloadStore
	OllamaCatalog        OllamaCatalogAuthority
}

type memoryCandidatePromotionRequest struct {
	Tier      string                         `json:"tier"`
	Authority model.MemoryPromotionAuthority `json:"authority"`
}

type feedbackRequest struct {
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Feedback    string                     `json:"feedback"`
}

type cancelRequest struct {
	OperationID queue.LifecycleOperationID `json:"operation_id"`
	Reason      string                     `json:"reason"`
}

func NewServer(repo *queue.Repository, embeddingClient llm.EmbeddingClient) *Server {
	return NewServerWithOptions(repo, embeddingClient, ServerOptions{})
}

func NewServerWithOptions(repo *queue.Repository, embeddingClient llm.EmbeddingClient, options ServerOptions) *Server {
	lifecycleContext := options.LifecycleContext
	if lifecycleContext == nil {
		lifecycleContext = context.Background()
	}
	providerConfig := options.ProviderConfig
	providerConfig.CompatibleProviders = config.CloneCompatibleProviders(providerConfig.CompatibleProviders)
	providerConfig.ProviderModels = config.CloneProviderModels(providerConfig.ProviderModels)
	defaultProvider := strings.TrimSpace(providerConfig.LLMProvider)
	if definition, ok := catalog.Lookup(defaultProvider); ok {
		defaultProvider = definition.ID
	}
	providerConfig.LLMProvider = defaultProvider
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = providerConfig.RequestTimeout
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 90 * time.Second
	}
	providerConfig.RequestTimeout = options.RequestTimeout
	if options.RealtimeMaxClients < 1 {
		options.RealtimeMaxClients = 512
	}
	if options.RealtimeStreamMaxAge < time.Minute {
		options.RealtimeStreamMaxAge = 10 * time.Minute
	}
	if options.RealtimeHeartbeat < 5*time.Second {
		options.RealtimeHeartbeat = 25 * time.Second
	}
	if options.RealtimeWriteTimeout < time.Second {
		options.RealtimeWriteTimeout = 10 * time.Second
	}
	if options.UISessionTTL < time.Minute {
		options.UISessionTTL = 30 * time.Minute
	}

	var channels channelStore
	var enqueueChannelTurn enqueueChannelTurnFunc
	var enqueueRoleplayChannelTurn enqueueRoleplayChannelTurnFunc
	var ollamaDownloads OllamaDownloadStore = options.OllamaDownloads
	if repo != nil {
		channels = repo
		enqueueChannelTurn = repo.EnqueueChannelTurnWithDataAuthority
		enqueueRoleplayChannelTurn = repo.EnqueueRoleplayChannelTurn
		if ollamaDownloads == nil {
			ollamaDownloads = repo
		}
	}
	ollamaModels := providerConfig.ProviderModels["ollama"]
	ollamaEmbeddingModel := strings.TrimSpace(ollamaModels.Embedding)
	if strings.EqualFold(strings.TrimSpace(providerConfig.EmbeddingProvider), "ollama") && strings.TrimSpace(providerConfig.EmbeddingModel) != "" {
		ollamaEmbeddingModel = strings.TrimSpace(providerConfig.EmbeddingModel)
	}

	s := &Server{
		lifecycleContext:           lifecycleContext,
		repo:                       repo,
		channelStore:               channels,
		roleplaySimulation:         options.RoleplaySimulation,
		enqueueChannelTurn:         enqueueChannelTurn,
		enqueueRoleplayChannelTurn: enqueueRoleplayChannelTurn,
		embeddingClient:            embeddingClient,
		mux:                        http.NewServeMux(),
		providerConfig:             providerConfig,
		defaultProvider:            defaultProvider,
		requestTimeout:             options.RequestTimeout,
		ollamaBaseURL:              strings.TrimSpace(providerConfig.OllamaBaseURL),
		ollamaEmbeddingModel:       ollamaEmbeddingModel,
		ollamaModelAuthority:       options.OllamaModelAuthority,
		ollamaModelLifecycle:       options.OllamaModelLifecycle,
		ollamaDownloads:            ollamaDownloads,
		ollamaCatalog:              options.OllamaCatalog,
		ollamaDownloadRunning:      make(map[string]struct{}),
		ollamaDownloadSlots:        make(chan struct{}, 1),
		webSearchProviders:         append([]string(nil), options.WebSearchProviders...),
		coreURLDefault:             strings.TrimSpace(options.CoreURL),
		listenAddr:                 strings.TrimSpace(options.ListenAddr),
		realtimeMaxClients:         options.RealtimeMaxClients,
		realtimeStreamMaxAge:       options.RealtimeStreamMaxAge,
		realtimeHeartbeat:          options.RealtimeHeartbeat,
		realtimeWriteTimeout:       options.RealtimeWriteTimeout,
		redisURL:                   strings.TrimSpace(options.RedisURL),
		uiRedisRequired:            options.UIRedisRequired,
		uiSessionTTL:               options.UISessionTTL,
		uiMemorySessions:           make(map[string]uiMemorySessionRecord),
		hostAgentURL:               strings.TrimSpace(options.HostAgentURL),
		hostAgentToken:             strings.TrimSpace(options.HostAgentToken),
		integrationAPIToken:        options.IntegrationAPIToken,
	}
	if redis, err := newUIRedisClient(s.redisURL); err == nil {
		s.uiRedis = redis
	} else if strings.TrimSpace(s.redisURL) != "" {
		s.uiRedisInitError = err.Error()
	}
	if repo != nil {
		s.secretsResolver = secrets.NewResolver(repo)
		secrets.SetGlobal(s.secretsResolver)
	}
	s.realtimeHub = NewRealtimeHub(RealtimeHubOptions{MaxClients: s.realtimeMaxClients})
	s.routes()
	if repo != nil {
		s.startRealtimeTelemetryListener(lifecycleContext)
	}
	if s.ollamaDownloads != nil {
		s.resumeOllamaModelDownloads()
	}
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReadiness)
	s.mux.HandleFunc("/v1/providers", s.handleProviderCatalog)
	s.mux.HandleFunc("/v1/status/research", s.handleResearchStatus)
	s.mux.HandleFunc("/v1/scrum", s.handleScrum)
	s.mux.HandleFunc("/v1/scrum/cards", s.handleScrumCards)
	s.mux.HandleFunc("/v1/scrum/cards/", s.handleScrumCardByID)
	s.mux.HandleFunc("/v1/scrum/files", s.handleScrumFiles)
	s.mux.HandleFunc("/v1/scrum/tags", s.handleScrumTags)
	s.mux.HandleFunc("/v1/scrum/flow-metrics", s.handleScrumFlowMetrics)
	s.mux.HandleFunc("/v1/settings/models", s.handleModelSettings)
	s.mux.HandleFunc("/v1/settings/secrets", s.handleAPISecrets)
	s.mux.HandleFunc("/v1/settings/network", s.handleNetworkSettings)
	s.mux.HandleFunc("/v1/browse", s.handleBrowse)
	s.mux.HandleFunc("/v1/browse/mkdir", s.handleBrowseMkdir)
	s.mux.HandleFunc("/v1/host/status", s.handleHostBridgeStatus)
	s.mux.HandleFunc("/v1/host/pick-directory", s.handleHostPickDirectory)
	s.mux.HandleFunc("/v1/host/terminal/preflight", s.handleHostTerminalPreflight)
	s.mux.HandleFunc("/v1/host/terminal/ws", s.handleHostTerminalWS)
	s.mux.HandleFunc("/v1/ui/runtime-config", s.handleUIRuntimeConfig)
	s.mux.HandleFunc("/v1/ui/session", s.handleUISession)
	s.mux.HandleFunc("/v1/ui/panel", s.handleUIPanel)
	s.mux.HandleFunc("/v1/ui/chat/neutral", s.handleChatNeutralTranscript)
	s.mux.HandleFunc("/v1/ui/chat/roleplay", s.handleChatRoleplaySimulation)
	s.mux.HandleFunc("/v1/ui/chat/slash-commands", s.handleChatSlashCommands)
	s.mux.HandleFunc("/v1/ui/roleplay/worlds", s.handleRoleplayWorldsComponent)
	s.mux.HandleFunc("/v1/ui/roleplay/library", s.handleRoleplayLibraryComponent)
	s.mux.HandleFunc("/v1/ui/roleplay/character", s.handleRoleplayCharacterEditor)
	s.mux.HandleFunc("/v1/ui/admin", s.handleUIAdminComponent)
	s.mux.HandleFunc("/v1/host/screen/monitors", s.handleHostScreenMonitors)
	s.mux.HandleFunc("/v1/ui/screen/monitors", s.handleUIScreenMonitors)
	s.mux.HandleFunc("/v1/host/screen/mjpeg", s.handleHostScreenMJPEG)
	s.mux.HandleFunc("/v1/projects", s.handleProjects)
	s.mux.HandleFunc("/v1/projects/", s.handleProjectByID)
	s.mux.HandleFunc("/v1/realtime/ws", s.handleRealtimeWS)
	if s.repo != nil {
		s.mux.HandleFunc("/v1/ai/control", s.handleAIControl)
		s.mux.HandleFunc("/v1/jobs", s.handleJobs)
		s.mux.HandleFunc("/v1/jobs/", s.handleJobByID)
		s.mux.HandleFunc("/v1/activity", s.handleActivity)
		s.mux.HandleFunc("/v1/memory", s.handleMemory)
		s.mux.HandleFunc("/v1/memory/batch", s.handleMemoryBatch)
		s.mux.HandleFunc("/v1/memory/", s.handleMemoryByID)
		s.mux.HandleFunc("/v1/memory/categories", s.handleMemoryCategories)
		s.mux.HandleFunc("/v1/memory/tags", s.handleMemoryTags)
		s.mux.HandleFunc("/v1/ingest/documents", s.handleIngestDocuments)
		s.mux.HandleFunc("/v1/admin/mind/stats", s.handleMindStats)
		s.mux.HandleFunc("/v1/admin/data-sources", s.handleDataSources)
		s.mux.HandleFunc("/v1/admin/data-sources/", s.handleDataSourceByID)
		s.mux.HandleFunc("/v1/ui/admin/data-sources", s.handleUIAdminDataSources)
		s.mux.HandleFunc("/v1/ui/admin/data-sources/schema", s.handleUIAdminDataSourceSchema)
		s.mux.HandleFunc("/v1/ui/admin/data-sources/query", s.handleUIAdminDataSourceQuery)
		s.mux.HandleFunc("/v1/ui/data", s.handleUIDataComponent)
		s.mux.HandleFunc("/v1/ui/projects", s.handleUIProjectsComponent)
		s.mux.HandleFunc("/v1/ui/projects/modal", s.handleUIProjectModal)
		s.mux.HandleFunc("/v1/ui/projects/", s.handleUIProjectComponent)
		s.mux.HandleFunc("/v1/ui/scrum/create-card", s.handleUIScrumCreateCard)
		s.mux.HandleFunc("/v1/data-sources", s.handlePublicDataSources)
		s.mux.HandleFunc("/v1/data-sources/", s.handlePublicDataSourceByID)
		s.mux.HandleFunc("/v1/ollama/models", s.handleOllamaModels)
		s.mux.HandleFunc("/v1/ollama/models/", s.handleOllamaModelByName)
		s.mux.HandleFunc("/v1/ollama/catalog", s.handleOllamaCatalog)
		s.mux.HandleFunc("/v1/ollama/downloads", s.handleOllamaDownloads)
		s.mux.HandleFunc("/v1/memory-candidates", s.handleMemoryCandidates)
		s.mux.HandleFunc("/v1/memory-candidates/", s.handleMemoryCandidateByID)
		s.mux.HandleFunc("/v1/metrics/live", s.handleMetricsLive)
		s.mux.HandleFunc("/v1/metrics/runs", s.handleMetricsRuns)
		s.mux.HandleFunc("/v1/metrics/runs/", s.handleMetricsRunByID)
		s.mux.HandleFunc("/v1/metrics/models", s.handleMetricsModels)
		s.mux.HandleFunc("/v1/metrics/context-shrink", s.handleMetricsContextShrink)
		s.mux.HandleFunc("/v1/metrics/context-usage", s.handleMetricsContextUsage)
		s.mux.HandleFunc("/v1/metrics/operations", s.handleMetricsOperations)
		s.mux.HandleFunc("/v1/metrics/scrum", s.handleMetricsScrum)
		s.mux.HandleFunc("/v1/metrics/glance", s.handleMetricsGlance)
		s.mux.HandleFunc("/v1/integrations/data-sources", s.requireIntegrationAuthentication(s.handleIntegrationDataSources))
		s.mux.HandleFunc("/v1/integrations/jobs/", s.requireIntegrationAuthentication(s.handleIntegrationJobByID))
		s.mux.HandleFunc("/v1/ui/chat/jobs", s.handleChatJobsComponent)
		s.mux.HandleFunc("/v1/ui/chat/jobs/", s.handleChatJobStateComponent)
		s.mux.HandleFunc("/v1/ui/chat/data-sources", s.handleChatDataSourceOptions)
		s.mux.HandleFunc("/v1/ui/chat/memory", s.handleChatMemoryComponent)
		s.mux.HandleFunc("/v1/ui/chat/timeline", s.handleChatTimelineComponent)
		s.mux.HandleFunc("/v1/ui/chat/metrics", s.handleChatMetricsComponent)
	}
	if s.channelStore != nil {
		s.mux.HandleFunc("/v1/channels", s.handleChannels)
		s.mux.HandleFunc("/v1/channels/", s.handleChannelByID)
		s.mux.HandleFunc("/v1/ui/chat/channels", s.handleChatChannelOptions)
		s.mux.HandleFunc("/v1/integrations/channels", s.requireIntegrationAuthentication(s.handleIntegrationChannels))
		s.mux.HandleFunc("/v1/integrations/channels/", s.requireIntegrationAuthentication(s.handleIntegrationChannelByID))
	}
	s.registerUIRoutes()
}
