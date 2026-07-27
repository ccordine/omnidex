package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/research"
	"github.com/gryph/omnidex/internal/secrets"
)

type Server struct {
	lifecycleContext          context.Context
	repo                      *queue.Repository
	channelStore              channelStore
	llmClient                 llm.Client
	mux                       *http.ServeMux
	instructIntegration       *instructIntegrationService
	providerConfig            config.Config
	defaultProvider           string
	requestTimeout            time.Duration
	v3Enabled                 bool
	ollamaBaseURL             string
	ollamaDefaultModel        string
	ollamaEmbeddingModel      string
	webSearchEnabled          bool
	webSearchProviders        []string
	webSearchTimeout          time.Duration
	secretsResolver           *secrets.Resolver
	coreURLDefault            string
	listenAddr                string
	realtimeMaxClients        int
	realtimeStreamMaxAge      time.Duration
	realtimeHeartbeat         time.Duration
	realtimeWriteTimeout      time.Duration
	redisURL                  string
	uiRedisRequired           bool
	uiSessionTTL              time.Duration
	uiRedis                   *uiRedisClient
	uiRedisInitError          string
	ollamaURLMu               sync.RWMutex
	hostAgentURL              string
	hostAgentToken            string
	realtimeHub               *RealtimeHub
	jobOutputOnce             sync.Once
	jobOutputCoalescer        *jobOutputCoalescer
	telemetryRealtimeOnce     sync.Once
	telemetryRealtime         *telemetryRealtimeCoalescer
	scrumAutoWorkMu           sync.Mutex
	scrumAutoWorkAsyncMu      sync.Mutex
	scrumAutoWorkAsyncRunning bool
	scrumAutoWorkAsyncPending bool
}

type ServerOptions struct {
	LifecycleContext     context.Context
	ProviderConfig       config.Config
	RequestTimeout       time.Duration
	V3Enabled            bool
	WebSearchEnabled     bool
	WebSearchProviders   []string
	WebSearchTimeout     time.Duration
	CoreURL              string
	ListenAddr           string
	HostAgentURL         string
	HostAgentToken       string
	RealtimeMaxClients   int
	RealtimeStreamMaxAge time.Duration
	RealtimeHeartbeat    time.Duration
	RealtimeWriteTimeout time.Duration
	RedisURL             string
	UIRedisRequired      bool
	UISessionTTL         time.Duration
}

type enqueueRequest struct {
	Instruction string          `json:"instruction"`
	Pipeline    string          `json:"pipeline"`
	Metadata    json.RawMessage `json:"metadata"`
}

type memoryRequest struct {
	Source  string   `json:"source"`
	Kind    string   `json:"kind"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type researchIngestRequest struct {
	Topic                  string   `json:"topic"`
	Source                 string   `json:"source"`
	Kind                   string   `json:"kind"`
	Tags                   []string `json:"tags"`
	ChunkSize              int      `json:"chunk_size"`
	Overlap                int      `json:"overlap"`
	MaxChunks              int      `json:"max_chunks"`
	IncludeOfficialSources *bool    `json:"include_official_sources,omitempty"`
}

type researchIngestResponse struct {
	Topic             string              `json:"topic"`
	Slug              string              `json:"slug"`
	SourcePrefix      string              `json:"source_prefix"`
	StoredChunks      int                 `json:"stored_chunks"`
	Tags              []string            `json:"tags"`
	Warnings          []string            `json:"warnings,omitempty"`
	Dossier           string              `json:"dossier,omitempty"`
	Sources           []string            `json:"sources,omitempty"`
	StoredChunkSource []string            `json:"stored_chunk_sources,omitempty"`
	Documents         []research.Document `json:"documents,omitempty"`
}

type memoryCandidatePromotionRequest struct {
	Tier string `json:"tier"`
}

type feedbackRequest struct {
	Feedback string `json:"feedback"`
}

type cancelRequest struct {
	Reason string `json:"reason"`
}

type personaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type personaRequest struct {
	Model       string                      `json:"model"`
	System      string                      `json:"system"`
	Prompt      string                      `json:"prompt"`
	Context     json.RawMessage             `json:"context"`
	History     []personaMessage            `json:"history"`
	LLM         *personaLLMRequest          `json:"llm,omitempty"`
	Integration *instructIntegrationRequest `json:"integration,omitempty"`
}

type personaLLMRequest struct {
	Provider   string                   `json:"provider,omitempty"`
	Model      string                   `json:"model,omitempty"`
	Compatible *personaCompatibleConfig `json:"compatible,omitempty"`
}

type personaCompatibleConfig struct {
	APIKey       string `json:"api_key,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	Organization string `json:"organization,omitempty"`
	Project      string `json:"project,omitempty"`
}

type resolvedPersonaLLM struct {
	Client   llm.Client
	Provider string
	Model    string
}

type personaRequestError struct {
	StatusCode int
	Message    string
}

func (e personaRequestError) Error() string {
	return strings.TrimSpace(e.Message)
}

type personaStage struct {
	Name   string `json:"name"`
	Output string `json:"output"`
}

type personaResponse struct {
	Persona     string                     `json:"persona"`
	Model       string                     `json:"model"`
	Output      string                     `json:"output"`
	LatencyMS   int64                      `json:"latency_ms"`
	Stages      []personaStage             `json:"stages,omitempty"`
	Integration *instructIntegrationResult `json:"integration,omitempty"`
}

func NewServer(repo *queue.Repository, llmClient llm.Client) *Server {
	return NewServerWithOptions(repo, llmClient, ServerOptions{})
}

func NewServerWithOptions(repo *queue.Repository, llmClient llm.Client, options ServerOptions) *Server {
	lifecycleContext := options.LifecycleContext
	if lifecycleContext == nil {
		lifecycleContext = context.Background()
	}
	providerConfig := options.ProviderConfig
	providerConfig.CompatibleProviders = config.CloneCompatibleProviders(providerConfig.CompatibleProviders)
	providerConfig.ProviderModels = config.CloneProviderModels(providerConfig.ProviderModels)
	defaultProvider := strings.TrimSpace(providerConfig.LLMProvider)
	if defaultProvider == "" {
		defaultProvider = "ollama"
	} else if definition, ok := catalog.Lookup(defaultProvider); ok {
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
	if repo != nil {
		channels = repo
	} else {
		channels = newInMemoryChannelStore()
	}
	ollamaModels := providerConfig.ProviderModels["ollama"]
	ollamaDefaultModel := strings.TrimSpace(ollamaModels.Default)
	if defaultProvider == "ollama" && strings.TrimSpace(providerConfig.DefaultModel) != "" {
		ollamaDefaultModel = strings.TrimSpace(providerConfig.DefaultModel)
	}
	ollamaEmbeddingModel := strings.TrimSpace(ollamaModels.Embedding)
	if strings.EqualFold(strings.TrimSpace(providerConfig.EmbeddingProvider), "ollama") && strings.TrimSpace(providerConfig.EmbeddingModel) != "" {
		ollamaEmbeddingModel = strings.TrimSpace(providerConfig.EmbeddingModel)
	}

	s := &Server{
		lifecycleContext:     lifecycleContext,
		repo:                 repo,
		channelStore:         channels,
		llmClient:            llmClient,
		mux:                  http.NewServeMux(),
		instructIntegration:  newInstructIntegrationService(repo),
		providerConfig:       providerConfig,
		defaultProvider:      defaultProvider,
		requestTimeout:       options.RequestTimeout,
		v3Enabled:            options.V3Enabled,
		ollamaBaseURL:        strings.TrimSpace(providerConfig.OllamaBaseURL),
		ollamaDefaultModel:   ollamaDefaultModel,
		ollamaEmbeddingModel: ollamaEmbeddingModel,
		webSearchEnabled:     options.WebSearchEnabled,
		webSearchProviders:   append([]string(nil), options.WebSearchProviders...),
		webSearchTimeout:     options.WebSearchTimeout,
		coreURLDefault:       strings.TrimSpace(options.CoreURL),
		listenAddr:           strings.TrimSpace(options.ListenAddr),
		realtimeMaxClients:   options.RealtimeMaxClients,
		realtimeStreamMaxAge: options.RealtimeStreamMaxAge,
		realtimeHeartbeat:    options.RealtimeHeartbeat,
		realtimeWriteTimeout: options.RealtimeWriteTimeout,
		redisURL:             strings.TrimSpace(options.RedisURL),
		uiRedisRequired:      options.UIRedisRequired,
		uiSessionTTL:         options.UISessionTTL,
		hostAgentURL:         strings.TrimSpace(options.HostAgentURL),
		hostAgentToken:       strings.TrimSpace(options.HostAgentToken),
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
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/v1/providers", s.handleProviderCatalog)
	s.mux.HandleFunc("/v1/status/research", s.handleResearchStatus)
	s.mux.HandleFunc("/v1/instruct", s.handleInstruct)
	s.mux.HandleFunc("/v1/roleplay", s.handleRoleplay)
	s.mux.HandleFunc("/v1/narrate", s.handleNarrate)
	s.mux.HandleFunc("/v1/reasoning", s.handleReasoning)
	s.mux.HandleFunc("/v1/scrum", s.handleScrum)
	s.mux.HandleFunc("/v1/scrum/cards", s.handleScrumCards)
	s.mux.HandleFunc("/v1/scrum/cards/sync", s.handleScrumCardSync)
	s.mux.HandleFunc("/v1/scrum/cards/", s.handleScrumCardByID)
	s.mux.HandleFunc("/v1/scrum/files", s.handleScrumFiles)
	s.mux.HandleFunc("/v1/scrum/tags", s.handleScrumTags)
	s.mux.HandleFunc("/v1/scrum/flow-metrics", s.handleScrumFlowMetrics)
	s.mux.HandleFunc("/v1/settings/models", s.handleModelSettings)
	s.mux.HandleFunc("/v1/models/resolved", s.handleResolvedModels)
	s.mux.HandleFunc("/v1/agents/resolved", s.handleResolvedAgents)
	s.mux.HandleFunc("/v1/settings/agents", s.handleAgentSettings)
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
	s.mux.HandleFunc("/v1/host/screen/monitors", s.handleHostScreenMonitors)
	s.mux.HandleFunc("/v1/host/screen/mjpeg", s.handleHostScreenMJPEG)
	s.mux.HandleFunc("/v1/recipes", s.handleRecipes)
	s.mux.HandleFunc("/v1/recipes/", s.handleRecipeByID)
	s.mux.HandleFunc("/v1/projects", s.handleProjects)
	s.mux.HandleFunc("/v1/projects/", s.handleProjectByID)
	s.mux.HandleFunc("/v1/realtime/ws", s.handleRealtimeWS)
	if s.repo != nil {
		s.mux.HandleFunc("/v1/ai/control", s.handleAIControl)
		s.mux.HandleFunc("/v1/jobs", s.handleJobs)
		s.mux.HandleFunc("/v1/jobs/", s.handleJobByID)
		s.mux.HandleFunc("/v1/activity", s.handleActivity)
		s.mux.HandleFunc("/v1/memory", s.handleMemory)
		s.mux.HandleFunc("/v1/memory/", s.handleMemoryByID)
		s.mux.HandleFunc("/v1/memory/categories", s.handleMemoryCategories)
		s.mux.HandleFunc("/v1/memory/tags", s.handleMemoryTags)
		s.mux.HandleFunc("/v1/ingest/documents", s.handleIngestDocuments)
		s.mux.HandleFunc("/v1/admin/mind/stats", s.handleMindStats)
		s.mux.HandleFunc("/v1/admin/data-sources", s.handleDataSources)
		s.mux.HandleFunc("/v1/admin/data-sources/", s.handleDataSourceByID)
		s.mux.HandleFunc("/v1/data-sources", s.handlePublicDataSources)
		s.mux.HandleFunc("/v1/data-sources/", s.handlePublicDataSourceByID)
		s.mux.HandleFunc("/v1/ollama/models", s.handleOllamaModels)
		s.mux.HandleFunc("/v1/ollama/models/", s.handleOllamaModelByName)
		s.mux.HandleFunc("/v1/research/ingest", s.handleResearchIngest)
		s.mux.HandleFunc("/v1/memory-candidates", s.handleMemoryCandidates)
		s.mux.HandleFunc("/v1/memory-candidates/", s.handleMemoryCandidateByID)
		s.mux.HandleFunc("/v1/admin/migrate-fresh", s.handleAdminMigrateFresh)
		s.mux.HandleFunc("/v1/metrics/live", s.handleMetricsLive)
		s.mux.HandleFunc("/v1/metrics/runs", s.handleMetricsRuns)
		s.mux.HandleFunc("/v1/metrics/runs/", s.handleMetricsRunByID)
		s.mux.HandleFunc("/v1/metrics/models", s.handleMetricsModels)
		s.mux.HandleFunc("/v1/metrics/playbooks", s.handleMetricsPlaybooks)
		s.mux.HandleFunc("/v1/metrics/benchmarks", s.handleMetricsBenchmarks)
		s.mux.HandleFunc("/v1/metrics/context-shrink", s.handleMetricsContextShrink)
		s.mux.HandleFunc("/v1/metrics/context-usage", s.handleMetricsContextUsage)
		s.mux.HandleFunc("/v1/metrics/operations", s.handleMetricsOperations)
		s.mux.HandleFunc("/v1/metrics/scrum", s.handleMetricsScrum)
		s.mux.HandleFunc("/v1/metrics/glance", s.handleMetricsGlance)
	}
	if s.channelStore != nil {
		s.mux.HandleFunc("/v1/channels", s.handleChannels)
		s.mux.HandleFunc("/v1/channels/", s.handleChannelByID)
	}
	s.registerUIRoutes()
}
