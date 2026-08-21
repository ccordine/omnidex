package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/station"
)

func TestRuntimeBrowserContextRelevanceIsExplicitAndResolvesExactConfiguredModel(t *testing.T) {
	broker, model, err := runtimeBrowserContextRelevance(config.Config{
		ContextRelevanceProvider: config.ContextRelevanceProviderServer,
	})
	if err != nil || broker != nil || model != "" {
		t.Fatalf("server provider broker/model/error=%#v/%q/%v", broker, model, err)
	}

	_, _, err = runtimeBrowserContextRelevance(config.Config{
		ContextRelevanceProvider: config.ContextRelevanceProviderBrowserWebGPU,
		StationModels:            map[station.ID]string{},
	})
	if err == nil || !strings.Contains(err.Error(), "OMNI_CONTEXT_RELEVANCE_MODEL") {
		t.Fatalf("missing model error=%v", err)
	}

	broker, model, err = runtimeBrowserContextRelevance(config.Config{
		ContextRelevanceProvider: config.ContextRelevanceProviderBrowserWebGPU,
		StationModels: map[station.ID]string{
			station.ContextRelevance: "exact-browser-model",
		},
	})
	if err != nil || broker == nil || model != "exact-browser-model" {
		t.Fatalf("browser provider broker/model/error=%#v/%q/%v", broker, model, err)
	}
}
