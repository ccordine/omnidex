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

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

type chatStreamChunk struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

func (c *Client) ChatStream(
	ctx context.Context,
	model, system, user string,
	onChunk func(string) error,
) (string, error) {
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
	request := chatRequest{Model: model, Messages: messages, Stream: true}
	if c.contextTokens > 0 {
		if err := llm.ValidateInferenceBudget(c.contextTokens, 0, system, user); err != nil {
			return "", err
		}
		request.Options = &chatOptions{NumCtx: c.contextTokens}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload),
	)
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return "", c.wrapConnectivityError(err, "/api/chat")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
		if readErr != nil {
			return "", readErr
		}
		if len(body) > 64*1024 {
			return "", fmt.Errorf("ollama chat error response exceeds 65536 bytes")
		}
		return "", fmt.Errorf("ollama chat failed: status=%d body=%s", response.StatusCode, string(body))
	}
	var full strings.Builder
	done := false
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk chatStreamChunk
		if err := exactjson.ValidateCompatibleObject(
			[]byte(line), chatStreamChunk{}, "Ollama chat stream chunk",
		); err != nil {
			return full.String(), fmt.Errorf("decode exact Ollama chat stream: %w", err)
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return full.String(), fmt.Errorf("decode Ollama chat stream: %w", err)
		}
		if text := chunk.Message.Content; text != "" {
			full.WriteString(text)
			if onChunk != nil {
				if err := onChunk(text); err != nil {
					return full.String(), err
				}
			}
		}
		if chunk.Done {
			done = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), err
	}
	if !done {
		return full.String(), fmt.Errorf("ollama chat stream ended before the done frame")
	}
	if strings.TrimSpace(full.String()) == "" {
		return "", fmt.Errorf("ollama response missing message content")
	}
	return full.String(), nil
}
