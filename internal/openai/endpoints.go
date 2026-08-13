package openai

import (
	"fmt"
	"net/url"
	"strings"
)

func normalizeCompatibleBaseURL(baseURL string) (string, error) {
	value := strings.TrimSpace(baseURL)
	if value == "" {
		return "", fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("must be an absolute HTTP(S) URL, received %q", value)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf(
			"must not contain credentials, query parameters, or fragments, received %q", value,
		)
	}
	return strings.TrimRight(value, "/"), nil
}

func normalizeAzureBaseURLForStyle(baseURL, style string) string {
	value := strings.TrimSpace(baseURL)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	if normalizeAzureAIStyle(style, value) == "azure_v1" {
		parsed, err := url.Parse(value)
		if err == nil && (parsed.Path == "" || parsed.Path == "/") {
			parsed.Path = "/openai/v1"
			value = parsed.String()
		}
	}
	return strings.TrimRight(value, "/")
}

func normalizeAzureAIStyle(style, baseURL string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "foundry", "ai-foundry", "azure-foundry", "models", "model-inference":
		return "foundry"
	case "v1", "openai-v1", "azure-v1", "azure_v1", "azure_openai_v1":
		return "azure_v1"
	case "openai", "azure-openai", "azure_openai", "deployments", "deployment":
		return "azure_openai"
	}
	if strings.Contains(strings.ToLower(baseURL), "/openai/v1") {
		return "azure_v1"
	}
	if strings.Contains(strings.ToLower(baseURL), ".services.ai.azure.com") {
		return "foundry"
	}
	return "azure_openai"
}
