package api

import (
	"fmt"
	"net/http"
	"strings"
)

type webSearchRuntimeStatus struct {
	Enabled   bool     `json:"enabled"`
	Providers []string `json:"providers,omitempty"`
}

func (s *Server) handleResearchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.collectResearchStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fragment := renderResearchStatusHTML(status)
	status.HTML.Bundle = renderRecyclrTemplateHTML("research-status-output", fragment, "innerHTML")
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) collectWebSearchStatus() (webSearchRuntimeStatus, error) {
	providers, err := configuredWebSearchProviders(s.webSearchProviders)
	if err != nil {
		return webSearchRuntimeStatus{}, err
	}
	return webSearchRuntimeStatus{Enabled: len(providers) > 0, Providers: providers}, nil
}

func configuredWebSearchProviders(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		provider := strings.ToLower(strings.TrimSpace(value))
		if provider == "" {
			continue
		}
		switch provider {
		case "brave", "duckduckgo", "google", "reddit", "yahoo":
		default:
			return nil, fmt.Errorf("web search status rejects unregistered provider %q", value)
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	return out, nil
}

func normalizeURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func truncateStatusText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
