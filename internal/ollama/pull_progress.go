package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/modelref"
)

const (
	maxPullProgressEventBytes = 64 * 1024
	maxPullProgressEvents     = 1_000_000
)

// PullProgress is one provider-reported state from Ollama's streaming pull
// endpoint. It is observation data; it never grants workflow authority.
type PullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (progress PullProgress) Validate() error {
	if progress.Status == "" || progress.Status != strings.TrimSpace(progress.Status) ||
		len(progress.Status) > 512 || !utf8.ValidString(progress.Status) ||
		strings.ContainsRune(progress.Status, '\x00') {
		return fmt.Errorf("Ollama pull status must be bounded canonical text")
	}
	if progress.Digest != strings.TrimSpace(progress.Digest) || len(progress.Digest) > 256 ||
		!utf8.ValidString(progress.Digest) || strings.ContainsRune(progress.Digest, '\x00') {
		return fmt.Errorf("Ollama pull digest must be bounded canonical text")
	}
	if progress.Total < 0 || progress.Completed < 0 ||
		(progress.Total > 0 && progress.Completed > progress.Total) {
		return fmt.Errorf("Ollama pull byte progress is invalid")
	}
	if progress.Error != strings.TrimSpace(progress.Error) || len(progress.Error) > 2*1024 ||
		!utf8.ValidString(progress.Error) || strings.ContainsRune(progress.Error, '\x00') {
		return fmt.Errorf("Ollama pull error must be bounded canonical text")
	}
	return nil
}

func (c *Client) PullModelProgress(
	ctx context.Context,
	model string,
	observe func(PullProgress) error,
) error {
	if err := modelref.ValidateOllamaName(model); err != nil {
		return err
	}
	if observe == nil {
		return fmt.Errorf("progress observer is required")
	}
	payload, err := json.Marshal(pullModelRequest{Name: model, Model: model, Stream: true})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return c.wrapConnectivityError(err, "/api/pull")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxPullProgressEventBytes+1))
		if readErr != nil {
			return readErr
		}
		if len(body) > maxPullProgressEventBytes {
			return fmt.Errorf("Ollama pull failure body exceeds %d bytes", maxPullProgressEventBytes)
		}
		return fmt.Errorf("ollama pull failed: status=%d body=%s", response.StatusCode, body)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4*1024), maxPullProgressEventBytes)
	terminal := false
	count := 0
	for scanner.Scan() {
		count++
		if count > maxPullProgressEvents {
			return fmt.Errorf("Ollama pull stream exceeds %d events", maxPullProgressEvents)
		}
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			return fmt.Errorf("Ollama pull stream contains an empty event")
		}
		if err := exactjson.ValidateCompatibleObject(raw, PullProgress{}, "Ollama pull event"); err != nil {
			return err
		}
		var progress PullProgress
		if err := json.Unmarshal(raw, &progress); err != nil {
			return fmt.Errorf("decode Ollama pull event: %w", err)
		}
		if err := progress.Validate(); err != nil {
			return err
		}
		if progress.Error != "" {
			return fmt.Errorf("Ollama pull failed: %s", progress.Error)
		}
		if terminal {
			return fmt.Errorf("Ollama pull emitted state after terminal success")
		}
		if err := observe(progress); err != nil {
			return fmt.Errorf("persist Ollama pull progress: %w", err)
		}
		terminal = progress.Status == "success"
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Ollama pull stream: %w", err)
	}
	if !terminal {
		return fmt.Errorf("Ollama pull stream ended without terminal success")
	}
	return nil
}
