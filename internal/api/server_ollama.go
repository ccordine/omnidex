package api

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) ollamaEndpoint() string {
	s.ollamaURLMu.RLock()
	defer s.ollamaURLMu.RUnlock()
	return normalizeURL(firstNonEmpty(s.ollamaBaseURL, "http://host.docker.internal:11434"))
}

func (s *Server) setOllamaEndpoint(endpoint string) {
	endpoint = ollama.NormalizeBaseURL(endpoint)
	if endpoint == "" {
		return
	}
	s.ollamaURLMu.Lock()
	s.ollamaBaseURL = endpoint
	s.ollamaURLMu.Unlock()
}

func (s *Server) refreshOllamaEndpoint(ctx context.Context) string {
	current := s.ollamaEndpoint()
	resolved, err := ollama.ResolveReachableBaseURL(ctx, current, 4*time.Second)
	if err != nil {
		return current
	}
	if resolved != current {
		log.Printf("ollama endpoint switched %s -> %s", current, resolved)
		s.setOllamaEndpoint(resolved)
	}
	return resolved
}

func (s *Server) ollamaClientWithTimeout(timeout time.Duration) *ollama.Client {
	return ollama.New(s.ollamaEndpoint(), s.ollamaDefaultModel, "", timeout)
}

func isOllamaConnectivityError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "/api/tags") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "cannot reach ollama")
}

func webChatJobMetadata(payload map[string]any) bool {
	return metadataString(payload, "source") == "omni-web-chat"
}
