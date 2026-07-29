package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

func validateWorkerOptions(opts Options) error {
	if opts.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be at least 1, received %d", opts.WorkerCount)
	}
	if opts.FragmentConcurrency < 1 || opts.FragmentConcurrency > maxDirectCodingFragmentConcurrency {
		return fmt.Errorf("fragment_concurrency must be between 1 and %d, received %d", maxDirectCodingFragmentConcurrency, opts.FragmentConcurrency)
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
	if strings.TrimSpace(opts.EmbeddingProvider) == "" {
		return fmt.Errorf("embedding_provider is required")
	}
	if strings.TrimSpace(opts.EmbeddingModel) == "" {
		return fmt.Errorf("embedding_model is required")
	}

	modelRoles := []struct {
		name  string
		value string
	}{
		{name: "default", value: opts.Models.Default},
		{name: "fast", value: opts.Models.Fast},
		{name: "glue", value: opts.Models.Glue},
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

	if opts.Workspace.Enabled && strings.TrimSpace(opts.Workspace.Root) == "" {
		return fmt.Errorf("workspace.root is required when workspace scanning is enabled")
	}
	if root := strings.TrimSpace(opts.Workspace.HostRoot); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("workspace.host_root must be absolute when configured, received %q", root)
	}
	if opts.Workspace.MaxFiles < 1 {
		return fmt.Errorf("workspace.max_files must be positive, received %d", opts.Workspace.MaxFiles)
	}
	if opts.Workspace.ContextBudget < 1 {
		return fmt.Errorf("workspace.context_budget must be positive, received %d", opts.Workspace.ContextBudget)
	}
	if strings.TrimSpace(opts.SkillsRoot) == "" {
		return fmt.Errorf("skills_root is required")
	}
	if opts.Logger == nil {
		return fmt.Errorf("logger is required")
	}
	return nil
}

func normalizeWorkerOptions(opts Options) Options {
	opts.Models.Default = strings.TrimSpace(opts.Models.Default)
	opts.Models.Fast = strings.TrimSpace(opts.Models.Fast)
	opts.Models.Glue = strings.TrimSpace(opts.Models.Glue)
	opts.Models.Reasoning = strings.TrimSpace(opts.Models.Reasoning)
	opts.Models.Tagging = strings.TrimSpace(opts.Models.Tagging)
	opts.Models.Plan = strings.TrimSpace(opts.Models.Plan)
	opts.Models.Analyze = strings.TrimSpace(opts.Models.Analyze)
	opts.Models.Response = strings.TrimSpace(opts.Models.Response)
	opts.Models.Search = strings.TrimSpace(opts.Models.Search)
	opts.Models.Memory = strings.TrimSpace(opts.Models.Memory)
	opts.EmbeddingProvider = strings.TrimSpace(opts.EmbeddingProvider)
	opts.EmbeddingModel = strings.TrimSpace(opts.EmbeddingModel)

	specialistModels := make(map[string]string, len(opts.Models.Specialist))
	for role, modelName := range opts.Models.Specialist {
		specialistModels[strings.TrimSpace(role)] = strings.TrimSpace(modelName)
	}
	opts.Models.Specialist = specialistModels
	opts.Workspace.Root = strings.TrimSpace(opts.Workspace.Root)
	opts.Workspace.HostRoot = strings.TrimSpace(opts.Workspace.HostRoot)
	opts.SkillsRoot = strings.TrimSpace(opts.SkillsRoot)
	return opts
}
