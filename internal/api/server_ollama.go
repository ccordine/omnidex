package api

import (
	"context"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) ollamaEndpoint() string {
	return s.providerConfig.OllamaBaseURL
}

func (s *Server) ollamaClient() *ollama.Client {
	return ollama.New(s.ollamaEndpoint(), "", "")
}

func (s *Server) hasInstalledOllamaModel(ctx context.Context, model string) (bool, error) {
	return s.ollamaClient().HasModel(ctx, model)
}

func (s *Server) listInstalledOllamaModels(
	ctx context.Context,
	limit, offset int,
) (ollama.ModelPage, error) {
	return s.ollamaClient().ListModelPage(ctx, limit, offset)
}
