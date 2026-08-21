package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/modelref"
)

type Client struct {
	baseURL        string
	defaultModel   string
	embeddingModel string
	contextTokens  int
	httpClient     *http.Client
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

type ModelInfo struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
	ModifiedAt time.Time    `json:"modified_at"`
}

type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

func (c *Client) HasModel(ctx context.Context, model string) (bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}
	found := false
	err := c.visitModels(ctx, func(item ModelInfo) {
		if MatchesOllamaModel(model, item.Name) {
			found = true
		}
	})
	return found, err
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

func New(baseURL, defaultModel, embeddingModel string, timeout time.Duration, contextTokens int) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return newClient(baseURL, defaultModel, embeddingModel, timeout, contextTokens)
}

// NewUnbounded leaves response duration under the supplied request context.
// It retains a bounded connection dial so an unreachable local provider does
// not hang before a request is established.
func NewUnbounded(baseURL, defaultModel, embeddingModel string, contextTokens int) *Client {
	return newClient(baseURL, defaultModel, embeddingModel, 0, contextTokens)
}

func newClient(
	baseURL, defaultModel, embeddingModel string,
	timeout time.Duration,
	contextTokens int,
) *Client {
	dialTimeout := 5 * time.Second
	if timeout > 0 && timeout < dialTimeout {
		dialTimeout = timeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.MaxResponseHeaderBytes = maxExactProviderResponseHeaderBytes
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

func (c *Client) PullModel(ctx context.Context, model string) error {
	if err := modelref.ValidateOllamaName(model); err != nil {
		return err
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
	if err := modelref.ValidateOllamaName(model); err != nil {
		return err
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
