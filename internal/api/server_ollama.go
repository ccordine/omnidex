package api

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/ollamacatalog"
	"github.com/gryph/omnidex/internal/queue"
)

type OllamaModelAuthority interface {
	HasModel(context.Context, string) (bool, error)
	ListModelPage(context.Context, int, int) (ollama.ModelPage, error)
}

type OllamaModelLifecycleAuthority interface {
	PullModelProgress(context.Context, string, func(ollama.PullProgress) error) error
	HasModel(context.Context, string) (bool, error)
	DeleteModel(context.Context, string) error
}

type OllamaDownloadStore interface {
	CreateOllamaModelDownload(context.Context, string) (queue.OllamaModelDownload, error)
	StartOllamaModelDownload(context.Context, string) (queue.OllamaModelDownload, error)
	RecordOllamaModelDownloadProgress(context.Context, string, ollama.PullProgress) (queue.OllamaModelDownload, error)
	CompleteOllamaModelDownload(context.Context, string) (queue.OllamaModelDownload, error)
	FailOllamaModelDownload(context.Context, string, string) (queue.OllamaModelDownload, error)
	ListOllamaModelDownloads(context.Context, int, int) (queue.OllamaModelDownloadPage, error)
	ListActiveOllamaModelDownloads(context.Context) ([]queue.OllamaModelDownload, error)
}

type OllamaCatalogAuthority interface {
	Search(context.Context, string, int) (ollamacatalog.Page, error)
}

func (s *Server) ollamaEndpoint() string {
	s.ollamaURLMu.RLock()
	defer s.ollamaURLMu.RUnlock()
	return s.ollamaBaseURL
}

func (s *Server) ollamaClientWithTimeout(timeout time.Duration) *ollama.Client {
	return ollama.New(s.ollamaEndpoint(), "", "", timeout, s.providerConfig.InferenceContextTokens)
}

func (s *Server) hasInstalledOllamaModel(ctx context.Context, model string) (bool, error) {
	if s.ollamaModelAuthority != nil {
		return s.ollamaModelAuthority.HasModel(ctx, model)
	}
	return s.ollamaClientWithTimeout(30*time.Second).HasModel(ctx, model)
}

func (s *Server) listInstalledOllamaModels(
	ctx context.Context,
	limit, offset int,
) (ollama.ModelPage, error) {
	if s.ollamaModelAuthority != nil {
		return s.ollamaModelAuthority.ListModelPage(ctx, limit, offset)
	}
	return s.ollamaClientWithTimeout(30*time.Second).ListModelPage(ctx, limit, offset)
}

func (s *Server) ollamaLifecycleClient() OllamaModelLifecycleAuthority {
	if s.ollamaModelLifecycle != nil {
		return s.ollamaModelLifecycle
	}
	return ollama.NewUnbounded(
		s.ollamaEndpoint(), "", "", s.providerConfig.InferenceContextTokens,
	)
}
