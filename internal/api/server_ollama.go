package api

import (
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) ollamaEndpoint() string {
	s.ollamaURLMu.RLock()
	defer s.ollamaURLMu.RUnlock()
	return normalizeURL(s.ollamaBaseURL)
}

func (s *Server) ollamaClientWithTimeout(timeout time.Duration) *ollama.Client {
	return ollama.New(s.ollamaEndpoint(), "", "", timeout, s.providerConfig.InferenceContextTokens)
}
