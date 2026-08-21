package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/browserinference"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/station"
)

func runtimeBrowserContextRelevance(
	cfg config.Config,
) (*browserinference.ContextRelevanceBroker, string, error) {
	if cfg.ContextRelevanceProvider == config.ContextRelevanceProviderServer {
		return nil, "", nil
	}
	if cfg.ContextRelevanceProvider != config.ContextRelevanceProviderBrowserWebGPU {
		return nil, "", fmt.Errorf(
			"context relevance provider %q is not registered",
			cfg.ContextRelevanceProvider,
		)
	}
	model := strings.TrimSpace(cfg.StationModels[station.ContextRelevance])
	if model == "" {
		return nil, "", fmt.Errorf(
			"browser WebGPU context relevance requires OMNI_CONTEXT_RELEVANCE_MODEL",
		)
	}
	return browserinference.NewContextRelevanceBroker(), model, nil
}
