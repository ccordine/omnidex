package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

func validateWorkerOptions(opts Options) error {
	if opts.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be at least 1, received %d", opts.WorkerCount)
	}
	if opts.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive, received %s", opts.PollInterval)
	}
	if opts.RetrievalLimit < 1 || opts.RetrievalLimit > maxMemoryRetrievalLimit {
		return fmt.Errorf("retrieval_limit must be between 1 and %d, received %d", maxMemoryRetrievalLimit, opts.RetrievalLimit)
	}
	if opts.ContextBudget < 1 {
		return fmt.Errorf("context_budget must be positive, received %d", opts.ContextBudget)
	}
	if err := llm.ValidateInferenceContextTokens(opts.InferenceContextTokens); err != nil {
		return fmt.Errorf("inference_context_tokens is invalid: %w", err)
	}

	modelRoles := []struct {
		name  string
		value string
	}{
		{name: "default", value: opts.Models.Default},
		{name: "fast", value: opts.Models.Fast},
		{name: "reasoning", value: opts.Models.Reasoning},
		{name: "tagging", value: opts.Models.Tagging},
		{name: "plan", value: opts.Models.Plan},
		{name: "analyze", value: opts.Models.Analyze},
		{name: "response", value: opts.Models.Response},
		{name: "search", value: opts.Models.Search},
		{name: "memory", value: opts.Models.Memory},
	}
	for _, role := range modelRoles {
		if strings.TrimSpace(role.value) == "" {
			return fmt.Errorf("models.%s is required", role.name)
		}
	}
	if len(opts.Models.Specialist) == 0 {
		return fmt.Errorf("models.specialist must define at least one specialist model")
	}
	for role, modelName := range opts.Models.Specialist {
		if strings.TrimSpace(role) == "" {
			return fmt.Errorf("models.specialist contains an empty role")
		}
		if strings.TrimSpace(modelName) == "" {
			return fmt.Errorf("models.specialist[%q] is required", role)
		}
	}

	if opts.Cognition.SufficientContextChars < 1 {
		return fmt.Errorf("sufficient_context_chars must be positive, received %d", opts.Cognition.SufficientContextChars)
	}
	if opts.Cognition.MemoryInferenceMaxItems < 0 {
		return fmt.Errorf("memory_inference_max_items cannot be negative, received %d", opts.Cognition.MemoryInferenceMaxItems)
	}
	if opts.Tournament.ChunkChars < 500 {
		return fmt.Errorf("tournament.chunk_chars must be at least 500, received %d", opts.Tournament.ChunkChars)
	}
	if opts.Tournament.SummaryChars < 120 {
		return fmt.Errorf("tournament.summary_chars must be at least 120, received %d", opts.Tournament.SummaryChars)
	}
	if opts.Tournament.SummaryChars > opts.Tournament.ChunkChars {
		return fmt.Errorf("tournament.summary_chars cannot exceed tournament.chunk_chars")
	}
	if opts.Tournament.MaxRounds < 1 || opts.Tournament.MaxRounds > 8 {
		return fmt.Errorf("tournament.max_rounds must be between 1 and 8, received %d", opts.Tournament.MaxRounds)
	}

	if opts.Workspace.Enabled && strings.TrimSpace(opts.Workspace.Root) == "" {
		return fmt.Errorf("workspace.root is required when workspace scanning is enabled")
	}
	if opts.Workspace.MaxFiles < 1 {
		return fmt.Errorf("workspace.max_files must be positive, received %d", opts.Workspace.MaxFiles)
	}
	if opts.Workspace.ContextBudget < 1 {
		return fmt.Errorf("workspace.context_budget must be positive, received %d", opts.Workspace.ContextBudget)
	}
	if opts.HallucinationRetryLimit < 1 || opts.HallucinationRetryLimit > maxHallucinationRetryLimit {
		return fmt.Errorf("hallucination_retry_limit must be between 1 and %d, received %d", maxHallucinationRetryLimit, opts.HallucinationRetryLimit)
	}
	if opts.OllamaRestartTimeout <= 0 {
		return fmt.Errorf("ollama_restart_timeout must be positive, received %s", opts.OllamaRestartTimeout)
	}
	if opts.V3Enabled && strings.TrimSpace(opts.SkillsRoot) == "" {
		return fmt.Errorf("skills_root is required when V3 is enabled")
	}
	if opts.Logger == nil {
		return fmt.Errorf("logger is required")
	}
	return nil
}

func normalizeWorkerOptions(opts Options) Options {
	opts.Models.Default = strings.TrimSpace(opts.Models.Default)
	opts.Models.Fast = strings.TrimSpace(opts.Models.Fast)
	opts.Models.Reasoning = strings.TrimSpace(opts.Models.Reasoning)
	opts.Models.Tagging = strings.TrimSpace(opts.Models.Tagging)
	opts.Models.Plan = strings.TrimSpace(opts.Models.Plan)
	opts.Models.Analyze = strings.TrimSpace(opts.Models.Analyze)
	opts.Models.Response = strings.TrimSpace(opts.Models.Response)
	opts.Models.Search = strings.TrimSpace(opts.Models.Search)
	opts.Models.Memory = strings.TrimSpace(opts.Models.Memory)

	specialistModels := make(map[string]string, len(opts.Models.Specialist))
	for role, modelName := range opts.Models.Specialist {
		specialistModels[strings.TrimSpace(role)] = strings.TrimSpace(modelName)
	}
	opts.Models.Specialist = specialistModels
	opts.Workspace.Root = strings.TrimSpace(opts.Workspace.Root)
	opts.OllamaRestartCommand = strings.TrimSpace(opts.OllamaRestartCommand)
	opts.OllamaBaseURL = strings.TrimSpace(opts.OllamaBaseURL)
	opts.SkillsRoot = strings.TrimSpace(opts.SkillsRoot)
	return opts
}
