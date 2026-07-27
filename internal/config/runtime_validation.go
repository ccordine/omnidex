package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

const maxConfiguredRetrievalLimit = 64

func validateRuntimeConfig(cfg Config) error {
	if cfg.AnthropicMaxTokens < 1 {
		return fmt.Errorf("ANTHROPIC_MAX_TOKENS must be positive, received %d", cfg.AnthropicMaxTokens)
	}
	if cfg.WorkerCount < 1 {
		return fmt.Errorf("WORKER_COUNT must be at least 1, received %d", cfg.WorkerCount)
	}
	if cfg.WorkerPollInterval <= 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must be positive, received %s", cfg.WorkerPollInterval)
	}
	if cfg.RequestTimeout <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT must be positive, received %s", cfg.RequestTimeout)
	}
	if cfg.RetrievalLimit < 1 || cfg.RetrievalLimit > maxConfiguredRetrievalLimit {
		return fmt.Errorf("RETRIEVAL_LIMIT must be between 1 and %d, received %d", maxConfiguredRetrievalLimit, cfg.RetrievalLimit)
	}
	if cfg.ContextCharBudget < 1 {
		return fmt.Errorf("CONTEXT_CHAR_BUDGET must be positive, received %d", cfg.ContextCharBudget)
	}
	if err := llm.ValidateInferenceContextTokens(cfg.InferenceContextTokens); err != nil {
		return fmt.Errorf("INFERENCE_CONTEXT_TOKENS is invalid: %w", err)
	}
	if cfg.SufficientContextChars < 1 {
		return fmt.Errorf("SUFFICIENT_CONTEXT_CHARS must be positive, received %d", cfg.SufficientContextChars)
	}
	if cfg.MemoryInferenceMaxItems < 0 {
		return fmt.Errorf("MEMORY_INFERENCE_MAX_ITEMS cannot be negative, received %d", cfg.MemoryInferenceMaxItems)
	}
	if cfg.TournamentChunkChars < 500 {
		return fmt.Errorf("TOURNAMENT_CHUNK_CHARS must be at least 500, received %d", cfg.TournamentChunkChars)
	}
	if cfg.TournamentSummaryChars < 120 {
		return fmt.Errorf("TOURNAMENT_SUMMARY_CHARS must be at least 120, received %d", cfg.TournamentSummaryChars)
	}
	if cfg.TournamentSummaryChars > cfg.TournamentChunkChars {
		return fmt.Errorf("TOURNAMENT_SUMMARY_CHARS cannot exceed TOURNAMENT_CHUNK_CHARS")
	}
	if cfg.TournamentMaxRounds < 1 || cfg.TournamentMaxRounds > 8 {
		return fmt.Errorf("TOURNAMENT_MAX_ROUNDS must be between 1 and 8, received %d", cfg.TournamentMaxRounds)
	}
	if cfg.WorkspaceMaxFiles < 1 {
		return fmt.Errorf("WORKSPACE_MAX_FILES must be positive, received %d", cfg.WorkspaceMaxFiles)
	}
	if cfg.WorkspaceContextBudget < 1 {
		return fmt.Errorf("WORKSPACE_CONTEXT_BUDGET must be positive, received %d", cfg.WorkspaceContextBudget)
	}
	if cfg.OllamaRestartTimeout <= 0 {
		return fmt.Errorf("OLLAMA_RESTART_TIMEOUT must be positive, received %s", cfg.OllamaRestartTimeout)
	}
	if cfg.HallucinationRetryLimit < 1 || cfg.HallucinationRetryLimit > 6 {
		return fmt.Errorf("HALLUCINATION_RETRY_LIMIT must be between 1 and 6, received %d", cfg.HallucinationRetryLimit)
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
	if strings.TrimSpace(cfg.DefaultModel) == "" {
		return fmt.Errorf("default model is required for LLM_PROVIDER=%s", cfg.LLMProvider)
	}
	if strings.TrimSpace(cfg.EmbeddingModel) == "" {
		return fmt.Errorf("embedding model is required for EMBEDDING_PROVIDER=%s", cfg.EmbeddingProvider)
	}
	if cfg.V3Enabled && strings.TrimSpace(cfg.SkillsRoot) == "" {
		return fmt.Errorf("OMNIDEX_SKILLS_ROOT is required when OMNIDEX_V3_ENABLED=true")
	}
	return nil
}

func validateWebSearchProviders(providers []string) error {
	known := map[string]struct{}{
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
