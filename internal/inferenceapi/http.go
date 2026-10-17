package inferenceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Native HTTP providers consume the bounded primary input and context directly.
// Their model-specific input interpretation belongs outside the framework.
type HTTPProvider struct {
	baseURL, token, model string
	client                *http.Client
}

func NewHTTPProvider(baseURL, token, model string, timeout time.Duration) (*HTTPProvider, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 || strings.TrimSpace(model) == "" || timeout <= 0 || timeout > time.Minute {
		return nil, fmt.Errorf("invalid native inference provider configuration")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return nil, fmt.Errorf("native inference requires HTTPS or a loopback test listener")
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return nil, fmt.Errorf("invalid native provider token")
		}
	}
	return &HTTPProvider{baseURL: strings.TrimRight(baseURL, "/"), token: token, model: model,
		client: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (p *HTTPProvider) Check(ctx context.Context) error {
	var result struct{ Status, Model string }
	if err := p.request(ctx, "GET", "/healthz", nil, &result, 4096, false); err != nil || result.Status != "ready" || result.Model != p.model {
		return fmt.Errorf("configured native inference model is unavailable")
	}
	return nil
}

func (p *HTTPProvider) Predict(ctx context.Context, input Request) (string, error) {
	if err := validate(input); err != nil {
		return "", err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	var result struct {
		Text         string `json:"text"`
		Model        string `json:"model"`
		OutputTokens int    `json:"output_tokens"`
		FinishReason string `json:"finish_reason"`
	}
	if err := p.request(ctx, "POST", "/infer", body, &result, 24*1024, true); err != nil {
		return "", err
	}
	if result.Model != p.model || result.FinishReason != "stop" || result.OutputTokens < 1 || result.OutputTokens > input.MaxOutputTokens || strings.TrimSpace(result.Text) == "" || len(result.Text) > 16384 {
		return "", fmt.Errorf("native model did not return a complete bounded result")
	}
	return result.Text, nil
}

func (p *HTTPProvider) request(ctx context.Context, method, path string, body []byte, result any, limit int64, strict bool) error {
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid native inference request")
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("native inference request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("native inference returned an error")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return fmt.Errorf("native inference response exceeded its bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if decoder.Decode(result) != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid native inference response")
	}
	return nil
}
