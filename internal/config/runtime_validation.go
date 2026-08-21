package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

const (
	ContextRelevanceProviderServer        = "server"
	ContextRelevanceProviderBrowserWebGPU = "browser_webgpu"
)

func validateRuntimeConfig(cfg Config) error {
	switch cfg.ContextRelevanceProvider {
	case ContextRelevanceProviderServer, ContextRelevanceProviderBrowserWebGPU:
	default:
		return fmt.Errorf(
			"OMNI_CONTEXT_RELEVANCE_PROVIDER must be %q or %q, received %q",
			ContextRelevanceProviderServer,
			ContextRelevanceProviderBrowserWebGPU,
			cfg.ContextRelevanceProvider,
		)
	}
	if cfg.WorkerCount < 1 {
		return fmt.Errorf("WORKER_COUNT must be at least 1, received %d", cfg.WorkerCount)
	}
	if cfg.CodingFragmentConcurrency < 1 || cfg.CodingFragmentConcurrency > 4 {
		return fmt.Errorf("CODING_FRAGMENT_CONCURRENCY must be between 1 and 4, received %d", cfg.CodingFragmentConcurrency)
	}
	if cfg.WorkerPollInterval <= 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be positive, received %s", cfg.WorkerPollInterval)
	}
	if cfg.RequestTimeout <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT must be positive, received %s", cfg.RequestTimeout)
	}
	if err := llm.ValidateInferenceContextTokens(cfg.InferenceContextTokens); err != nil {
		return fmt.Errorf("INFERENCE_CONTEXT_TOKENS is invalid: %w", err)
	}
	if root := strings.TrimSpace(cfg.WorkspaceHostRoot); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("HOST_WORKSPACE_PATH must be absolute when configured, received %q", root)
	}
	if cfg.RealtimeMaxClients < 1 {
		return fmt.Errorf("REALTIME_MAX_CLIENTS must be at least 1, received %d", cfg.RealtimeMaxClients)
	}
	if cfg.RealtimeStreamMaxAge < time.Minute {
		return fmt.Errorf("REALTIME_STREAM_MAX_AGE must be at least 1m, received %s", cfg.RealtimeStreamMaxAge)
	}
	if cfg.RealtimeHeartbeat < 5*time.Second {
		return fmt.Errorf("REALTIME_HEARTBEAT must be at least 5s, received %s", cfg.RealtimeHeartbeat)
	}
	if cfg.RealtimeHeartbeat >= cfg.RealtimeStreamMaxAge {
		return fmt.Errorf("REALTIME_HEARTBEAT must be shorter than REALTIME_STREAM_MAX_AGE")
	}
	if cfg.RealtimeWriteTimeout < time.Second {
		return fmt.Errorf("REALTIME_WRITE_TIMEOUT must be at least 1s, received %s", cfg.RealtimeWriteTimeout)
	}
	if cfg.UISessionTTL < time.Minute {
		return fmt.Errorf("UI_SESSION_TTL must be at least 1m, received %s", cfg.UISessionTTL)
	}
	if cfg.WebSearchTimeout <= 0 {
		return fmt.Errorf("WEB_SEARCH_TIMEOUT must be positive, received %s", cfg.WebSearchTimeout)
	}
	if cfg.WebSearchPerSourceBudget < 1 {
		return fmt.Errorf("WEB_SEARCH_PER_SOURCE_BUDGET must be positive, received %d", cfg.WebSearchPerSourceBudget)
	}
	if cfg.WebSearchTotalBudget < cfg.WebSearchPerSourceBudget {
		return fmt.Errorf("WEB_SEARCH_TOTAL_BUDGET must be at least WEB_SEARCH_PER_SOURCE_BUDGET")
	}
	if err := validateWebSearchProviders(cfg.WebSearchProviders); err != nil {
		return err
	}
	return nil
}

func validateWebSearchProviders(providers []string) error {
	known := map[string]struct{}{
		"brave":      {},
		"duckduckgo": {},
		"google":     {},
		"reddit":     {},
		"yahoo":      {},
	}
	seen := make(map[string]struct{}, len(providers))
	for _, raw := range providers {
		provider := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := known[provider]; !ok {
			return fmt.Errorf("WEB_SEARCH_PROVIDERS contains unsupported provider %q", raw)
		}
		if _, duplicate := seen[provider]; duplicate {
			return fmt.Errorf("WEB_SEARCH_PROVIDERS contains duplicate provider %q", provider)
		}
		seen[provider] = struct{}{}
	}
	if len(seen) == 0 {
		return fmt.Errorf("WEB_SEARCH_PROVIDERS must contain at least one provider")
	}
	return nil
}
