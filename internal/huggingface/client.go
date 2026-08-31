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
)

type Client struct {
	baseURL        string
	apiKey         string
	embeddingModel string
	httpClient     *http.Client
}

func NewEmbedding(
	baseURL string,
	apiKey string,
	embeddingModel string,
	timeout time.Duration,
) *Client {
	return &Client{
		baseURL:        normalizeBaseURL(baseURL),
		apiKey:         strings.TrimSpace(apiKey),
		embeddingModel: strings.TrimSpace(embeddingModel),
		httpClient:     &http.Client{Timeout: timeout},
	}
}

func (c *Client) Embedding(ctx context.Context, input string) ([]float64, error) {
	model := strings.TrimSpace(c.embeddingModel)
	if model == "" {
		return nil, fmt.Errorf("HUGGINGFACE_EMBEDDING_MODEL is required")
	}
	path := "/hf-inference/models/" + escapeModelID(model) + "/pipeline/feature-extraction"
	var parsed any
	if err := c.doJSON(ctx, path, map[string]any{"inputs": input}, &parsed); err != nil {
		return nil, err
	}
	vector := extractEmbeddingVector(parsed)
	if len(vector) == 0 {
		return nil, fmt.Errorf("huggingface embedding response missing vector")
	}
	return vector, nil
}

func (c *Client) doJSON(ctx context.Context, path string, payload any, out any) error {
	if strings.TrimSpace(c.apiKey) == "" {
		return fmt.Errorf("HUGGINGFACE_API_KEY is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewReader(encoded),
	)
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
		return fmt.Errorf(
			"huggingface request failed: status=%d body=%s",
			resp.StatusCode, strings.TrimSpace(string(data)),
		)
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
