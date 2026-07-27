package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type Client struct {
	baseURL        string
	defaultModel   string
	embeddingModel string
	contextTokens  int
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
	Format   string        `json:"format,omitempty"`
	Think    *bool         `json:"think,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
}

type chatOptions struct {
	NumPredict  int      `json:"num_predict,omitempty"`
	NumCtx      int      `json:"num_ctx,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

const controlPlaneMaxOutputTokens = 2048

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// Chat runs a direct /api/chat call without creating ephemeral context modelfiles.
// Use for interactive scrum pilot chat where latency matters.
func (c *Client) Chat(ctx context.Context, model, system, user string) (string, error) {
	return c.chat(ctx, model, system, user, 0, c.contextTokens, "")
}

func (c *Client) chat(ctx context.Context, model, system, user string, maxOutputTokens, contextTokens int, responseFormat string) (string, error) {
	if responseFormat != "" && responseFormat != llm.ResponseFormatJSON {
		return "", fmt.Errorf("unsupported response format %q", responseFormat)
	}
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model is required")
	}
	messages := make([]chatMessage, 0, 2)
	if strings.TrimSpace(system) != "" {
		messages = append(messages, chatMessage{Role: "system", Content: strings.TrimSpace(system)})
	}
	user = strings.TrimSpace(user)
	if user == "" {
		user = "(empty)"
	}
	messages = append(messages, chatMessage{Role: "user", Content: user})

	request := chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	controlPlane := strings.Contains(system, "CONTROL_PLANE_COMMAND:") || responseFormat == llm.ResponseFormatJSON
	if controlPlane {
		if maxOutputTokens <= 0 {
			maxOutputTokens = controlPlaneMaxOutputTokens
		}
		thinkingDisabled := false
		request.Format = "json"
		request.Think = &thinkingDisabled
	}
	if contextTokens > 0 {
		if err := llm.ValidateInferenceBudget(contextTokens, maxOutputTokens, system, user); err != nil {
			return "", err
		}
		request.Options = &chatOptions{NumCtx: contextTokens}
	}
	if controlPlane {
		if request.Options == nil {
			request.Options = &chatOptions{}
		}
		zero := 0.0
		request.Options.NumPredict = maxOutputTokens
		request.Options.Temperature = &zero
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", c.wrapConnectivityError(err, "/api/chat")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama chat failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	out := strings.TrimSpace(parsed.Message.Content)
	if out == "" {
		return "", fmt.Errorf("ollama response missing message content")
	}
	return out, nil
}

type chatStreamChunk struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// ChatStream streams /api/chat tokens to onChunk and returns the full assembled reply.
func (c *Client) ChatStream(ctx context.Context, model, system, user string, onChunk func(string) error) (string, error) {
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model is required")
	}
	messages := make([]chatMessage, 0, 2)
	if strings.TrimSpace(system) != "" {
		messages = append(messages, chatMessage{Role: "system", Content: strings.TrimSpace(system)})
	}
	user = strings.TrimSpace(user)
	if user == "" {
		user = "(empty)"
	}
	messages = append(messages, chatMessage{Role: "user", Content: user})

	request := chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}
	if c.contextTokens > 0 {
		if err := llm.ValidateInferenceBudget(c.contextTokens, 0, system, user); err != nil {
			return "", err
		}
		request.Options = &chatOptions{NumCtx: c.contextTokens}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", c.wrapConnectivityError(err, "/api/chat")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama chat failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if text := chunk.Message.Content; text != "" {
			full.WriteString(text)
			if onChunk != nil {
				if err := onChunk(text); err != nil {
					return full.String(), err
				}
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), err
	}
	return strings.TrimSpace(full.String()), nil
}

type deleteModelRequest struct {
	Name  string `json:"name,omitempty"`
	Model string `json:"model,omitempty"`
}

type pullModelRequest struct {
	Name   string `json:"name,omitempty"`
	Model  string `json:"model,omitempty"`
	Stream bool   `json:"stream"`
}

type tagsResponse struct {
	Models []ModelInfo `json:"models"`
}

type ModelInfo struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	for _, item := range models {
		if name := strings.TrimSpace(item.Name); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.wrapConnectivityError(err, "/api/tags")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama tags failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var payload tagsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(payload.Models))
	for _, item := range payload.Models {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.Model)
		}
		if name == "" {
			continue
		}
		item.Name = name
		out = append(out, item)
	}
	return out, nil
}

func (c *Client) HasModel(ctx context.Context, model string) (bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}
	tags, err := c.ListTags(ctx)
	if err != nil {
		return false, err
	}
	for _, tag := range tags {
		if MatchesOllamaModel(model, tag) {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) EnsureModels(ctx context.Context, models []string) ([]string, error) {
	pulled := []string{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		ok, err := c.HasModel(ctx, model)
		if err != nil {
			return pulled, err
		}
		if ok {
			continue
		}
		if err := c.PullModel(ctx, model); err != nil {
			return pulled, fmt.Errorf("pull %s: %w", model, err)
		}
		pulled = append(pulled, model)
	}
	return pulled, nil
}

type embeddingsRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Input  string `json:"input,omitempty"`
}

type embeddingsResponse struct {
	Embedding  []float64   `json:"embedding"`
	Embeddings [][]float64 `json:"embeddings"`
}

const minimalGeneratePrompt = llm.MinimalGeneratePrompt

func derivePreparedModelPromptHint(fullPrompt string) string {
	return llm.DerivePreparedModelPromptHint(fullPrompt)
}

func extractPromptBlock(fullPrompt string, blockName string) string {
	return llm.ExtractPromptBlock(fullPrompt, blockName)
}

func truncatePromptHint(value string, maxChars int) string {
	return llm.TruncatePromptHint(value, maxChars)
}

func New(baseURL, defaultModel, embeddingModel string, timeout time.Duration, contextTokens int) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	dialTimeout := 5 * time.Second
	if timeout < dialTimeout {
		dialTimeout = timeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return &Client{
		baseURL:        strings.TrimSuffix(NormalizeBaseURL(baseURL), "/"),
		defaultModel:   defaultModel,
		embeddingModel: embeddingModel,
		contextTokens:  contextTokens,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	prepared, err := c.PrepareContextModel(ctx, model, prompt)
	if err != nil {
		return "", err
	}
	defer c.CleanupPreparedModel(prepared)

	return c.GeneratePrepared(ctx, prepared)
}

func (c *Client) PrepareContextModel(_ context.Context, model, prompt string) (llm.PreparedModel, error) {
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
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
		PromptHint:    llm.DerivePreparedModelPromptHint(prompt),
		Prompt:        prompt,
		ContextTokens: c.contextTokens,
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
	contextTokens := prepared.ContextTokens
	if contextTokens == 0 {
		contextTokens = c.contextTokens
	}
	return c.chat(ctx, model, system, promptHint, prepared.MaxOutputTokens, contextTokens, prepared.ResponseFormat)
}

func (c *Client) CleanupPreparedModel(llm.PreparedModel) {}

func (c *Client) PullModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	payload, err := json.Marshal(pullModelRequest{
		Name:   model,
		Model:  model,
		Stream: false,
	})
	if err != nil {
		return err
	}
	status, body, err := c.postJSON(ctx, "/api/pull", payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("ollama pull failed: status=%d body=%s", status, string(body))
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, c.wrapConnectivityError(err, endpoint)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func (c *Client) DeleteModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model name is required")
	}
	payload, err := json.Marshal(deleteModelRequest{Name: model, Model: model})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/delete", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.wrapConnectivityError(err, "/api/delete")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama delete failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) Embedding(ctx context.Context, content string) ([]float64, error) {
	payload, err := json.Marshal(embeddingsRequest{
		Model:  c.embeddingModel,
		Prompt: content,
		Input:  content,
	})
	if err != nil {
		return nil, err
	}

	endpoints := []string{"/api/embeddings", "/api/embed"}
	var lastErr error

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = c.wrapConnectivityError(err, endpoint)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("ollama embedding failed endpoint=%s status=%d body=%s", endpoint, resp.StatusCode, string(body))
			continue
		}

		var parsed embeddingsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err
			continue
		}

		if len(parsed.Embedding) > 0 {
			return parsed.Embedding, nil
		}
		if len(parsed.Embeddings) > 0 && len(parsed.Embeddings[0]) > 0 {
			return parsed.Embeddings[0], nil
		}

		lastErr = fmt.Errorf("embedding response missing vectors")
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("embedding request failed")
	}
	return nil, lastErr
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

func (c *Client) wrapConnectivityError(err error, endpoint string) error {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connect: connection refused") && strings.Contains(c.baseURL, "host.docker.internal") {
		return fmt.Errorf(
			"%w (cannot reach Ollama at %s%s; if Ollama runs on host, expose it to Docker with OLLAMA_HOST=0.0.0.0:11434 before starting Ollama, or run core locally with OLLAMA_BASE_URL=http://localhost:11434)",
			err,
			c.baseURL,
			endpoint,
		)
	}

	return err
}
