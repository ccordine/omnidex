package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) requireOllamaModelUnused(ctx context.Context, installedModel string) error {
	config, err := s.envModelConfig()
	if err != nil {
		return fmt.Errorf("load model routing before removal: %w", err)
	}
	configured := config.ModelNames()
	providerConfig := s.providerConfiguration()
	if providerConfig.EmbeddingProvider == "ollama" {
		embedding := strings.TrimSpace(providerConfig.EmbeddingModel)
		if embedding == "" {
			embedding = strings.TrimSpace(providerConfig.ProviderModels["ollama"].Embedding)
		}
		if embedding != "" {
			configured = append(configured, embedding)
		}
	}
	for _, modelName := range configured {
		if ollama.MatchesOllamaModel(modelName, installedModel) {
			return fmt.Errorf("Ollama model %q is still used by global routing", installedModel)
		}
	}
	if s.repo != nil {
		usage, err := s.repo.RoleplayOllamaModelUsage(ctx, installedModel)
		if err != nil {
			return fmt.Errorf("load roleplay model usage before removal: %w", err)
		}
		if usage.InUse() {
			return fmt.Errorf(
				"Ollama model %q is used by %d narrative and %d voice character settings",
				installedModel, usage.NarrativeCharacters, usage.VoiceCharacters,
			)
		}
	}
	if s.ollamaDownloads != nil {
		active, err := s.ollamaDownloads.ListActiveOllamaModelDownloads(ctx)
		if err != nil {
			return fmt.Errorf("load active model downloads before removal: %w", err)
		}
		for _, download := range active {
			if ollama.MatchesOllamaModel(download.Model, installedModel) {
				return fmt.Errorf("Ollama model %q has active download %s", installedModel, download.ID)
			}
		}
	}
	return nil
}
