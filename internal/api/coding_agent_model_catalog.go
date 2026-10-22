package api

import (
	"context"

	"github.com/gryph/omnidex/internal/codex"
)

func (server *Server) codingAgentModelCatalog(ctx context.Context, native []codex.Model) ([]codingAgentModel, error) {
	catalog := make([]codingAgentModel, 0, len(native))
	for _, model := range native {
		catalog = append(catalog, codingAgentModel{Provider: "codex", Model: model})
	}
	if server.ollamaEndpoint() == "" {
		return catalog, nil
	}
	for offset := 0; ; offset += 100 {
		page, err := server.listInstalledOllamaModels(ctx, 100, offset)
		if err != nil {
			return nil, err
		}
		for _, model := range page.Models {
			catalog = append(catalog, codingAgentModel{Provider: "ollama", Model: codex.Model{
				ID: model.Name, DisplayName: model.Name + " (local)", Efforts: []codex.EffortOption{}, ServiceTiers: []codex.ServiceTier{},
			}})
		}

		if !page.HasMore {
			return catalog, nil
		}
	}
}
