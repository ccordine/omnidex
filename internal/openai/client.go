package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxProviderResponseBytes = 16 << 20
	maxProviderErrorBytes    = 4 << 10
)

type Client struct {
	baseURL        string
	apiKey         string
	embeddingModel string
	organization   string
	project        string
	providerName   string
	apiKeyName     string
	apiStyle       string
	apiVersion     string
	httpClient     *http.Client
}

type embeddingsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingsResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func NewEmbedding(
	baseURL string,
	apiKey string,
	embeddingModel string,
	organization string,
	project string,
	timeout time.Duration,
) (*Client, error) {
	return NewCompatibleEmbedding(
		"openai", "OPENAI_API_KEY", baseURL, apiKey, embeddingModel,
		organization, project, timeout,
	)
}

func NewCompatibleEmbedding(
	providerName string,
	apiKeyName string,
	baseURL string,
	apiKey string,
	embeddingModel string,
	organization string,
	project string,
	timeout time.Duration,
) (*Client, error) {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return nil, fmt.Errorf("provider name is required")
	}
	apiKeyName = strings.TrimSpace(apiKeyName)
	if apiKeyName == "" {
		return nil, fmt.Errorf("API key environment name is required for provider %s", providerName)
	}
	normalizedBaseURL, err := normalizeCompatibleBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%s base URL: %w", providerName, err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s is required for provider %s", apiKeyName, providerName)
	}
	embeddingModel = strings.TrimSpace(embeddingModel)
	if embeddingModel == "" {
		return nil, fmt.Errorf("embedding model is required for provider %s", providerName)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("request timeout must be positive for provider %s", providerName)
	}
	return &Client{
		baseURL: normalizedBaseURL, apiKey: apiKey, embeddingModel: embeddingModel,
		organization: strings.TrimSpace(organization), project: strings.TrimSpace(project),
		providerName: providerName, apiKeyName: apiKeyName, apiStyle: "openai",
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func NewAzureAIEmbedding(
	baseURL string,
	apiKey string,
	embeddingModel string,
	apiVersion string,
	apiStyle string,
	timeout time.Duration,
) (*Client, error) {
	apiStyle = normalizeAzureAIStyle(apiStyle, baseURL)
	if strings.TrimSpace(apiVersion) == "" {
		switch apiStyle {
		case "foundry":
			apiVersion = "2024-05-01-preview"
		case "azure_v1":
			apiVersion = ""
		default:
			apiVersion = "2024-10-21"
		}
	}
	baseURL = normalizeAzureBaseURLForStyle(baseURL, apiStyle)
	if _, err := normalizeCompatibleBaseURL(baseURL); err != nil {
		return nil, fmt.Errorf("azure base URL: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("AZURE_AI_API_KEY is required for provider azure")
	}
	embeddingModel = strings.TrimSpace(embeddingModel)
	if embeddingModel == "" {
		return nil, fmt.Errorf("embedding model is required for provider azure")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("request timeout must be positive for provider azure")
	}
	return &Client{
		baseURL: baseURL, apiKey: apiKey, embeddingModel: embeddingModel,
		providerName: "azure", apiKeyName: "AZURE_AI_API_KEY",
		apiStyle: apiStyle, apiVersion: strings.TrimSpace(apiVersion),
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Embedding(ctx context.Context, content string) ([]float64, error) {
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}

	var response embeddingsResponse
	if err := c.doJSON(
		ctx, http.MethodPost, c.embeddingsPath(model),
		embeddingsRequest{Model: model, Input: content}, &response,
	); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response missing vectors")
	}
	return response.Data[0].Embedding, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	path string,
	payload any,
	out any,
) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("%s is required", c.apiKeyName)
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(
		ctx, method, strings.TrimRight(c.baseURL, "/")+path, body,
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiStyle == "azure_openai" || c.apiStyle == "foundry" {
		req.Header.Set("api-key", c.apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.organization != "" {
		req.Header.Set("OpenAI-Organization", c.organization)
	}
	if c.project != "" {
		req.Header.Set("OpenAI-Project", c.project)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", c.providerName, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read %s response: %w", c.providerName, err)
	}
	if len(data) > maxProviderResponseBytes {
		return fmt.Errorf("%s response exceeded %d bytes", c.providerName, maxProviderResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.providerError(resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s response: %w", c.providerName, err)
		}
	}
	return nil
}

func (c *Client) providerError(status int, data []byte) error {
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && strings.TrimSpace(body.Error.Message) != "" {
		message := strings.TrimSpace(body.Error.Message)
		if kind := strings.TrimSpace(body.Error.Type); kind != "" {
			message = fmt.Sprintf("%s (%s)", message, kind)
		}
		return fmt.Errorf("%s request failed: %s", c.providerName, message)
	}
	message := strings.TrimSpace(string(data))
	if len(message) > maxProviderErrorBytes {
		message = message[:maxProviderErrorBytes] + "...[truncated]"
	}
	return fmt.Errorf("%s request failed: status=%d body=%s", c.providerName, status, message)
}

func (c *Client) embeddingsPath(model string) string {
	switch c.apiStyle {
	case "azure_openai":
		return c.withAPIVersion(
			"/openai/deployments/" + url.PathEscape(strings.TrimSpace(model)) + "/embeddings",
		)
	case "foundry":
		return c.withAPIVersion("/models/embeddings")
	default:
		return "/embeddings"
	}
}

func (c *Client) withAPIVersion(path string) string {
	version := strings.TrimSpace(c.apiVersion)
	if version == "" {
		return path
	}
	return path + "?api-version=" + url.QueryEscape(version)
}
