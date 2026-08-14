package llmprovider

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
)

func TestLazyTransportsPermitAbsentAuthorityUntilActualUse(t *testing.T) {
	transports := NewLazyFromConfig(config.Config{
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	if transports.Stations == nil || transports.Embeddings == nil {
		t.Fatalf("lazy transports are missing: %+v", transports)
	}
	if err := transports.Stations.RequireExactPreparedContract(); err != nil {
		t.Fatalf("contract capability resolved provider authority: %v", err)
	}

	selection := llm.ProviderIdentitySelection{
		Model: "unused-model", NativeContextLimit: llm.DefaultInferenceContextTokens,
	}
	observed, err := transports.Stations.DiscoverProviderIdentityEvidence(
		context.Background(), selection, strings.Repeat("a", 64),
	)
	if err == nil || !strings.Contains(err.Error(), "LLM_PROVIDER is not configured") {
		t.Fatalf("discovery error=%v, want absent authority", err)
	}
	if evidenceErr := observed.Evidence.ValidateFailure(selection, nil); evidenceErr != nil {
		t.Fatalf("absent authority lacks exact undispatched evidence: %v", evidenceErr)
	}
	for _, operation := range observed.Evidence.Operations {
		if operation.RequestDisposition != llm.ProviderRequestNotDispatched {
			t.Fatalf("absent authority contacted a provider: %+v", operation)
		}
	}

	if _, err := transports.Embeddings.Embedding(context.Background(), "semantic need"); err == nil || !strings.Contains(err.Error(), "EMBEDDING_PROVIDER is not configured") {
		t.Fatalf("embedding error=%v, want absent authority", err)
	}
}

func TestLazyTransportsDeferMalformedConfiguredAuthorityUntilUse(t *testing.T) {
	transports := NewLazyFromConfig(config.Config{
		LLMProvider: "openai", EmbeddingProvider: "qwen", EmbeddingModel: "embedding",
		CompatibleProviders: map[string]config.CompatibleProviderConfig{
			"qwen": {BaseURL: "not-an-http-url"},
		},
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	if err := transports.Stations.RequireExactPreparedContract(); err != nil {
		t.Fatalf("startup resolved malformed dormant authority: %v", err)
	}
	selection := llm.ProviderIdentitySelection{
		Model: "unused-model", NativeContextLimit: llm.DefaultInferenceContextTokens,
	}
	if _, err := transports.Stations.DiscoverProviderIdentityEvidence(
		context.Background(), selection, strings.Repeat("b", 64),
	); err == nil || !strings.Contains(err.Error(), "exact prepared station contract") {
		t.Fatalf("station use error=%v, want unsupported authority", err)
	}
	if _, err := transports.Embeddings.Embedding(context.Background(), "semantic need"); err == nil || !strings.Contains(err.Error(), "absolute HTTP(S) URL") {
		t.Fatalf("embedding use error=%v, want malformed authority", err)
	}
}
