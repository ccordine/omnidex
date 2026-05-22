package anthropic

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

	"github.com/gryph/omnidex/internal/llm"
)

const defaultVersion = "2023-06-01"

type Client struct {
	baseURL      string
	apiKey       string
	defaultModel string
	version      string
	maxTokens    int
	httpClient   *http.Client
}

type messageRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []messagePayload `json:"messages"`
}

type messagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messageResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func New(baseURL, apiKey, defaultModel, version string, maxTokens int, timeout time.Duration) *Client {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	version = strings.TrimSpace(version)
	if version == "" {
		version = defaultVersion
	}
	return &Client{
		baseURL:      normalizeBaseURL(baseURL),
		apiKey:       strings.TrimSpace(apiKey),
		defaultModel: strings.TrimSpace(defaultModel),
		version:      version,
		maxTokens:    maxTokens,
		httpClient:   &http.Client{Timeout: timeout},
	}
}

func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	prepared, err := c.PrepareContextModel(ctx, model, prompt)
	if err != nil {
		return "", err
	}
	return c.GeneratePrepared(ctx, prepared)
}

func (c *Client) PrepareContextModel(_ context.Context, model, prompt string) (llm.PreparedModel, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = c.defaultModel
	}
	if model == "" {
		return llm.PreparedModel{}, fmt.Errorf("model is required")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "(empty prompt)"
	}
	return llm.PreparedModel{
		BaseModel:    model,
		ContextModel: model,
		PromptHint:   llm.DerivePreparedModelPromptHint(prompt),
		Prompt:       prompt,
	}, nil
}

func (c *Client) GeneratePrepared(ctx context.Context, prepared llm.PreparedModel) (string, error) {
	model := strings.TrimSpace(prepared.ContextModel)
	if model == "" {
		model = strings.TrimSpace(prepared.BaseModel)
	}
	if model == "" {
		model = c.defaultModel
	}
	if model == "" {
		return "", fmt.Errorf("model is required")
	}

	system := strings.TrimSpace(prepared.Prompt)
	if system == "" {
		system = "(empty prompt)"
	}
	promptHint := strings.TrimSpace(prepared.PromptHint)
	if promptHint == "" {
		promptHint = llm.MinimalGeneratePrompt
	}

	var parsed messageResponse
	if err := c.doJSON(ctx, "/messages", messageRequest{
		Model:     model,
		MaxTokens: c.maxTokens,
		System:    system,
		Messages:  []messagePayload{{Role: "user", Content: promptHint}},
	}, &parsed); err != nil {
		return "", err
	}

	parts := make([]string, 0, len(parsed.Content))
	for _, item := range parsed.Content {
		if strings.EqualFold(item.Type, "text") && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		return "", fmt.Errorf("anthropic response missing text content")
	}
	return out, nil
}

func (c *Client) CleanupPreparedModel(_ llm.PreparedModel) {}

func (c *Client) Embedding(context.Context, string) ([]float64, error) {
	return nil, fmt.Errorf("anthropic does not provide embeddings; configure EMBEDDING_PROVIDER=ollama|openai|google|huggingface")
}

func (c *Client) SuggestTags(ctx context.Context, content string, maxTags int) ([]string, error) {
	return c.SuggestTagsWithModel(ctx, c.defaultModel, content, maxTags)
}

func (c *Client) SuggestTagsWithModel(ctx context.Context, model, content string, maxTags int) ([]string, error) {
	if maxTags <= 0 {
		maxTags = 8
	}
	prompt := strings.Join([]string{
		"Extract compact relevance tags for retrieval.",
		"Operational mode: text analysis only. Do not roleplay or invent fictional context.",
		"Return only comma-separated lowercase tags.",
		fmt.Sprintf("Maximum tags: %d.", maxTags),
		"Do not include punctuation-only tokens.",
		"Text:",
		content,
	}, "\n")
	result, err := c.Generate(ctx, model, prompt)
	if err != nil {
		return nil, err
	}
	return llm.ParseSuggestedTags(result, content, maxTags), nil
}

func (c *Client) doJSON(ctx context.Context, path string, payload any, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("anthropic request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}

func normalizeBaseURL(baseURL string) string {
	value := strings.TrimSpace(baseURL)
	if value == "" {
		value = "https://api.anthropic.com/v1"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err == nil && (parsed.Path == "" || parsed.Path == "/") {
		parsed.Path = "/v1"
		value = parsed.String()
	}
	return strings.TrimRight(value, "/")
}
