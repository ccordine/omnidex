package ollama

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Message                       chatMessage                     `json:"message"`
	Done                          *bool                           `json:"done,omitempty"`
	TotalDuration                 *int64                          `json:"total_duration,omitempty"`
	LoadDuration                  *int64                          `json:"load_duration,omitempty"`
	PromptEvalCount               *int                            `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration            *int64                          `json:"prompt_eval_duration,omitempty"`
	EvalCount                     *int                            `json:"eval_count,omitempty"`
	EvalDuration                  *int64                          `json:"eval_duration,omitempty"`
	ProviderRequestSHA256         string                          `json:"-"`
	ProviderRequestDisposition    llm.ProviderRequestDisposition  `json:"-"`
	ProviderHTTPStatus            int                             `json:"-"`
	ProviderResponseDisposition   llm.ProviderResponseDisposition `json:"-"`
	ProviderResponseComplete      bool                            `json:"-"`
	ProviderResponseSHA256        string                          `json:"-"`
	ProviderResponseBytes         int64                           `json:"-"`
	ProviderResponseCaptureSHA256 string                          `json:"-"`
	ProviderResponseCapturedBytes int                             `json:"-"`
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
	parsed, err := c.chatResponse(
		ctx, model, system, user, maxOutputTokens, contextTokens,
		responseFormat, responseSchema, thinkingEnabled, temperature,
	)
	if err != nil {
		return "", err
	}
	return parsed.Message.Content, nil
}

func (c *Client) chatResponse(
	ctx context.Context,
	model, system, user string,
	maxOutputTokens, contextTokens int,
	responseFormat string,
	responseSchema map[string]any,
	thinkingEnabled bool,
	temperature *float64,
) (chatResponse, error) {
	if err := llm.ValidateResponseContract(llm.PreparedModel{
		ResponseFormat: responseFormat, ResponseSchema: responseSchema,
		ThinkingEnabled: thinkingEnabled, Temperature: temperature,
	}); err != nil {
		return chatResponse{}, err
	}
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return chatResponse{}, fmt.Errorf("model is required")
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
			return chatResponse{}, err
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
	payload, err := exactjson.Canonical(request)
	if err != nil {
		return chatResponse{}, err
	}
	requestDigest := sha256.Sum256(payload)
	partial := chatResponse{ProviderRequestSHA256: hex.EncodeToString(requestDigest[:])}
	httpRequest, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload),
	)
	if err != nil {
		return chatResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, disposition, err := c.doExactProviderRequest(httpRequest)
	partial.ProviderRequestDisposition = disposition
	if err != nil {
		partial.ProviderResponseDisposition = llm.ProviderResponseTransportError
		return partial, c.wrapConnectivityError(err, "/api/chat")
	}
	defer response.Body.Close()
	partial.ProviderHTTPStatus = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, ollamaChatResponseBodyLimit+1))
	partial.ProviderResponseCapturedBytes = len(body)
	captureDigest := sha256.Sum256(body)
	partial.ProviderResponseCaptureSHA256 = hex.EncodeToString(captureDigest[:])
	if err != nil {
		partial.ProviderResponseBytes = int64(len(body))
		partial.ProviderResponseDisposition = llm.ProviderResponseBodyReadError
		return partial, err
	}
	if len(body) > ollamaChatResponseBodyLimit {
		partial.ProviderResponseBytes = int64(len(body))
		partial.ProviderResponseDisposition = llm.ProviderResponseBodyLimit
		return partial, fmt.Errorf("ollama chat response exceeds %d bytes", ollamaChatResponseBodyLimit)
	}
	partial.ProviderResponseBytes = int64(len(body))
	partial.ProviderResponseComplete = true
	partial.ProviderResponseSHA256 = partial.ProviderResponseCaptureSHA256
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var parsed chatResponse
		decodeErr := exactjson.ValidateCompatibleObject(body, chatResponse{}, "Ollama chat error response")
		if decodeErr == nil {
			decodeErr = json.Unmarshal(body, &parsed)
		}
		parsed = mergeChatResponseEvidence(parsed, partial)
		parsed.ProviderResponseDisposition = llm.ProviderResponseHTTPError
		if decodeErr != nil {
			return parsed, fmt.Errorf("decode exact Ollama chat error response: %w", decodeErr)
		}
		return parsed, fmt.Errorf("ollama chat failed: status=%d body=%s", response.StatusCode, string(body))
	}
	var parsed chatResponse
	decodeErr := exactjson.ValidateCompatibleObject(body, chatResponse{}, "Ollama chat response")
	if decodeErr == nil {
		decodeErr = json.Unmarshal(body, &parsed)
	}
	parsed = mergeChatResponseEvidence(parsed, partial)
	if decodeErr != nil {
		parsed.ProviderResponseDisposition = llm.ProviderResponseInvalidJSON
		return parsed, fmt.Errorf("decode exact Ollama chat response: %w", decodeErr)
	}
	out := parsed.Message.Content
	if strings.TrimSpace(out) == "" {
		parsed.ProviderResponseDisposition = llm.ProviderResponseEmptyContent
		return parsed, fmt.Errorf("ollama response missing message content")
	}
	parsed.ProviderResponseDisposition = llm.ProviderResponseSucceeded
	return parsed, nil
}

func mergeChatResponseEvidence(parsed chatResponse, evidence chatResponse) chatResponse {
	parsed.ProviderRequestSHA256 = evidence.ProviderRequestSHA256
	parsed.ProviderRequestDisposition = evidence.ProviderRequestDisposition
	parsed.ProviderHTTPStatus = evidence.ProviderHTTPStatus
	parsed.ProviderResponseDisposition = evidence.ProviderResponseDisposition
	parsed.ProviderResponseComplete = evidence.ProviderResponseComplete
	parsed.ProviderResponseSHA256 = evidence.ProviderResponseSHA256
	parsed.ProviderResponseBytes = evidence.ProviderResponseBytes
	parsed.ProviderResponseCaptureSHA256 = evidence.ProviderResponseCaptureSHA256
	parsed.ProviderResponseCapturedBytes = evidence.ProviderResponseCapturedBytes
	return parsed
}
