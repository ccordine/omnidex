package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	minOllamaProbeTimeout = 10 * time.Second
	maxOllamaProbeTimeout = 15 * time.Second
	ollamaProbeAttempts   = 2
	ollamaProbeRetryDelay = 400 * time.Millisecond
)

type webSearchRuntimeStatus struct {
	Enabled           bool                   `json:"enabled"`
	Providers         []string               `json:"providers,omitempty"`
	ReachableProvider bool                   `json:"reachable_provider"`
	LastProviderError string                 `json:"last_provider_error,omitempty"`
	Probes            []webSearchProbeStatus `json:"probes,omitempty"`
}

type webSearchProbeStatus struct {
	Provider   string `json:"provider"`
	TargetURL  string `json:"target_url,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) ollamaProbeTimeout() time.Duration {
	timeout := minOllamaProbeTimeout
	if s.requestTimeout > 0 && s.requestTimeout < timeout {
		timeout = s.requestTimeout
	}
	if timeout > maxOllamaProbeTimeout {
		timeout = maxOllamaProbeTimeout
	}
	return timeout
}

func (s *Server) ollamaReachabilityBudget() time.Duration {
	attempts := time.Duration(ollamaProbeAttempts)
	return s.ollamaProbeTimeout()*attempts + ollamaProbeRetryDelay*(attempts-1) + 500*time.Millisecond
}

func (s *Server) handleResearchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.ollamaReachabilityBudget())
	defer cancel()
	status := s.collectResearchStatus(ctx)
	status.HTML = renderResearchStatusHTML(status)
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) collectWebSearchStatus(ctx context.Context) webSearchRuntimeStatus {
	status := webSearchRuntimeStatus{
		Enabled:   s.webSearchEnabled,
		Providers: configuredWebSearchProviders(s.webSearchProviders),
	}
	if !status.Enabled {
		return status
	}
	timeout := s.webSearchTimeout
	if timeout <= 0 || timeout > maxOllamaProbeTimeout {
		timeout = maxOllamaProbeTimeout
	}
	for _, provider := range status.Providers {
		target := webSearchProbeURL(provider)
		probe := webSearchProbeStatus{Provider: provider, TargetURL: target}
		if target == "" {
			probe.Error = "no probe URL mapping"
			status.LastProviderError = probe.Error
			status.Probes = append(status.Probes, probe)
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		code, err := probeHTTP(probeCtx, target)
		cancel()
		if err != nil {
			probe.Error = err.Error()
			status.LastProviderError = fmt.Sprintf("%s: %s", provider, err.Error())
		} else {
			probe.StatusCode = code
			probe.Reachable = true
			status.ReachableProvider = true
		}
		status.Probes = append(status.Probes, probe)
	}
	return status
}

func (s *Server) probeOllamaTags(ctx context.Context) ([]string, error) {
	client := s.ollamaClientWithTimeout(s.ollamaProbeTimeout())
	var lastErr error
	for attempt := 0; attempt < ollamaProbeAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, ollamaProbeRetryDelay); err != nil {
				return nil, err
			}
			s.refreshOllamaEndpoint(ctx)
			client = s.ollamaClientWithTimeout(s.ollamaProbeTimeout())
		}
		models, err := client.ListTags(ctx)
		if err == nil {
			return models, nil
		}
		lastErr = err
		if isOllamaConnectivityError(err) {
			s.refreshOllamaEndpoint(ctx)
			client = s.ollamaClientWithTimeout(s.ollamaProbeTimeout())
		}
	}
	return nil, lastErr
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func configuredWebSearchProviders(values []string) []string {
	if len(values) == 0 {
		return []string{"duckduckgo", "google", "reddit"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		provider := strings.ToLower(strings.TrimSpace(value))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	if len(out) == 0 {
		return []string{"duckduckgo", "google", "reddit"}
	}
	return out
}

func webSearchProbeURL(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "duckduckgo":
		return "https://duckduckgo.com"
	case "google":
		return "https://www.google.com"
	case "reddit":
		return "https://www.reddit.com"
	case "yahoo":
		return "https://search.yahoo.com"
	default:
		if strings.Contains(provider, ".") {
			return "https://" + strings.TrimRight(provider, "/")
		}
		return ""
	}
}

func probeHTTP(ctx context.Context, endpoint string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "omnidex-status/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 256)
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("status=%d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func normalizeURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	parsed.Host = normalizeURLHost(parsed.Host)
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeURLHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if hostname, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(strings.TrimSuffix(hostname, "."), port)
	}
	return strings.TrimSuffix(host, ".")
}

func truncateStatusText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
