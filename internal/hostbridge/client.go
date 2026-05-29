package hostbridge

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
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:   strings.TrimSpace(token),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	return c.getJSON(ctx, "/healthz")
}

func (c *Client) Browse(ctx context.Context, path string) (*BrowseResult, error) {
	query := url.Values{}
	if strings.TrimSpace(path) != "" {
		query.Set("path", strings.TrimSpace(path))
	}
	payload, err := c.getJSON(ctx, "/v1/browse?"+query.Encode())
	if err != nil {
		return nil, err
	}
	result := &BrowseResult{
		Path:   stringField(payload, "path"),
		Parent: stringField(payload, "parent"),
	}
	if rawEntries, ok := payload["entries"].([]any); ok {
		for _, item := range rawEntries {
			entryMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			result.Entries = append(result.Entries, Entry{
				Name:  stringField(entryMap, "name"),
				Path:  stringField(entryMap, "path"),
				IsDir: boolField(entryMap, "is_dir"),
			})
		}
	}
	return result, nil
}

type PickResult struct {
	Path     string
	Canceled bool
}

func (c *Client) PickDirectory(ctx context.Context, startPath string) (PickResult, error) {
	body, err := json.Marshal(map[string]string{"start_path": strings.TrimSpace(startPath)})
	if err != nil {
		return PickResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/pick-directory", bytes.NewReader(body))
	if err != nil {
		return PickResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return PickResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return PickResult{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return PickResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PickResult{}, fmt.Errorf("%s", stringField(payload, "error"))
	}
	if boolField(payload, "canceled") {
		return PickResult{Canceled: true}, nil
	}
	path := stringField(payload, "path")
	if path == "" {
		return PickResult{Canceled: true}, nil
	}
	return PickResult{Path: path}, nil
}

func (c *Client) getJSON(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", stringField(payload, "error"))
	}
	return payload, nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.Token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
}

func stringField(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func boolField(payload map[string]any, key string) bool {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), "true")
	}
}
