package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

const (
	minOllamaProbeTimeout = 10 * time.Second
	maxOllamaProbeTimeout = 15 * time.Second
	ollamaProbeAttempts   = 2
	ollamaProbeRetryDelay = 400 * time.Millisecond
)

type researchStatusResponse struct {
	GenerationProvider generationProviderStatus `json:"generation_provider"`
	Ollama             ollamaRuntimeStatus      `json:"ollama,omitempty"`
	WebSearch          webSearchRuntimeStatus   `json:"web_search"`
	ResearchRunnable   bool                     `json:"research_runnable"`
	Warnings           []string                 `json:"warnings,omitempty"`
}

type generationProviderStatus struct {
	Provider  string `json:"provider"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

type ollamaRuntimeStatus struct {
	BaseURL             string   `json:"base_url,omitempty"`
	Reachable           bool     `json:"reachable"`
	ConfiguredModels    []string `json:"configured_models,omitempty"`
	AvailableModels     []string `json:"available_models,omitempty"`
	MissingModels       []string `json:"missing_models,omitempty"`
	EmbeddingModel      string   `json:"embedding_model,omitempty"`
	EmbeddingAvailable  bool     `json:"embedding_available"`
	LastProviderError   string   `json:"last_provider_error,omitempty"`
	RecommendedHostHint string   `json:"recommended_host_hint,omitempty"`
}

type webSearchRuntimeStatus struct {
	Enabled           bool                   `json:"enabled"`
	Providers         []string               `json:"providers,omitempty"`
	ReachableProvider bool                   `json:"reachable_provider"`
	LastProviderError string                 `json:"last_provider_error,omitempty"`
	Probes            []webSearchProbeStatus `json:"probes,omitempty"`
}

type webSearchProbeStatus struct {
	Provider   string `json:"provider"`
	TargetURL  string `json:"target_url,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) ollamaProbeTimeout() time.Duration {
	timeout := minOllamaProbeTimeout
	if s.requestTimeout > 0 && s.requestTimeout < timeout {
		timeout = s.requestTimeout
	}
	if timeout > maxOllamaProbeTimeout {
		timeout = maxOllamaProbeTimeout
	}
	return timeout
}

func (s *Server) ollamaReachabilityBudget() time.Duration {
	attempts := time.Duration(ollamaProbeAttempts)
	return s.ollamaProbeTimeout()*attempts + ollamaProbeRetryDelay*(attempts-1) + 500*time.Millisecond
}

func (s *Server) handleResearchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.ollamaReachabilityBudget())
	defer cancel()
	writeJSON(w, http.StatusOK, s.collectResearchStatus(ctx))
}

func (s *Server) collectResearchStatus(ctx context.Context) researchStatusResponse {
	provider := strings.ToLower(strings.TrimSpace(s.defaultProvider))
	if provider == "" {
		provider = "ollama"
	}
	status := researchStatusResponse{
		GenerationProvider: generationProviderStatus{Provider: provider, Reachable: true},
		WebSearch:          s.collectWebSearchStatus(ctx),
		ResearchRunnable:   true,
	}
	if provider == "ollama" {
		status.Ollama = s.collectOllamaRuntimeStatus(ctx)
		status.GenerationProvider.Reachable = status.Ollama.Reachable
		status.GenerationProvider.Error = status.Ollama.LastProviderError
		if !status.Ollama.Reachable {
			status.ResearchRunnable = false
			status.Warnings = append(status.Warnings, "ollama is unreachable from the core process; queued jobs may fail until OLLAMA_BASE_URL is reachable from the core container")
		}
		if len(status.Ollama.MissingModels) > 0 {
			status.Warnings = append(status.Warnings, "one or more configured Ollama models are missing")
		}
	}
	if status.WebSearch.Enabled && !status.WebSearch.ReachableProvider {
		status.Warnings = append(status.Warnings, "no configured web search provider passed the reachability probe; research may run in degraded mode from local/docs/memory context only")
	}
	return status
}

func (s *Server) collectOllamaRuntimeStatus(ctx context.Context) ollamaRuntimeStatus {
	status := ollamaRuntimeStatus{
		BaseURL:             normalizeURL(firstNonEmpty(s.ollamaBaseURL, "http://host.docker.internal:11434")),
		ConfiguredModels:    s.configuredOllamaModels(),
		EmbeddingModel:      strings.TrimSpace(s.ollamaEmbeddingModel),
		RecommendedHostHint: "If core runs in Docker, prefer OLLAMA_BASE_URL=http://host.docker.internal:11434 or the docker-compose.host-ollama.yml override; host Ollama must listen beyond loopback when using bridge networking.",
	}
	models, err := s.probeOllamaTags(ctx)
	if err != nil {
		status.LastProviderError = err.Error()
		return status
	}
	status.Reachable = true
	status.AvailableModels = models
	available := make(map[string]struct{}, len(models))
	for _, model := range models {
		available[strings.TrimSpace(model)] = struct{}{}
	}
	for _, model := range status.ConfiguredModels {
		if _, ok := available[model]; !ok {
			status.MissingModels = append(status.MissingModels, model)
		}
	}
	if status.EmbeddingModel != "" {
		_, status.EmbeddingAvailable = available[status.EmbeddingModel]
	}
	return status
}

func (s *Server) configuredOllamaModels() []string {
	values := []string{s.ollamaDefaultModel}
	if s.ollamaEmbeddingModel != "" {
		values = append(values, s.ollamaEmbeddingModel)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		model := strings.TrimSpace(value)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func (s *Server) collectWebSearchStatus(ctx context.Context) webSearchRuntimeStatus {
	status := webSearchRuntimeStatus{
		Enabled:   s.webSearchEnabled,
		Providers: configuredWebSearchProviders(s.webSearchProviders),
	}
	if !status.Enabled {
		return status
	}
	timeout := s.webSearchTimeout
	if timeout <= 0 || timeout > maxOllamaProbeTimeout {
		timeout = maxOllamaProbeTimeout
	}
	for _, provider := range status.Providers {
		target := webSearchProbeURL(provider)
		probe := webSearchProbeStatus{Provider: provider, TargetURL: target}
		if target == "" {
			probe.Error = "no probe URL mapping"
			status.LastProviderError = probe.Error
			status.Probes = append(status.Probes, probe)
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		code, err := probeHTTP(probeCtx, target)
		cancel()
		if err != nil {
			probe.Error = err.Error()
			status.LastProviderError = fmt.Sprintf("%s: %s", provider, err.Error())
		} else {
			probe.StatusCode = code
			probe.Reachable = true
			status.ReachableProvider = true
		}
		status.Probes = append(status.Probes, probe)
	}
	return status
}

func (s *Server) probeOllamaTags(ctx context.Context) ([]string, error) {
	baseURL := normalizeURL(firstNonEmpty(s.ollamaBaseURL, "http://host.docker.internal:11434"))
	client := ollama.New(baseURL, "", "", s.ollamaProbeTimeout())
	var lastErr error
	for attempt := 0; attempt < ollamaProbeAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, ollamaProbeRetryDelay); err != nil {
				return nil, err
			}
		}
		models, err := client.ListTags(ctx)
		if err == nil {
			return models, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func configuredWebSearchProviders(values []string) []string {
	if len(values) == 0 {
		return []string{"duckduckgo", "google", "reddit"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		provider := strings.ToLower(strings.TrimSpace(value))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	if len(out) == 0 {
		return []string{"duckduckgo", "google", "reddit"}
	}
	return out
}

func webSearchProbeURL(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "duckduckgo":
		return "https://duckduckgo.com"
	case "google":
		return "https://www.google.com"
	case "reddit":
		return "https://www.reddit.com"
	case "yahoo":
		return "https://search.yahoo.com"
	default:
		if strings.Contains(provider, ".") {
			return "https://" + strings.TrimRight(provider, "/")
		}
		return ""
	}
}

func probeHTTP(ctx context.Context, endpoint string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "omnidex-status/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 256)
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("status=%d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func normalizeURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	parsed.Host = normalizeURLHost(parsed.Host)
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeURLHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if hostname, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(strings.TrimSuffix(hostname, "."), port)
	}
	return strings.TrimSuffix(host, ".")
}

func truncateStatusText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
