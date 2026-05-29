package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/research"
	"github.com/gryph/omnidex/internal/secrets"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	repo                      *queue.Repository
	channelStore              channelStore
	scrumStore                *ScrumStore
	llmClient                 llm.Client
	mux                       *http.ServeMux
	instructIntegration       *instructIntegrationService
	defaultProvider           string
	requestTimeout            time.Duration
	v3Enabled                 bool
	ollamaBaseURL             string
	ollamaDefaultModel        string
	ollamaEmbeddingModel      string
	openAIBaseURL             string
	openAIAPIKey              string
	openAIOrganization        string
	openAIProject             string
	openAIDefaultModel        string
	openAIEmbeddingModel      string
	azureAIBaseURL            string
	azureAIAPIKey             string
	azureAIAPIVersion         string
	azureAIAPIStyle           string
	azureAIDefaultModel       string
	azureAIEmbeddingModel     string
	xAIBaseURL                string
	xAIAPIKey                 string
	xAIDefaultModel           string
	googleBaseURL             string
	googleAPIKey              string
	googleDefaultModel        string
	googleEmbeddingModel      string
	anthropicBaseURL          string
	anthropicAPIKey           string
	anthropicVersion          string
	anthropicMaxTokens        int
	anthropicDefaultModel     string
	huggingFaceBaseURL        string
	huggingFaceAPIKey         string
	huggingFaceDefaultModel   string
	huggingFaceEmbeddingModel string
	webSearchEnabled          bool
	webSearchProviders        []string
	webSearchTimeout          time.Duration
	secretsResolver           *secrets.Resolver
	coreURLDefault            string
	listenAddr                string
	ollamaURLMu               sync.RWMutex
	hostAgentURL              string
	hostAgentToken            string
}

type ServerOptions struct {
	DefaultProvider           string
	RequestTimeout            time.Duration
	V3Enabled                 bool
	OllamaBaseURL             string
	OllamaDefaultModel        string
	OllamaEmbeddingModel      string
	OpenAIBaseURL             string
	OpenAIAPIKey              string
	OpenAIOrganization        string
	OpenAIProject             string
	OpenAIDefaultModel        string
	OpenAIEmbeddingModel      string
	AzureAIBaseURL            string
	AzureAIAPIKey             string
	AzureAIAPIVersion         string
	AzureAIAPIStyle           string
	AzureAIDefaultModel       string
	AzureAIEmbeddingModel     string
	XAIBaseURL                string
	XAIAPIKey                 string
	XAIDefaultModel           string
	GoogleBaseURL             string
	GoogleAPIKey              string
	GoogleDefaultModel        string
	GoogleEmbeddingModel      string
	AnthropicBaseURL          string
	AnthropicAPIKey           string
	AnthropicVersion          string
	AnthropicMaxTokens        int
	AnthropicDefaultModel     string
	HuggingFaceBaseURL        string
	HuggingFaceAPIKey         string
	HuggingFaceDefaultModel   string
	HuggingFaceEmbeddingModel string
	WebSearchEnabled          bool
	WebSearchProviders        []string
	WebSearchTimeout          time.Duration
	CoreURL                   string
	ListenAddr                string
	HostAgentURL              string
	HostAgentToken            string
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
	Provider string               `json:"provider,omitempty"`
	Model    string               `json:"model,omitempty"`
	OpenAI   *personaOpenAIConfig `json:"openai,omitempty"`
}

type personaOpenAIConfig struct {
	APIKey            string `json:"api_key,omitempty"`
	BaseURL           string `json:"base_url,omitempty"`
	Organization      string `json:"organization,omitempty"`
	Project           string `json:"project,omitempty"`
	UseServerFallback bool   `json:"use_server_fallback"`
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
	defaultProvider := normalizePersonaProvider(options.DefaultProvider)
	if defaultProvider == "" {
		defaultProvider = "ollama"
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 90 * time.Second
	}

	var channels channelStore
	if repo != nil {
		channels = repo
	} else {
		channels = newInMemoryChannelStore()
	}
	s := &Server{
		repo:                      repo,
		channelStore:              channels,
		llmClient:                 llmClient,
		mux:                       http.NewServeMux(),
		instructIntegration:       newInstructIntegrationService(repo),
		defaultProvider:           defaultProvider,
		requestTimeout:            options.RequestTimeout,
		v3Enabled:                 options.V3Enabled,
		ollamaBaseURL:             strings.TrimSpace(options.OllamaBaseURL),
		ollamaDefaultModel:        strings.TrimSpace(options.OllamaDefaultModel),
		ollamaEmbeddingModel:      strings.TrimSpace(options.OllamaEmbeddingModel),
		openAIBaseURL:             strings.TrimSpace(options.OpenAIBaseURL),
		openAIAPIKey:              strings.TrimSpace(options.OpenAIAPIKey),
		openAIOrganization:        strings.TrimSpace(options.OpenAIOrganization),
		openAIProject:             strings.TrimSpace(options.OpenAIProject),
		openAIDefaultModel:        strings.TrimSpace(options.OpenAIDefaultModel),
		openAIEmbeddingModel:      strings.TrimSpace(options.OpenAIEmbeddingModel),
		azureAIBaseURL:            strings.TrimSpace(options.AzureAIBaseURL),
		azureAIAPIKey:             strings.TrimSpace(options.AzureAIAPIKey),
		azureAIAPIVersion:         strings.TrimSpace(options.AzureAIAPIVersion),
		azureAIAPIStyle:           strings.TrimSpace(options.AzureAIAPIStyle),
		azureAIDefaultModel:       strings.TrimSpace(options.AzureAIDefaultModel),
		azureAIEmbeddingModel:     strings.TrimSpace(options.AzureAIEmbeddingModel),
		xAIBaseURL:                strings.TrimSpace(options.XAIBaseURL),
		xAIAPIKey:                 strings.TrimSpace(options.XAIAPIKey),
		xAIDefaultModel:           strings.TrimSpace(options.XAIDefaultModel),
		googleBaseURL:             strings.TrimSpace(options.GoogleBaseURL),
		googleAPIKey:              strings.TrimSpace(options.GoogleAPIKey),
		googleDefaultModel:        strings.TrimSpace(options.GoogleDefaultModel),
		googleEmbeddingModel:      strings.TrimSpace(options.GoogleEmbeddingModel),
		anthropicBaseURL:          strings.TrimSpace(options.AnthropicBaseURL),
		anthropicAPIKey:           strings.TrimSpace(options.AnthropicAPIKey),
		anthropicVersion:          strings.TrimSpace(options.AnthropicVersion),
		anthropicMaxTokens:        options.AnthropicMaxTokens,
		anthropicDefaultModel:     strings.TrimSpace(options.AnthropicDefaultModel),
		huggingFaceBaseURL:        strings.TrimSpace(options.HuggingFaceBaseURL),
		huggingFaceAPIKey:         strings.TrimSpace(options.HuggingFaceAPIKey),
		huggingFaceDefaultModel:   strings.TrimSpace(options.HuggingFaceDefaultModel),
		huggingFaceEmbeddingModel: strings.TrimSpace(options.HuggingFaceEmbeddingModel),
		webSearchEnabled:          options.WebSearchEnabled,
		webSearchProviders:        append([]string(nil), options.WebSearchProviders...),
		webSearchTimeout:          options.WebSearchTimeout,
		coreURLDefault:            strings.TrimSpace(options.CoreURL),
		listenAddr:                strings.TrimSpace(options.ListenAddr),
		hostAgentURL:              strings.TrimSpace(options.HostAgentURL),
		hostAgentToken:            strings.TrimSpace(options.HostAgentToken),
	}
	if repo != nil {
		s.secretsResolver = secrets.NewResolver(repo)
		secrets.SetGlobal(s.secretsResolver)
		s.applyStoredSecrets(context.Background())
	}
	if store, err := NewScrumStore(); err == nil {
		s.scrumStore = store
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
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
	s.mux.HandleFunc("/v1/host/terminal/ws", s.handleHostTerminalWS)
	s.mux.HandleFunc("/v1/recipes", s.handleRecipes)
	s.mux.HandleFunc("/v1/recipes/", s.handleRecipeByID)
	s.mux.HandleFunc("/v1/projects", s.handleProjects)
	s.mux.HandleFunc("/v1/projects/", s.handleProjectByID)
	s.mux.HandleFunc("/v1/workspace", s.handleWorkspace)
	if s.repo != nil {
		s.mux.HandleFunc("/v1/jobs", s.handleJobs)
		s.mux.HandleFunc("/v1/jobs/", s.handleJobByID)
		s.mux.HandleFunc("/v1/activity", s.handleActivity)
		s.mux.HandleFunc("/v1/memory", s.handleMemory)
		s.mux.HandleFunc("/v1/memory/", s.handleMemoryByID)
		s.mux.HandleFunc("/v1/memory/categories", s.handleMemoryCategories)
		s.mux.HandleFunc("/v1/memory/tags", s.handleMemoryTags)
		s.mux.HandleFunc("/v1/ingest/documents", s.handleIngestDocuments)
		s.mux.HandleFunc("/v1/admin/mind/stats", s.handleMindStats)
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
	}
	if s.channelStore != nil {
		s.mux.HandleFunc("/v1/channels", s.handleChannels)
		s.mux.HandleFunc("/v1/channels/", s.handleChannelByID)
	}
	s.registerUIRoutes()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	coreURL, source := s.resolveCoreURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"time":          time.Now().UTC(),
		"queue_enabled": s.repo != nil,
		"core_url":      coreURL,
		"core_url_source": source,
		"listen_addr":   strings.TrimSpace(s.listenAddr),
	})
}

func (s *Server) handleInstruct(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "instruct")
}

func (s *Server) handleRoleplay(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "roleplay")
}

func (s *Server) handleNarrate(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "narrate")
}

func (s *Server) handleReasoning(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, ok := decodePersonaRequest(w, r)
	if !ok {
		return
	}

	resolvedLLM, err := s.resolvePersonaLLM(req)
	if err != nil {
		var requestErr personaRequestError
		if errors.As(err, &requestErr) {
			writeError(w, requestErr.StatusCode, requestErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if strings.TrimSpace(resolvedLLM.Model) != "" {
		req.Model = strings.TrimSpace(resolvedLLM.Model)
	}

	started := time.Now()
	parseOutput, parseModel, err := s.runPersona(r.Context(), "reasoning_parse", req, nil, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	deliberateOutput, deliberateModel, err := s.runPersona(r.Context(), "reasoning_deliberate", req, map[string]string{
		"Reasoning Parse Output": parseOutput,
	}, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	finalOutput, finalModel, err := s.runPersona(r.Context(), "reasoning_final", req, map[string]string{
		"Reasoning Parse Output":      parseOutput,
		"Reasoning Deliberate Output": deliberateOutput,
	}, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, personaResponse{
		Persona:   "reasoning",
		Model:     firstNonEmpty(finalModel, deliberateModel, parseModel, strings.TrimSpace(req.Model)),
		Output:    strings.TrimSpace(finalOutput),
		LatencyMS: time.Since(started).Milliseconds(),
		Stages: []personaStage{
			{Name: "parse", Output: parseOutput},
			{Name: "deliberate", Output: deliberateOutput},
			{Name: "final", Output: finalOutput},
		},
	})
}

func (s *Server) handlePersona(w http.ResponseWriter, r *http.Request, persona string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, ok := decodePersonaRequest(w, r)
	if !ok {
		return
	}

	resolvedLLM, err := s.resolvePersonaLLM(req)
	if err != nil {
		var requestErr personaRequestError
		if errors.As(err, &requestErr) {
			writeError(w, requestErr.StatusCode, requestErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if strings.TrimSpace(resolvedLLM.Model) != "" {
		req.Model = strings.TrimSpace(resolvedLLM.Model)
	}

	started := time.Now()
	if strings.EqualFold(persona, "instruct") && s.instructIntegration != nil {
		integrationResult, handled, statusCode, err := s.instructIntegration.Handle(r.Context(), req)
		if err != nil {
			writeError(w, statusCode, err.Error())
			return
		}
		if handled {
			writeJSON(w, statusCode, personaResponse{
				Persona:     "instruct",
				Model:       "integration:" + integrationResult.Action,
				Output:      integrationResult.Message,
				LatencyMS:   time.Since(started).Milliseconds(),
				Integration: &integrationResult,
			})
			return
		}
	}

	output, modelName, err := s.runPersona(r.Context(), persona, req, nil, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, personaResponse{
		Persona:   persona,
		Model:     firstNonEmpty(modelName, strings.TrimSpace(req.Model)),
		Output:    strings.TrimSpace(output),
		LatencyMS: time.Since(started).Milliseconds(),
	})
}

func decodePersonaRequest(w http.ResponseWriter, r *http.Request) (personaRequest, bool) {
	var req personaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return personaRequest{}, false
	}

	req.Model = strings.TrimSpace(req.Model)
	req.System = strings.TrimSpace(req.System)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.LLM != nil {
		req.LLM.Provider = strings.TrimSpace(req.LLM.Provider)
		req.LLM.Model = strings.TrimSpace(req.LLM.Model)
		if req.LLM.OpenAI != nil {
			req.LLM.OpenAI.APIKey = strings.TrimSpace(req.LLM.OpenAI.APIKey)
			req.LLM.OpenAI.BaseURL = strings.TrimSpace(req.LLM.OpenAI.BaseURL)
			req.LLM.OpenAI.Organization = strings.TrimSpace(req.LLM.OpenAI.Organization)
			req.LLM.OpenAI.Project = strings.TrimSpace(req.LLM.OpenAI.Project)
		}
	}
	if req.Prompt == "" && (req.Integration == nil || strings.TrimSpace(req.Integration.Instruction) == "") {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return personaRequest{}, false
	}
	return req, true
}

func (s *Server) resolvePersonaLLM(req personaRequest) (resolvedPersonaLLM, error) {
	if s.llmClient == nil {
		return resolvedPersonaLLM{}, fmt.Errorf("llm client is not configured")
	}

	baseModel := strings.TrimSpace(req.Model)
	if req.LLM == nil {
		return resolvedPersonaLLM{
			Client:   s.llmClient,
			Provider: s.defaultProvider,
			Model:    baseModel,
		}, nil
	}

	requestedProvider := normalizePersonaProvider(req.LLM.Provider)
	if requestedProvider == "" {
		requestedProvider = s.defaultProvider
	}
	if !isSupportedPersonaProvider(requestedProvider) {
		return resolvedPersonaLLM{}, personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("llm.provider %q is unsupported (allowed: ollama, openai, azure, xai, google, anthropic, huggingface)", strings.TrimSpace(req.LLM.Provider)),
		}
	}
	if req.LLM.OpenAI != nil && requestedProvider != "openai" {
		return resolvedPersonaLLM{}, personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    "llm.openai fields are only valid when llm.provider is openai",
		}
	}

	requestedModel := firstNonEmpty(strings.TrimSpace(req.LLM.Model), baseModel)
	if requestedProvider == s.defaultProvider && !personaLLMRequiresDedicatedClient(req.LLM) {
		return resolvedPersonaLLM{
			Client:   s.llmClient,
			Provider: requestedProvider,
			Model:    requestedModel,
		}, nil
	}

	cfg, resolvedModel, err := s.personaProviderConfig(requestedProvider, requestedModel, req.LLM.OpenAI)
	if err != nil {
		return resolvedPersonaLLM{}, err
	}
	if strings.TrimSpace(resolvedModel) == "" {
		return resolvedPersonaLLM{}, personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("%s provider requested but no model is available", requestedProvider),
		}
	}
	client, buildErr := llmprovider.NewProvider(cfg, llmprovider.Options{
		Provider: requestedProvider,
		Model:    resolvedModel,
		Timeout:  s.requestTimeout,
	})
	if buildErr != nil {
		return resolvedPersonaLLM{}, buildErr
	}

	return resolvedPersonaLLM{
		Client:   client,
		Provider: requestedProvider,
		Model:    resolvedModel,
	}, nil
}

func (s *Server) personaProviderConfig(provider, requestedModel string, openAIConfig *personaOpenAIConfig) (config.Config, string, error) {
	cfg := config.Config{
		LLMProvider:        provider,
		EmbeddingProvider:  provider,
		RequestTimeout:     s.requestTimeout,
		OllamaBaseURL:      s.ollamaBaseURL,
		OpenAIBaseURL:      s.openAIBaseURL,
		OpenAIAPIKey:       s.openAIAPIKey,
		OpenAIOrganization: s.openAIOrganization,
		OpenAIProject:      s.openAIProject,
		AzureAIBaseURL:     s.azureAIBaseURL,
		AzureAIAPIKey:      s.azureAIAPIKey,
		AzureAIAPIVersion:  s.azureAIAPIVersion,
		AzureAIAPIStyle:    s.azureAIAPIStyle,
		XAIBaseURL:         s.xAIBaseURL,
		XAIAPIKey:          s.xAIAPIKey,
		GoogleBaseURL:      s.googleBaseURL,
		GoogleAPIKey:       s.googleAPIKey,
		AnthropicBaseURL:   s.anthropicBaseURL,
		AnthropicAPIKey:    s.anthropicAPIKey,
		AnthropicVersion:   s.anthropicVersion,
		AnthropicMaxTokens: s.anthropicMaxTokens,
		HuggingFaceBaseURL: s.huggingFaceBaseURL,
		HuggingFaceAPIKey:  s.huggingFaceAPIKey,
	}
	switch provider {
	case "ollama":
		model := firstNonEmpty(requestedModel, s.ollamaDefaultModel)
		cfg.DefaultModel = model
		cfg.EmbeddingModel = s.ollamaEmbeddingModel
		if strings.TrimSpace(s.ollamaBaseURL) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "ollama provider requested but OLLAMA_BASE_URL is unavailable"}
		}
		return cfg, model, nil
	case "openai":
		if openAIConfig != nil {
			cfg.OpenAIAPIKey = firstNonEmpty(openAIConfig.APIKey, cfg.OpenAIAPIKey)
			cfg.OpenAIBaseURL = firstNonEmpty(openAIConfig.BaseURL, cfg.OpenAIBaseURL)
			cfg.OpenAIOrganization = firstNonEmpty(openAIConfig.Organization, cfg.OpenAIOrganization)
			cfg.OpenAIProject = firstNonEmpty(openAIConfig.Project, cfg.OpenAIProject)
			if strings.TrimSpace(openAIConfig.APIKey) == "" && !openAIConfig.UseServerFallback {
				cfg.OpenAIAPIKey = ""
			}
		}
		model := firstNonEmpty(requestedModel, s.openAIDefaultModel)
		cfg.DefaultModel = model
		cfg.EmbeddingModel = s.openAIEmbeddingModel
		if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "openai provider requested but no API key is available (provide llm.openai.api_key or enable/use server fallback key)"}
		}
		return cfg, model, nil
	case "azure":
		model := firstNonEmpty(requestedModel, s.azureAIDefaultModel)
		cfg.DefaultModel = model
		cfg.EmbeddingModel = s.azureAIEmbeddingModel
		if strings.TrimSpace(cfg.AzureAIAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "azure provider requested but AZURE_AI_API_KEY or AZURE_OPENAI_API_KEY is unavailable"}
		}
		if strings.TrimSpace(cfg.AzureAIBaseURL) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "azure provider requested but AZURE_AI_BASE_URL or AZURE_OPENAI_ENDPOINT is unavailable"}
		}
		return cfg, model, nil
	case "xai":
		model := firstNonEmpty(requestedModel, s.xAIDefaultModel)
		cfg.DefaultModel = model
		if strings.TrimSpace(cfg.XAIAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "xai provider requested but XAI_API_KEY or GROK_API_KEY is unavailable"}
		}
		return cfg, model, nil
	case "google":
		model := firstNonEmpty(requestedModel, s.googleDefaultModel)
		cfg.DefaultModel = model
		cfg.EmbeddingModel = s.googleEmbeddingModel
		if strings.TrimSpace(cfg.GoogleAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "google provider requested but GOOGLE_API_KEY or GEMINI_API_KEY is unavailable"}
		}
		return cfg, model, nil
	case "anthropic":
		model := firstNonEmpty(requestedModel, s.anthropicDefaultModel)
		cfg.DefaultModel = model
		if strings.TrimSpace(cfg.AnthropicAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "anthropic provider requested but ANTHROPIC_API_KEY is unavailable"}
		}
		return cfg, model, nil
	case "huggingface":
		model := firstNonEmpty(requestedModel, s.huggingFaceDefaultModel)
		cfg.DefaultModel = model
		cfg.EmbeddingModel = s.huggingFaceEmbeddingModel
		if strings.TrimSpace(cfg.HuggingFaceAPIKey) == "" {
			return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: "huggingface provider requested but HUGGINGFACE_API_KEY or HF_TOKEN is unavailable"}
		}
		return cfg, model, nil
	default:
		return cfg, "", personaRequestError{StatusCode: http.StatusBadRequest, Message: "unsupported provider"}
	}
}

func personaLLMRequiresDedicatedClient(req *personaLLMRequest) bool {
	if req == nil {
		return false
	}

	if normalizePersonaProvider(req.Provider) != "" {
		return true
	}
	if req.OpenAI == nil {
		return false
	}
	if strings.TrimSpace(req.OpenAI.APIKey) != "" {
		return true
	}
	if strings.TrimSpace(req.OpenAI.BaseURL) != "" {
		return true
	}
	if strings.TrimSpace(req.OpenAI.Organization) != "" {
		return true
	}
	if strings.TrimSpace(req.OpenAI.Project) != "" {
		return true
	}

	return req.OpenAI.UseServerFallback
}

func normalizePersonaProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "chatgpt", "chat-gpt":
		return "openai"
	case "azure", "azureai", "azure-ai", "azure-openai", "azure_openai", "microsoft", "msai", "windows", "windowsai", "windows-ai":
		return "azure"
	case "xai", "x-ai", "grok", "grock":
		return "xai"
	case "ollama", "local":
		return "ollama"
	case "google", "gemini", "googleai", "google-ai":
		return "google"
	case "anthropic", "claude":
		return "anthropic"
	case "huggingface", "hugging-face", "hf":
		return "huggingface"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func isSupportedPersonaProvider(provider string) bool {
	switch normalizePersonaProvider(provider) {
	case "ollama", "openai", "azure", "xai", "google", "anthropic", "huggingface":
		return true
	default:
		return false
	}
}

func (s *Server) runPersona(ctx context.Context, persona string, req personaRequest, extraSections map[string]string, client llm.Client) (string, string, error) {
	if client == nil {
		return "", "", fmt.Errorf("llm client is not configured")
	}

	systemPrompt := buildPersonaSystemPrompt(persona, req, extraSections)
	prepared, err := client.PrepareContextModel(ctx, strings.TrimSpace(req.Model), systemPrompt)
	if err != nil {
		return "", "", err
	}
	defer client.CleanupPreparedModel(prepared)

	prepared.PromptHint = req.Prompt
	output, err := client.GeneratePrepared(ctx, prepared)
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(output), strings.TrimSpace(prepared.BaseModel), nil
}

func buildPersonaSystemPrompt(persona string, req personaRequest, extraSections map[string]string) string {
	sections := []string{
		"PERSONA",
		personaDirective(persona),
		"",
		"REQUEST_SYSTEM_CONTEXT",
		nonEmptyOrPlaceholder(req.System),
		"",
		"REQUEST_HISTORY",
		formatPersonaHistory(req.History),
		"",
		"REQUEST_CONTEXT_JSON",
		formatPersonaJSON(req.Context),
		"",
	}

	if len(extraSections) > 0 {
		keys := make([]string, 0, len(extraSections))
		for key := range extraSections {
			if strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sections = append(sections, "CHAIN_SECTION: "+strings.TrimSpace(key))
			sections = append(sections, nonEmptyOrPlaceholder(extraSections[key]))
			sections = append(sections, "")
		}
	}

	sections = append(sections, "USER_INSTRUCTION_CHANNEL")
	sections = append(sections, "The caller prompt is supplied separately as runtime user input.")
	return strings.Join(sections, "\n")
}

func personaDirective(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "instruct":
		return "Follow USER_INSTRUCTION exactly and directly. Do not add preambles."
	case "roleplay":
		return "Write an in-character response that strictly respects REQUEST_SYSTEM_CONTEXT, REQUEST_HISTORY, and REQUEST_CONTEXT_JSON."
	case "narrate":
		return "Write narrative prose that links actions and scene details from REQUEST_SYSTEM_CONTEXT, REQUEST_HISTORY, and REQUEST_CONTEXT_JSON."
	case "reasoning_parse":
		return "Extract intent, constraints, key facts, and unknowns from USER_INSTRUCTION and context. Return concise structured text."
	case "reasoning_deliberate":
		return "Propose a concise approach and rationale using USER_INSTRUCTION and any CHAIN_SECTION entries. Keep reasoning summary brief."
	case "reasoning_final":
		return "Return the best final answer for USER_INSTRUCTION using context and chain sections. Be direct and actionable."
	default:
		return "Use USER_INSTRUCTION as the primary request and context sections as background constraints."
	}
}

func nonEmptyOrPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}

func formatPersonaHistory(history []personaMessage) string {
	if len(history) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(history))
	for _, msg := range history {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", role, content))
	}
	if len(lines) == 0 {
		return "(none)"
	}
	return strings.Join(lines, "\n")
}

func formatPersonaJSON(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "(none)"
	}

	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err == nil {
		return strings.TrimSpace(out.String())
	}

	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.enqueueJob(w, r)
	case http.MethodGet:
		s.listJobs(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) enqueueJob(w http.ResponseWriter, r *http.Request) {
	var req enqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}

	if len(req.Metadata) == 0 {
		req.Metadata = []byte(`{}`)
	}
	if s.v3Enabled {
		var payload map[string]any
		if err := json.Unmarshal(req.Metadata, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "metadata must be a JSON object")
			return
		}
		if payload == nil {
			payload = map[string]any{}
		}
		if _, ok := payload["runtime"]; !ok {
			payload["runtime"] = "v3"
		}
		updated, err := json.Marshal(payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare metadata")
			return
		}
		req.Metadata = updated
	}

	enriched, _, enrichErr := s.enrichJobMetadata(r.Context(), req.Metadata, ScrumCard{})
	if enrichErr != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("model setup failed: %v", enrichErr))
		return
	}
	req.Metadata = enriched

	job, err := s.repo.EnqueueJob(r.Context(), req.Instruction, req.Pipeline, req.Metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"job": job,
	})
}

func safeStatusError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unavailable"
	}
	if len(value) > 300 {
		return value[:300] + "...[truncated]"
	}
	return value
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseInt(r.URL.Query().Get("limit"), 20)
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	jobs, err := s.repo.ListJobs(r.Context(), status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
	})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	idText = strings.TrimSpace(strings.Trim(idText, "/"))
	if idText == "" {
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	if strings.HasSuffix(idText, "/inspection") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/inspection")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.inspectJob(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/feedback") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/feedback")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.submitJobFeedback(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/interrupt") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/interrupt")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.interruptJob(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/replan") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/replan")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.replanJob(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/cancel") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/cancel")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.cancelJob(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	details, err := s.repo.GetJobDetails(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, details)
}

func (s *Server) inspectJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	limit := parseInt(r.URL.Query().Get("limit"), 200)
	if limit < 1 {
		limit = 200
	}
	inspection, err := s.repo.GetJobInspection(r.Context(), jobID, limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, inspection)
}

func (s *Server) submitJobFeedback(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Feedback = strings.TrimSpace(req.Feedback)
	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}

	job, err := s.repo.SubmitJobFeedback(r.Context(), jobID, req.Feedback)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job has no pending input request")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func (s *Server) interruptJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Feedback = strings.TrimSpace(req.Feedback)
	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}

	job, err := s.repo.InterruptJob(r.Context(), jobID, req.Feedback)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req cancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	job, err := s.repo.CancelJob(r.Context(), jobID, req.Reason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func (s *Server) replanJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Feedback = strings.TrimSpace(req.Feedback)
	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}

	job, err := s.repo.ReplanJob(r.Context(), jobID, req.Feedback)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listMemory(w, r)
	case http.MethodPost:
		s.addMemory(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("job_id")), 10, 64)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	items, err := s.repo.ListMemoryCandidates(r.Context(), jobID, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidates": items,
	})
}

func (s *Server) handleMemoryCandidateByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/v1/memory-candidates/")
	idText = strings.TrimSpace(strings.Trim(idText, "/"))
	if idText == "" {
		writeError(w, http.StatusBadRequest, "candidate id is required")
		return
	}

	if strings.HasSuffix(idText, "/promote") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/promote")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		s.promoteMemoryCandidate(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/reject") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/reject")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		s.rejectMemoryCandidate(w, r, id)
		return
	}

	if r.Method == http.MethodDelete {
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		if err := s.repo.DeleteMemoryCandidate(r.Context(), id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "memory candidate not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid candidate id")
		return
	}
	item, err := s.repo.GetMemoryCandidate(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidate": item,
	})
}

func (s *Server) addMemory(w http.ResponseWriter, r *http.Request) {
	var req memoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	embedding, err := s.llmClient.Embedding(r.Context(), req.Content)
	if err != nil {
		// Memory ingest still works without embeddings; retrieval will still use tags/fallback.
		embedding = nil
	}

	chunk, err := s.repo.AddMemoryChunk(r.Context(), req.Source, req.Kind, req.Content, req.Tags, embedding)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"memory": chunk,
	})
}

func (s *Server) handleMemoryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	facets, err := s.repo.ListMemoryCategories(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": facets})
}

func (s *Server) handleMemoryTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	facets, err := s.repo.ListMemoryTags(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": facets})
}

func (s *Server) handleResearchIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository is not configured")
		return
	}

	var req researchIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	includeOfficial := true
	if req.IncludeOfficialSources != nil {
		includeOfficial = *req.IncludeOfficialSources
	}
	sourcePrefix := strings.TrimSpace(req.Source)
	if sourcePrefix == "" {
		sourcePrefix = research.DefaultSourcePrefix
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = model.MemoryKindReference
	}
	slug := research.SanitizeToken(topic)
	if slug == "" {
		slug = fmt.Sprintf("topic-%d", time.Now().Unix())
	}

	documents := []research.Document{}
	warnings := []string{}
	if includeOfficial {
		fetched, fetchWarnings, err := research.FetchOfficialDocuments(r.Context(), topic)
		warnings = append(warnings, fetchWarnings...)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		documents = append(documents, fetched...)
	}
	if len(documents) == 0 {
		writeError(w, http.StatusBadRequest, "no research documents found for topic")
		return
	}

	prepared := research.PrepareChunks(documents, research.PrepareOptions{
		Topic:        topic,
		Slug:         slug,
		SourcePrefix: sourcePrefix,
		Tags:         req.Tags,
		ChunkSize:    req.ChunkSize,
		Overlap:      req.Overlap,
		MaxChunks:    req.MaxChunks,
	})
	if len(prepared) == 0 {
		writeError(w, http.StatusBadRequest, "no ingestible research chunks produced")
		return
	}

	storedSources := make([]string, 0, len(prepared))
	var storedTags []string
	for _, chunk := range prepared {
		embedding, err := s.llmClient.Embedding(r.Context(), chunk.Content)
		if err != nil {
			embedding = nil
		}
		if _, err := s.repo.AddMemoryChunk(r.Context(), chunk.Source, kind, chunk.Content, chunk.Tags, embedding); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		storedSources = append(storedSources, chunk.Source)
		if len(storedTags) == 0 {
			storedTags = chunk.Tags
		}
	}

	sources := make([]string, 0, len(documents))
	for _, doc := range documents {
		if url := research.DocumentURL(doc.Content); url != "" {
			sources = append(sources, url)
		}
	}

	writeJSON(w, http.StatusCreated, researchIngestResponse{
		Topic:             topic,
		Slug:              slug,
		SourcePrefix:      sourcePrefix,
		StoredChunks:      len(storedSources),
		Tags:              storedTags,
		Warnings:          warnings,
		Dossier:           research.BuildDossier(topic, 0, time.Now(), documents, storedTags, sourcePrefix, len(storedSources)),
		Sources:           sources,
		StoredChunkSource: storedSources,
	})
}

func (s *Server) promoteMemoryCandidate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	var req memoryCandidatePromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	tier := strings.ToLower(strings.TrimSpace(req.Tier))
	if tier == "" {
		tier = model.MemoryCandidateStatusApproved
	}
	if tier != model.MemoryCandidateStatusApproved && tier != model.MemoryCandidateStatusDurable {
		writeError(w, http.StatusBadRequest, "tier must be approved or durable")
		return
	}

	candidate, err := s.repo.GetMemoryCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	embed, err := s.llmClient.Embedding(r.Context(), candidate.Content)
	if err != nil {
		embed = nil
	}
	tags := append(memoryCandidateScopeTags(candidate), candidate.CandidateKind)
	source := fmt.Sprintf("job:%d:reviewed:%s", candidate.JobID, tier)
	chunk, err := s.repo.AddMemoryChunk(r.Context(), source, candidate.CandidateKind, candidate.Content, tags, embed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repo.UpdateMemoryCandidateStatus(r.Context(), candidate.ID, tier); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := s.repo.GetMemoryCandidate(r.Context(), candidate.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.MemoryCandidatePromotionResult{
		Candidate: updated,
		Memory:    &chunk,
	})
}

func (s *Server) rejectMemoryCandidate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	item, err := s.repo.GetMemoryCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repo.UpdateMemoryCandidateStatus(r.Context(), candidateID, model.MemoryCandidateStatusRejected); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.Status = model.MemoryCandidateStatusRejected
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidate": item,
	})
}

func memoryCandidateScopeTags(candidate model.MemoryCandidate) []string {
	if len(candidate.Provenance) == 0 || !json.Valid(candidate.Provenance) {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.Provenance, &payload); err != nil {
		return nil
	}
	raw, ok := payload["scope_tags"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(items))
	for _, item := range items {
		tag := strings.TrimSpace(fmt.Sprintf("%v", item))
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func (s *Server) handleAdminMigrateFresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.repo.MigrateFresh(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func parseInt(v string, fallback int) int {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func parsePositiveInt(v string, fallback int) int {
	parsed := parseInt(v, fallback)
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func Run(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	}
}
