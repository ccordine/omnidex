package main

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestRuntimeWebSearchConfigSeparatesRawTransportFromEvidenceBudgets(t *testing.T) {
	providers := []websearch.ProviderID{websearch.ProviderBrave, websearch.ProviderGoogle}
	got := runtimeWebSearchConfig(config.Config{
		WebSearchTimeout:         15 * time.Second,
		WebSearchPerSourceBudget: 3_000,
		WebSearchTotalBudget:     6_000,
	}, providers)

	if got.MaxResponseBytes != 1<<20 || got.PerDocumentBytes != 3_000 || got.TotalDocumentBytes != 6_000 {
		t.Fatalf("runtime web search budgets=%+v", got)
	}
	providers[0] = websearch.ProviderYahoo
	if got.Providers[0] != websearch.ProviderBrave {
		t.Fatalf("runtime provider authority aliased caller storage: %v", got.Providers)
	}
	if _, err := websearch.New(got); err != nil {
		t.Fatalf("runtime web search config is invalid: %v", err)
	}
}
