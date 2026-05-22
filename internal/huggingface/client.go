package huggingface

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

type Client struct {
	baseURL        string
	apiKey         string
	defaultModel   string
	embeddingModel string
	httpClient     *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func New(baseURL, apiKey, defaultModel, embeddingModel string, timeout time.Duration) *Client {
	return &Client{
		baseURL:        normalizeBaseURL(baseURL),
		apiKey:         strings.TrimSpace(apiKey),
		defaultModel:   strings.TrimSpace(defaultModel),
		embeddingModel: strings.TrimSpace(embeddingModel),
		httpClient:     &http.Client{Timeout: timeout},
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
	model := firstNonEmpty(prepared.ContextModel, prepared.BaseModel, c.defaultModel)
	if strings.TrimSpace(model) == "" {
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

	var parsed chatResponse
	if err := c.doRouterJSON(ctx, "/chat/completions", chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: promptHint},
		},
		Stream: false,
	}, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("huggingface response missing choices")
	}
	out := strings.TrimSpace(messageContentAsString(parsed.Choices[0].Message.Content))
	if out == "" {
		return "", fmt.Errorf("huggingface response missing message content")
	}
	return out, nil
}

func (c *Client) CleanupPreparedModel(_ llm.PreparedModel) {}

func (c *Client) Embedding(ctx context.Context, input string) ([]float64, error) {
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("HUGGINGFACE_EMBEDDING_MODEL is required")
	}
	path := "/hf-inference/models/" + escapeModelID(model) + "/pipeline/feature-extraction"
	var parsed any
	if err := c.doRawJSON(ctx, path, map[string]any{"inputs": input}, &parsed); err != nil {
		return nil, err
	}
	vector := extractEmbeddingVector(parsed)
	if len(vector) == 0 {
		return nil, fmt.Errorf("huggingface embedding response missing vector")
	}
	return vector, nil
}

func (c *Client) SuggestTags(ctx context.Context, content string, maxTags int) ([]string, error) {
	return c.SuggestTagsWithModel(ctx, c.defaultModel, content, maxTags)
}

func (c *Client) SuggestTagsWithModel(ctx context.Context, model, text string, maxTags int) ([]string, error) {
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
		text,
	}, "\n")
	result, err := c.Generate(ctx, model, prompt)
	if err != nil {
		return nil, err
	}
	return llm.ParseSuggestedTags(result, text, maxTags), nil
}

func (c *Client) doRouterJSON(ctx context.Context, path string, payload any, out any) error {
	return c.doRawJSON(ctx, "/v1"+path, payload, out)
}

func (c *Client) doRawJSON(ctx context.Context, path string, payload any, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("HUGGINGFACE_API_KEY or HF_TOKEN is required")
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
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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
		return fmt.Errorf("huggingface request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
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
		value = "https://router.huggingface.co"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err == nil && strings.TrimRight(parsed.Path, "/") == "/v1" {
		parsed.Path = ""
		value = parsed.String()
	}
	return strings.TrimRight(value, "/")
}

func escapeModelID(model string) string {
	return strings.ReplaceAll(url.PathEscape(strings.TrimSpace(model)), "%2F", "/")
}

func extractEmbeddingVector(value any) []float64 {
	switch typed := value.(type) {
	case []any:
		if len(typed) == 0 {
			return nil
		}
		if vector, ok := numberArray(typed); ok {
			return vector
		}
		return extractEmbeddingVector(typed[0])
	case map[string]any:
		for _, key := range []string{"embedding", "embeddings", "data"} {
			if inner, ok := typed[key]; ok {
				if vector := extractEmbeddingVector(inner); len(vector) > 0 {
					return vector
				}
			}
		}
	}
	return nil
}

func numberArray(values []any) ([]float64, bool) {
	out := make([]float64, 0, len(values))
	for _, value := range values {
		number, ok := value.(float64)
		if !ok {
			return nil, false
		}
		out = append(out, number)
	}
	return out, len(out) > 0
}

func messageContentAsString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text := strings.TrimSpace(fmt.Sprintf("%v", entry["text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprintf("%v", typed)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
