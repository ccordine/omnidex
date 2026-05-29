package hostbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		Path:    stringField(payload, "path"),
		Parent:  stringField(payload, "parent"),
		Entries: []Entry{},
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

func (c *Client) Mkdir(ctx context.Context, parent, name string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"parent": strings.TrimSpace(parent),
		"name":   strings.TrimSpace(name),
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/mkdir", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := readResponseBody(resp.Body)
	if err != nil {
		return "", err
	}
	payload, err := decodeResponseBody(raw, resp.StatusCode)
	if err != nil {
		return "", err
	}
	path := stringField(payload, "path")
	if path == "" {
		return "", fmt.Errorf("mkdir returned empty path")
	}
	return path, nil
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
	raw, err := readResponseBody(resp.Body)
	if err != nil {
		return PickResult{}, err
	}
	payload, err := decodeResponseBody(raw, resp.StatusCode)
	if err != nil {
		return PickResult{}, err
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

func (c *Client) ScanProjectTree(ctx context.Context, path string, maxFiles int) (ProjectWalkResult, error) {
	body, err := json.Marshal(map[string]any{
		"path":      strings.TrimSpace(path),
		"max_files": maxFiles,
	})
	if err != nil {
		return ProjectWalkResult{}, err
	}
	payload, err := c.postJSON(ctx, "/v1/project-map/scan", body)
	if err != nil {
		return ProjectWalkResult{}, err
	}
	return decodeProjectWalkResult(payload["walk"])
}

func (c *Client) PersistProjectMap(ctx context.Context, path string, indexJSON, mapJSON []byte) (string, string, error) {
	body, err := json.Marshal(map[string]any{
		"path":       strings.TrimSpace(path),
		"index_json": json.RawMessage(indexJSON),
		"map_json":   json.RawMessage(mapJSON),
	})
	if err != nil {
		return "", "", err
	}
	payload, err := c.postJSON(ctx, "/v1/project-map/scan", body)
	if err != nil {
		return "", "", err
	}
	return stringField(payload, "index_path"), stringField(payload, "map_path"), nil
}

func (c *Client) ReadProjectMap(ctx context.Context, path string) (map[string]any, string, bool, error) {
	query := url.Values{}
	query.Set("path", strings.TrimSpace(path))
	payload, err := c.getJSON(ctx, "/v1/project-map?"+query.Encode())
	if err != nil {
		return nil, "", false, err
	}
	rawMap, _ := payload["map"].(map[string]any)
	if rawMap == nil {
		rawMap = map[string]any{}
	}
	return rawMap, stringField(payload, "map_path"), boolField(payload, "exists"), nil
}

func decodeProjectWalkResult(raw any) (ProjectWalkResult, error) {
	if raw == nil {
		return ProjectWalkResult{}, fmt.Errorf("missing walk payload")
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return ProjectWalkResult{}, err
	}
	var walk ProjectWalkResult
	if err := json.Unmarshal(blob, &walk); err != nil {
		return ProjectWalkResult{}, err
	}
	return walk, nil
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
	raw, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeResponseBody(raw, resp.StatusCode)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeResponseBody(raw, resp.StatusCode)
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
