package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	MaxModelPageSize         = 100
	MaxModelCatalogBodyBytes = 4 * 1024 * 1024
)

type ModelPage struct {
	Models  []ModelInfo `json:"models"`
	Offset  int         `json:"offset"`
	HasMore bool        `json:"has_more"`
}

func (c *Client) ListModelPage(ctx context.Context, limit, offset int) (ModelPage, error) {
	if limit < 1 || limit > MaxModelPageSize {
		return ModelPage{}, fmt.Errorf("Ollama model page limit must be between 1 and %d", MaxModelPageSize)
	}
	if offset < 0 {
		return ModelPage{}, fmt.Errorf("Ollama model page offset must be non-negative")
	}
	models := make([]ModelInfo, 0, limit+1)
	seen := 0
	err := c.visitModels(ctx, func(item ModelInfo) {
		if seen >= offset && len(models) < limit+1 {
			models = append(models, item)
		}
		seen++
	})
	if err != nil {
		return ModelPage{}, err
	}
	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}
	return ModelPage{Models: models, Offset: offset, HasMore: hasMore}, nil
}

func (c *Client) visitModels(ctx context.Context, visit func(ModelInfo)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.wrapConnectivityError(err, "/api/tags")
	}
	defer resp.Body.Close()
	limited := &io.LimitedReader{R: resp.Body, N: MaxModelCatalogBodyBytes + 1}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(limited)
		if readErr != nil {
			return readErr
		}
		if limited.N == 0 {
			return fmt.Errorf("Ollama tags failure body exceeds %d bytes", MaxModelCatalogBodyBytes)
		}
		return fmt.Errorf("ollama tags failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	decoder := json.NewDecoder(limited)
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return fmt.Errorf("decode Ollama tags object: %w", exactJSONError(err))
	}
	foundModels := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode Ollama tags field: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("Ollama tags field name is not a string")
		}
		if key != "models" {
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return fmt.Errorf("decode Ollama tags field %q: %w", key, err)
			}
			continue
		}
		if foundModels {
			return fmt.Errorf("Ollama tags contains duplicate models fields")
		}
		foundModels = true
		if err := decodeModelArray(decoder, visit); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("finish Ollama tags object: %w", err)
	}
	if !foundModels {
		return fmt.Errorf("Ollama tags response is missing models")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("Ollama tags response contains trailing JSON")
		}
		return fmt.Errorf("finish Ollama tags response: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("Ollama tags response exceeds %d bytes", MaxModelCatalogBodyBytes)
	}
	return nil
}

func decodeModelArray(decoder *json.Decoder, visit func(ModelInfo)) error {
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return fmt.Errorf("Ollama tags models must be an array")
	}
	for decoder.More() {
		var item ModelInfo
		if err := decoder.Decode(&item); err != nil {
			return fmt.Errorf("decode Ollama model: %w", err)
		}
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = strings.TrimSpace(item.Model)
		}
		if item.Name != "" {
			visit(item)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("finish Ollama models array: %w", err)
	}
	return nil
}

func exactJSONError(err error) error {
	if err == nil {
		return fmt.Errorf("unexpected JSON token")
	}
	return err
}
