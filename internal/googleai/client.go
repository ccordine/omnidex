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
)

type Client struct {
	baseURL        string
	apiKey         string
	embeddingModel string
	httpClient     *http.Client
}

type part struct {
	Text string `json:"text"`
}

type content struct {
	Parts []part `json:"parts"`
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

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, reqURL.String(), bytes.NewReader(encoded),
	)
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
		return fmt.Errorf(
			"google request failed: status=%d body=%s",
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
		value = "https://generativelanguage.googleapis.com/v1beta"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	return strings.TrimRight(value, "/")
}

func modelPath(model string) string {
	model = strings.TrimPrefix(strings.TrimSpace(model), "/")
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}
