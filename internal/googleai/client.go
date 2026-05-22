package googleai

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

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type generateRequest struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
}

type generateResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
}

type embedRequest struct {
	Model   string  `json:"model,omitempty"`
	Content content `json:"content"`
}

type embedResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
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

	var parsed generateResponse
	if err := c.doJSON(ctx, modelPath(model)+":generateContent", generateRequest{
		SystemInstruction: &content{Parts: []part{{Text: system}}},
		Contents:          []content{{Role: "user", Parts: []part{{Text: promptHint}}}},
	}, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 {
		return "", fmt.Errorf("google response missing candidates")
	}
	parts := make([]string, 0, len(parsed.Candidates[0].Content.Parts))
	for _, item := range parsed.Candidates[0].Content.Parts {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		return "", fmt.Errorf("google response missing text content")
	}
	return out, nil
}

func (c *Client) CleanupPreparedModel(_ llm.PreparedModel) {}

func (c *Client) Embedding(ctx context.Context, input string) ([]float64, error) {
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("GOOGLE_EMBEDDING_MODEL is required")
	}
	var parsed embedResponse
	if err := c.doJSON(ctx, modelPath(model)+":embedContent", embedRequest{
		Model:   modelPath(model),
		Content: content{Parts: []part{{Text: input}}},
	}, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Embedding.Values) == 0 {
		return nil, fmt.Errorf("google embedding response missing values")
	}
	return parsed.Embedding.Values, nil
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

func (c *Client) doJSON(ctx context.Context, path string, payload any, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("GOOGLE_API_KEY is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	query := reqURL.Query()
	query.Set("key", c.apiKey)
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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
		return fmt.Errorf("google request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
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
		value = "https://generativelanguage.googleapis.com/v1beta"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	return strings.TrimRight(value, "/")
}

func modelPath(model string) string {
	model = strings.TrimSpace(model)
	model = strings.TrimPrefix(model, "/")
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
