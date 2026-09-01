package api

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) ollamaEndpoint() string {
	return s.providerConfig.OllamaBaseURL
}

func (s *Server) ollamaClientWithTimeout(timeout time.Duration) *ollama.Client {
	return ollama.New(s.ollamaEndpoint(), "", "", timeout)
}

func (s *Server) hasInstalledOllamaModel(ctx context.Context, model string) (bool, error) {
	return s.ollamaClientWithTimeout(30*time.Second).HasModel(ctx, model)
}

func (s *Server) listInstalledOllamaModels(
	ctx context.Context,
	limit, offset int,
) (ollama.ModelPage, error) {
	return s.ollamaClientWithTimeout(30*time.Second).ListModelPage(ctx, limit, offset)
}
