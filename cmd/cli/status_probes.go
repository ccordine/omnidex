package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func queryCoreHealth(ctx context.Context, coreURL string) (string, string, error) {
	endpoint := strings.TrimRight(coreURL, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("status=%d body=%s", resp.StatusCode, trimStatusBody(body))
	}

	var payload struct {
		Status string    `json:"status"`
		Time   time.Time `json:"time"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}

	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "unknown"
	}
	ts := ""
	if !payload.Time.IsZero() {
		ts = payload.Time.UTC().Format(time.RFC3339)
	}
	return status, ts, nil
}

func queryOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, trimStatusBody(body))
	}

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(payload.Models))
	for _, modelValue := range payload.Models {
		name := strings.TrimSpace(modelValue.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func probeHTTPReachability(ctx context.Context, endpoint string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "omni-status/1.0")

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

func parseStatusProviders(override string) []string {
	if strings.TrimSpace(override) != "" {
		values := parseStatusCSV(override)
		if len(values) > 0 {
			return values
		}
	}
	if values := parseStatusCSV(os.Getenv("WEB_SEARCH_PROVIDERS")); len(values) > 0 {
		return values
	}
	return []string{"duckduckgo", "google", "reddit"}
}

func parseStatusCSV(value string) []string {
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToLower(strings.TrimSpace(part))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func statusProviderProbeURL(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return strings.TrimRight(p, "/")
	}

	switch p {
	case "google":
		return "https://www.google.com"
	case "yahoo":
		return "https://search.yahoo.com"
	case "reddit":
		return "https://www.reddit.com"
	case "duckduckgo":
		return "https://duckduckgo.com"
	case "bing":
		return "https://www.bing.com"
	}

	if strings.Contains(p, ".") {
		return "https://" + strings.TrimRight(p, "/")
	}
	return ""
}

func statusEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on", "enabled":
		return true
	case "0", "false", "no", "n", "off", "disabled":
		return false
	default:
		return fallback
	}
}
