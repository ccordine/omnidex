package ollama

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type Client struct {
	baseURL        string
	defaultModel   string
	embeddingModel string
	httpClient     *http.Client
}

var contextModelCounter uint64

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
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
	Message chatMessage `json:"message"`
}

// Chat runs a direct /api/chat call without creating ephemeral context modelfiles.
// Use for interactive scrum pilot chat where latency matters.
func (c *Client) Chat(ctx context.Context, model, system, user string) (string, error) {
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

	payload, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	})
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
	return strings.TrimSpace(parsed.Message.Content), nil
}

type createModelRequest struct {
	Name      string `json:"name,omitempty"`
	Model     string `json:"model,omitempty"`
	From      string `json:"from,omitempty"`
	Modelfile string `json:"modelfile,omitempty"`
	Stream    bool   `json:"stream"`
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

func New(baseURL, defaultModel, embeddingModel string, timeout time.Duration) *Client {
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

func (c *Client) PrepareContextModel(ctx context.Context, model, prompt string) (llm.PreparedModel, error) {
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "(empty prompt)"
	}

	contextModel := buildContextModelName(model, prompt)
	modelfilePath, err := c.createContextModel(ctx, contextModel, model, prompt)
	if err != nil {
		return llm.PreparedModel{}, err
	}
	return llm.PreparedModel{
		BaseModel:     model,
		ContextModel:  contextModel,
		ModelfilePath: modelfilePath,
		PromptHint:    llm.DerivePreparedModelPromptHint(prompt),
		Prompt:        prompt,
	}, nil
}

func (c *Client) GeneratePrepared(ctx context.Context, prepared llm.PreparedModel) (string, error) {
	contextModel := strings.TrimSpace(prepared.ContextModel)
	if contextModel == "" {
		return "", fmt.Errorf("prepared model missing context model name")
	}
	promptHint := strings.TrimSpace(prepared.PromptHint)
	if promptHint == "" {
		promptHint = llm.MinimalGeneratePrompt
	}

	payload, err := json.Marshal(generateRequest{
		Model:  contextModel,
		Prompt: promptHint,
		Stream: false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", c.wrapConnectivityError(err, "/api/generate")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama generate failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed generateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	return strings.TrimSpace(parsed.Response), nil
}

func (c *Client) CleanupPreparedModel(prepared llm.PreparedModel) {
	c.bestEffortDeleteContextModel(prepared.ContextModel)
}

func (c *Client) createContextModel(ctx context.Context, contextModel, baseModel, prompt string) (string, error) {
	modelfile := buildContextModelfile(baseModel, prompt)
	modelfilePath, err := persistModelfile(contextModel, modelfile)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(createModelRequest{
		Name:      contextModel,
		Model:     contextModel,
		From:      baseModel,
		Modelfile: modelfile,
		Stream:    false,
	})
	if err != nil {
		return "", err
	}

	status, body, err := c.postJSON(ctx, "/api/create", payload)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		if isOllamaMissingModelResponse(string(body)) {
			if pullErr := c.PullModel(ctx, baseModel); pullErr != nil {
				return "", fmt.Errorf("ollama create failed because model %q is missing and pull failed: %w", baseModel, pullErr)
			}
			status, body, err = c.postJSON(ctx, "/api/create", payload)
			if err != nil {
				return "", err
			}
		}
		if status < 200 || status >= 300 {
			return "", fmt.Errorf("ollama create failed: status=%d body=%s", status, string(body))
		}
	}
	return modelfilePath, nil
}

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

func isOllamaMissingModelResponse(body string) bool {
	lower := strings.ToLower(strings.TrimSpace(body))
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "model") && strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "pull model") ||
		strings.Contains(lower, "try pulling")
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

func (c *Client) bestEffortDeleteContextModel(contextModel string) {
	contextModel = strings.TrimSpace(contextModel)
	if contextModel == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = c.DeleteModel(cleanupCtx, contextModel)
}

func buildContextModelName(baseModel string, prompt string) string {
	base := sanitizeModelNameComponent(baseModel)
	if base == "" {
		base = "model"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	hash := sha1.Sum([]byte(prompt))
	seq := atomic.AddUint64(&contextModelCounter, 1)
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{
		"ctx",
		base,
		fmt.Sprintf("%x", hash[:4]),
		strconv.FormatUint(seq, 10),
	}, "-")))
}

func sanitizeModelNameComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, ch := range value {
		isAlphaNum := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if isAlphaNum {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if lastDash {
			continue
		}
		b.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(b.String(), "-")
}

func buildContextModelfile(baseModel string, prompt string) string {
	baseModel = strings.TrimSpace(baseModel)
	if baseModel == "" {
		baseModel = "llama3.2"
	}
	prompt = strings.ReplaceAll(strings.TrimSpace(prompt), `"""`, `\"\"\"`)
	return strings.Join([]string{
		"FROM " + baseModel,
		"SYSTEM \"\"\"",
		prompt,
		"\"\"\"",
	}, "\n")
}

func persistModelfile(contextModel, modelfile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve home directory for modelfile storage: %w", err)
	}
	dir := filepath.Join(strings.TrimSpace(home), ".omnidex", "modelfiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create modelfile directory %s: %w", dir, err)
	}

	name := sanitizeModelNameComponent(contextModel)
	if name == "" {
		name = fmt.Sprintf("ctx-%d", time.Now().UnixNano())
	}
	path := filepath.Join(dir, name+".Modelfile")
	if err := os.WriteFile(path, []byte(modelfile), 0o600); err != nil {
		return "", fmt.Errorf("write modelfile %s: %w", path, err)
	}
	return path, nil
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
