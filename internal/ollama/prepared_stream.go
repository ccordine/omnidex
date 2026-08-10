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

type preparedStreamChunk struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
	Error   string      `json:"error,omitempty"`
}

func (c *Client) GeneratePreparedStream(
	ctx context.Context,
	prepared llm.PreparedModel,
	observe func(llm.GenerationProgress) error,
) (string, error) {
	request, err := c.preparedStreamRequest(prepared)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode Ollama streaming request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create Ollama streaming request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return "", c.wrapConnectivityError(err, "/api/chat")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		if readErr != nil {
			return "", fmt.Errorf("read Ollama streaming error response: %w", readErr)
		}
		return "", fmt.Errorf("ollama chat failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var output strings.Builder
	done := false
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk preparedStreamChunk
		if err := exactjson.ValidateCompatibleObject(
			[]byte(line), preparedStreamChunk{}, "Ollama streaming response",
		); err != nil {
			return output.String(), fmt.Errorf("decode exact Ollama streaming chunk: %w", err)
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return output.String(), fmt.Errorf("decode Ollama streaming chunk: %w", err)
		}
		if message := strings.TrimSpace(chunk.Error); message != "" {
			return output.String(), fmt.Errorf("ollama streaming generation failed: %s", message)
		}
		if text := chunk.Message.Content; text != "" {
			output.WriteString(text)
			if observe != nil {
				if err := observe(llm.GenerationProgress{OutputBytes: output.Len()}); err != nil {
					return output.String(), fmt.Errorf("observe Ollama streaming generation: %w", err)
				}
			}
		}
		if chunk.Done {
			done = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return output.String(), fmt.Errorf("read Ollama streaming response: %w", err)
	}
	if !done {
		return output.String(), fmt.Errorf("ollama streaming response ended before the done frame")
	}
	result := output.String()
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("ollama response missing message content")
	}
	return result, nil
}

func (c *Client) preparedStreamRequest(prepared llm.PreparedModel) (chatRequest, error) {
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return chatRequest{}, err
	}
	model := strings.TrimSpace(prepared.ContextModel)
	if model == "" {
		model = strings.TrimSpace(prepared.BaseModel)
	}
	if model == "" {
		model = strings.TrimSpace(c.defaultModel)
	}
	if model == "" {
		return chatRequest{}, fmt.Errorf("model is required")
	}
	system := strings.TrimSpace(prepared.Prompt)
	if system == "" {
		return chatRequest{}, fmt.Errorf("prepared prompt is required")
	}
	hint := strings.TrimSpace(prepared.PromptHint)
	if hint == "" {
		hint = llm.MinimalGeneratePrompt
	}
	contextTokens := prepared.ContextTokens
	if contextTokens == 0 {
		contextTokens = c.contextTokens
	}
	if err := llm.ValidateInferenceBudget(contextTokens, prepared.MaxOutputTokens, system, hint); err != nil {
		return chatRequest{}, err
	}
	request := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: hint},
		},
		Stream: true,
	}
	if contextTokens > 0 {
		request.Options = &chatOptions{NumCtx: contextTokens}
	}
	if prepared.MaxOutputTokens > 0 {
		thinkingDisabled := false
		zero := 0.0
		request.Think = &thinkingDisabled
		if request.Options == nil {
			request.Options = &chatOptions{}
		}
		request.Options.NumPredict = prepared.MaxOutputTokens
		request.Options.Temperature = &zero
	}
	if prepared.ResponseFormat == llm.ResponseFormatJSON {
		request.Format = "json"
		if len(prepared.ResponseSchema) > 0 {
			request.Format = prepared.ResponseSchema
		}
	}
	return request, nil
}
