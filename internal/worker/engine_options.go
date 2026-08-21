package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/station"
)

func validateWorkerOptions(opts Options) error {
	if _, ok := catalog.Lookup(opts.InferenceProvider); !ok {
		return fmt.Errorf("inference_provider %q is unsupported", strings.TrimSpace(opts.InferenceProvider))
	}
	if opts.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be at least 1, received %d", opts.WorkerCount)
	}
	if opts.FragmentConcurrency < 1 || opts.FragmentConcurrency > maxDirectCodingFragmentConcurrency {
		return fmt.Errorf("fragment_concurrency must be between 1 and %d, received %d", maxDirectCodingFragmentConcurrency, opts.FragmentConcurrency)
	}
	if opts.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive, received %s", opts.PollInterval)
	}
	if err := llm.ValidateInferenceContextTokens(opts.InferenceContextTokens); err != nil {
		return fmt.Errorf("inference_context_tokens is invalid: %w", err)
	}
	for stationID, modelName := range opts.Models.Stations {
		if err := stationID.Validate(); err != nil {
			return fmt.Errorf("models.stations: %w", err)
		}
		if strings.TrimSpace(modelName) == "" {
			return fmt.Errorf("models.stations[%q] is required", stationID)
		}
	}
	if root := strings.TrimSpace(opts.Workspace.HostRoot); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("workspace.host_root must be absolute when configured, received %q", root)
	}
	if root := strings.TrimSpace(opts.Workspace.Root); root != "" && !filepath.IsAbs(root) {
		return fmt.Errorf("workspace.root must be absolute when configured, received %q", root)
	}
	if opts.Logger == nil {
		return fmt.Errorf("logger is required")
	}
	return nil
}

func normalizeWorkerOptions(opts Options) Options {
	opts.EmbeddingProvider = strings.TrimSpace(opts.EmbeddingProvider)
	if definition, ok := catalog.Lookup(opts.InferenceProvider); ok {
		opts.InferenceProvider = definition.ID
	}
	opts.EmbeddingModel = strings.TrimSpace(opts.EmbeddingModel)

	stationModels := make(map[station.ID]string, len(opts.Models.Stations))
	for stationID, modelName := range opts.Models.Stations {
		stationModels[stationID] = strings.TrimSpace(modelName)
	}
	opts.Models.Stations = stationModels
	opts.Workspace.Root = strings.TrimSpace(opts.Workspace.Root)
	opts.Workspace.HostRoot = strings.TrimSpace(opts.Workspace.HostRoot)
	return opts
}
