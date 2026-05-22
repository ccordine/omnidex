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

	"github.com/gryph/omnidex/internal/llm"
)

type Client struct {
	baseURL        string
	apiKey         string
	defaultModel   string
	embeddingModel string
	organization   string
	project        string
	providerName   string
	apiKeyName     string
	apiStyle       string
	apiVersion     string
	httpClient     *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
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

func New(baseURL, apiKey, defaultModel, embeddingModel, organization, project string, timeout time.Duration) *Client {
	return NewCompatible("openai", "OPENAI_API_KEY", baseURL, apiKey, defaultModel, embeddingModel, organization, project, timeout)
}

func NewCompatible(providerName, apiKeyName, baseURL, apiKey, defaultModel, embeddingModel, organization, project string, timeout time.Duration) *Client {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		providerName = "openai"
	}
	apiKeyName = strings.TrimSpace(apiKeyName)
	if apiKeyName == "" {
		apiKeyName = "OPENAI_API_KEY"
	}
	return &Client{
		baseURL:        normalizeBaseURL(baseURL),
		apiKey:         strings.TrimSpace(apiKey),
		defaultModel:   strings.TrimSpace(defaultModel),
		embeddingModel: strings.TrimSpace(embeddingModel),
		organization:   strings.TrimSpace(organization),
		project:        strings.TrimSpace(project),
		providerName:   providerName,
		apiKeyName:     apiKeyName,
		apiStyle:       "openai",
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func NewAzureAI(baseURL, apiKey, defaultModel, embeddingModel, apiVersion, apiStyle string, timeout time.Duration) *Client {
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
	return &Client{
		baseURL:        normalizeAzureBaseURLForStyle(baseURL, apiStyle),
		apiKey:         strings.TrimSpace(apiKey),
		defaultModel:   strings.TrimSpace(defaultModel),
		embeddingModel: strings.TrimSpace(embeddingModel),
		providerName:   "azure",
		apiKeyName:     "AZURE_AI_API_KEY",
		apiStyle:       apiStyle,
		apiVersion:     strings.TrimSpace(apiVersion),
		httpClient: &http.Client{
			Timeout: timeout,
		},
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
		BaseModel:     model,
		ContextModel:  model,
		ModelfilePath: "",
		PromptHint:    llm.DerivePreparedModelPromptHint(prompt),
		Prompt:        prompt,
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

	prompt := strings.TrimSpace(prepared.Prompt)
	if prompt == "" {
		prompt = "(empty prompt)"
	}
	promptHint := strings.TrimSpace(prepared.PromptHint)
	if promptHint == "" {
		promptHint = llm.MinimalGeneratePrompt
	}

	var resp chatCompletionResponse
	err := c.doJSON(ctx, http.MethodPost, c.chatCompletionsPath(model), chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: promptHint},
		},
		Stream: false,
	}, &resp)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s response missing choices", c.providerName)
	}
	text := strings.TrimSpace(messageContentAsString(resp.Choices[0].Message.Content))
	if text == "" {
		return "", fmt.Errorf("%s response missing message content", c.providerName)
	}
	return text, nil
}

func (c *Client) CleanupPreparedModel(_ llm.PreparedModel) {}

func (c *Client) Embedding(ctx context.Context, content string) ([]float64, error) {
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}

	var resp embeddingsResponse
	err := c.doJSON(ctx, http.MethodPost, c.embeddingsPath(model), embeddingsRequest{
		Model: model,
		Input: content,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response missing vectors")
	}
	return resp.Data[0].Embedding, nil
}

func (c *Client) SuggestTags(ctx context.Context, content string, maxTags int) ([]string, error) {
	return c.SuggestTagsWithModel(ctx, c.defaultModel, content, maxTags)
}

func (c *Client) SuggestTagsWithModel(ctx context.Context, model, content string, maxTags int) ([]string, error) {
	if maxTags <= 0 {
		maxTags = 8
	}
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
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

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
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

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, body)
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
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
			msg := strings.TrimSpace(errBody.Error.Message)
			if strings.TrimSpace(errBody.Error.Type) != "" {
				msg = fmt.Sprintf("%s (%s)", msg, strings.TrimSpace(errBody.Error.Type))
			}
			return fmt.Errorf("%s request failed: %s", c.providerName, msg)
		}
		return fmt.Errorf("%s request failed: status=%d body=%s", c.providerName, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) chatCompletionsPath(model string) string {
	switch c.apiStyle {
	case "azure_openai":
		return c.withAPIVersion("/openai/deployments/" + url.PathEscape(strings.TrimSpace(model)) + "/chat/completions")
	case "foundry":
		return c.withAPIVersion("/models/chat/completions")
	case "azure_v1":
		return "/chat/completions"
	default:
		return "/chat/completions"
	}
}

func (c *Client) embeddingsPath(model string) string {
	switch c.apiStyle {
	case "azure_openai":
		return c.withAPIVersion("/openai/deployments/" + url.PathEscape(strings.TrimSpace(model)) + "/embeddings")
	case "foundry":
		return c.withAPIVersion("/models/embeddings")
	case "azure_v1":
		return "/embeddings"
	default:
		return "/embeddings"
	}
}

func (c *Client) withAPIVersion(path string) string {
	version := strings.TrimSpace(c.apiVersion)
	if version == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "api-version=" + url.QueryEscape(version)
}

func normalizeBaseURL(baseURL string) string {
	value := strings.TrimSpace(baseURL)
	if value == "" {
		value = "https://api.openai.com/v1"
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

func normalizeAzureBaseURL(baseURL string) string {
	return normalizeAzureBaseURLForStyle(baseURL, "")
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
