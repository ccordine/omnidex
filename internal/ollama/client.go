package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

type embeddingsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingsResponse struct {
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
		baseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
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
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("ollama embedding model is required")
	}
	payload, err := json.Marshal(embeddingsRequest{
		Model: model,
		Input: content,
	})
	if err != nil {
		return nil, fmt.Errorf("encode ollama embedding request: %w", err)
	}

	status, body, err := c.postJSON(ctx, "/api/embed", payload)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding request: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf(
			"ollama embedding failed: endpoint=/api/embed status=%d body=%s",
			status,
			string(body),
		)
	}

	var parsed embeddingsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode ollama embedding response: %w", err)
	}
	if len(parsed.Embeddings) != 1 {
		return nil, fmt.Errorf(
			"ollama embedding response returned %d vectors, expected exactly one",
			len(parsed.Embeddings),
		)
	}
	vector := parsed.Embeddings[0]
	if len(vector) == 0 {
		return nil, fmt.Errorf("ollama embedding response returned an empty vector")
	}
	for index, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf(
				"ollama embedding response returned a non-finite value at index %d",
				index,
			)
		}
	}
	return vector, nil
}

func (c *Client) wrapConnectivityError(err error, _ string) error {
	return err
}
