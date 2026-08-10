package ollama

import (
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

const (
	controlPlaneMaxOutputTokens = 2048
	ollamaChatResponseBodyLimit = 16 * 1024 * 1024
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   any           `json:"format,omitempty"`
	Think    *bool         `json:"think,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
}

type chatOptions struct {
	NumPredict  int      `json:"num_predict,omitempty"`
	NumCtx      int      `json:"num_ctx,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// Chat runs one direct request without creating an ephemeral model.
func (c *Client) Chat(ctx context.Context, model, system, user string) (string, error) {
	return c.chat(ctx, model, system, user, 0, c.contextTokens, "", nil, false, nil)
}

func (c *Client) chat(
	ctx context.Context,
	model, system, user string,
	maxOutputTokens, contextTokens int,
	responseFormat string,
	responseSchema map[string]any,
	thinkingEnabled bool,
	temperature *float64,
) (string, error) {
	if err := llm.ValidateResponseContract(llm.PreparedModel{
		ResponseFormat: responseFormat, ResponseSchema: responseSchema,
		ThinkingEnabled: thinkingEnabled, Temperature: temperature,
	}); err != nil {
		return "", err
	}
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
	request := chatRequest{Model: model, Messages: messages, Stream: false}
	controlPlane := strings.Contains(system, "CONTROL_PLANE_COMMAND:") || responseFormat == llm.ResponseFormatJSON
	if controlPlane {
		if maxOutputTokens <= 0 {
			maxOutputTokens = controlPlaneMaxOutputTokens
		}
		request.Format = "json"
		if len(responseSchema) > 0 {
			request.Format = responseSchema
		}
	}
	if contextTokens > 0 {
		if err := llm.ValidateInferenceBudget(contextTokens, maxOutputTokens, system, user); err != nil {
			return "", err
		}
		request.Options = &chatOptions{NumCtx: contextTokens}
	}
	if maxOutputTokens > 0 {
		if request.Options == nil {
			request.Options = &chatOptions{}
		}
		request.Think = &thinkingEnabled
		request.Options.NumPredict = maxOutputTokens
		if temperature == nil {
			zero := 0.0
			temperature = &zero
		}
		value := *temperature
		request.Options.Temperature = &value
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
	body, err := io.ReadAll(io.LimitReader(response.Body, ollamaChatResponseBodyLimit+1))
	if err != nil {
		return "", err
	}
	if len(body) > ollamaChatResponseBodyLimit {
		return "", fmt.Errorf("ollama chat response exceeds %d bytes", ollamaChatResponseBodyLimit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("ollama chat failed: status=%d body=%s", response.StatusCode, string(body))
	}
	var parsed chatResponse
	if err := exactjson.ValidateCompatibleObject(body, chatResponse{}, "Ollama chat response"); err != nil {
		return "", fmt.Errorf("decode exact Ollama chat response: %w", err)
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	out := parsed.Message.Content
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("ollama response missing message content")
	}
	return out, nil
}
