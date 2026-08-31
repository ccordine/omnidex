package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type Server struct {
	lifecycleContext           context.Context
	repo                       *queue.Repository
	roleplaySimulation         RoleplaySimulationStore
	embeddingClient            llm.EmbeddingClient
	mux                        *http.ServeMux
	providerConfig             config.Config
	coreURLDefault             string
	listenAddr                 string
	realtimeStreamMaxAge       string
	realtimeHeartbeat          string
	realtimeWriteTimeout       string
	redisURL                   string
	uiSessionTTL               string
	uiRedisMu                  sync.Mutex
	uiRedis                    *uiRedisClient
	roleplaySceneDraftMu       sync.Mutex
	hostAgentURL               string
	hostAgentToken             string
	integrationAPIToken        string
	realtimeHub                *RealtimeHub
	scrumAutoWorkMu            sync.Mutex
	scrumAutoWorkAsyncMu       sync.Mutex
	scrumAutoWorkAsyncRunning  bool
	scrumAutoWorkAsyncPending  bool
}

type ServerOptions struct {
	LifecycleContext     context.Context
	ProviderConfig       config.Config
	CoreURL              string
	ListenAddr           string
	HostAgentURL         string
	HostAgentToken       string
	IntegrationAPIToken  string
	RealtimeStreamMaxAge string
	RealtimeHeartbeat    string
	RealtimeWriteTimeout string
	RedisURL             string
	UISessionTTL         string
	RoleplaySimulation   RoleplaySimulationStore
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

func NewServer(
	repo *queue.Repository,
	embeddingClient llm.EmbeddingClient,
	options ServerOptions,
) (*Server, error) {
	if repo == nil {
		return nil, fmt.Errorf("server requires PostgreSQL repository authority")
	}
	if options.LifecycleContext == nil {
		return nil, fmt.Errorf("server requires lifecycle context")
	}
	providerConfig := options.ProviderConfig
	providerConfig.CompatibleProviders = config.CloneCompatibleProviders(providerConfig.CompatibleProviders)
	providerConfig.ProviderModels = config.CloneProviderModels(providerConfig.ProviderModels)
	s := &Server{
		lifecycleContext:           options.LifecycleContext,
		repo:                       repo,
		roleplaySimulation:         options.RoleplaySimulation,
		embeddingClient:            embeddingClient,
		mux:                        http.NewServeMux(),
		providerConfig:             providerConfig,
		coreURLDefault:             strings.TrimSpace(options.CoreURL),
		listenAddr:                 strings.TrimSpace(options.ListenAddr),
		realtimeStreamMaxAge:       options.RealtimeStreamMaxAge,
		realtimeHeartbeat:          options.RealtimeHeartbeat,
		realtimeWriteTimeout:       options.RealtimeWriteTimeout,
		redisURL:                   strings.TrimSpace(options.RedisURL),
		uiSessionTTL:               options.UISessionTTL,
		hostAgentURL:               strings.TrimSpace(options.HostAgentURL),
		hostAgentToken:             strings.TrimSpace(options.HostAgentToken),
		integrationAPIToken:        options.IntegrationAPIToken,
	}
	s.realtimeHub = NewRealtimeHub()
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReadiness)
	s.mux.HandleFunc("/v1/providers", s.handleProviderCatalog)
	s.mux.HandleFunc("/v1/scrum", s.handleScrum)
	s.mux.HandleFunc("/v1/scrum/cards", s.handleScrumCards)
	s.mux.HandleFunc("/v1/scrum/cards/", s.handleScrumCardByID)
	s.mux.HandleFunc("/v1/scrum/tags", s.handleScrumTags)
	s.mux.HandleFunc("/v1/scrum/flow-metrics", s.handleScrumFlowMetrics)
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
		s.mux.HandleFunc("/v1/ui/data", s.handleUIDataComponent)
		s.mux.HandleFunc("/v1/ui/projects", s.handleUIProjectsComponent)
		s.mux.HandleFunc("/v1/ui/projects/modal", s.handleUIProjectModal)
		s.mux.HandleFunc("/v1/ui/projects/", s.handleUIProjectComponent)
		s.mux.HandleFunc("/v1/ui/scrum/create-card", s.handleUIScrumCreateCard)
		s.mux.HandleFunc("/v1/data-sources", s.handlePublicDataSources)
		s.mux.HandleFunc("/v1/data-sources/", s.handlePublicDataSourceByID)
		s.mux.HandleFunc("/v1/memory-candidates", s.handleMemoryCandidates)
		s.mux.HandleFunc("/v1/memory-candidates/", s.handleMemoryCandidateByID)
		s.mux.HandleFunc("/v1/integrations/data-sources", s.requireIntegrationAuthentication(s.handleIntegrationDataSources))
		s.mux.HandleFunc("/v1/integrations/jobs/", s.requireIntegrationAuthentication(s.handleIntegrationJobByID))
		s.mux.HandleFunc("/v1/ui/chat/jobs", s.handleChatJobsComponent)
		s.mux.HandleFunc("/v1/ui/chat/jobs/", s.handleChatJobStateComponent)
		s.mux.HandleFunc("/v1/ui/chat/data-sources", s.handleChatDataSourceOptions)
		s.mux.HandleFunc("/v1/ui/chat/memory", s.handleChatMemoryComponent)
		s.mux.HandleFunc("/v1/ui/chat/timeline", s.handleChatTimelineComponent)
	s.mux.HandleFunc("/v1/channels", s.handleChannels)
		s.mux.HandleFunc("/v1/channels/", s.handleChannelByID)
		s.mux.HandleFunc("/v1/ui/chat/channels", s.handleChatChannelOptions)
		s.mux.HandleFunc("/v1/integrations/channels", s.requireIntegrationAuthentication(s.handleIntegrationChannels))
		s.mux.HandleFunc("/v1/integrations/channels/", s.requireIntegrationAuthentication(s.handleIntegrationChannelByID))
	s.registerUIRoutes()
}
