package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

const maxAdvisoryResponseBytes = 2 * 1024 * 1024

type advisoryChatResponse struct {
	Message struct {
		Thinking string `json:"thinking"`
		Content  string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

func (c *Client) GeneratePreparedAdvisory(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.AdvisoryResponse, error) {
	if !prepared.ThinkingEnabled {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation requires native thinking to be enabled")
	}
	if prepared.ResponseFormat != "" || len(prepared.ResponseSchema) != 0 {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation forbids a structured response contract")
	}
	model := strings.TrimSpace(prepared.ContextModel)
	if model == "" {
		model = strings.TrimSpace(prepared.BaseModel)
	}
	if model == "" {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation requires a model")
	}
	system := strings.TrimSpace(prepared.Prompt)
	user := strings.TrimSpace(prepared.PromptHint)
	if system == "" || user == "" {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation requires exact system and user prompts")
	}
	if prepared.MaxOutputTokens <= 0 || prepared.ContextTokens <= 0 {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation requires positive inference limits")
	}
	if err := llm.ValidateInferenceBudget(
		prepared.ContextTokens, prepared.MaxOutputTokens, system, user,
	); err != nil {
		return llm.AdvisoryResponse{}, err
	}
	think := true
	zero := 0.0
	payload := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false, Think: &think,
		Options: &chatOptions{
			NumPredict: prepared.MaxOutputTokens,
			NumCtx:     prepared.ContextTokens, Temperature: &zero,
		},
	}
	rawRequest, err := json.Marshal(payload)
	if err != nil {
		return llm.AdvisoryResponse{}, fmt.Errorf("encode Ollama advisory request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(rawRequest))
	if err != nil {
		return llm.AdvisoryResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return llm.AdvisoryResponse{}, c.wrapConnectivityError(err, "/api/chat")
	}
	defer response.Body.Close()
	rawResponse, err := io.ReadAll(io.LimitReader(response.Body, maxAdvisoryResponseBytes+1))
	if err != nil {
		return llm.AdvisoryResponse{}, err
	}
	if len(rawResponse) > maxAdvisoryResponseBytes {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory response exceeds %d bytes", maxAdvisoryResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory generation failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(rawResponse)))
	}
	var decoded advisoryChatResponse
	if err := json.Unmarshal(rawResponse, &decoded); err != nil {
		return llm.AdvisoryResponse{}, fmt.Errorf("decode Ollama advisory response: %w", err)
	}
	if !decoded.Done {
		return llm.AdvisoryResponse{}, fmt.Errorf("Ollama advisory response did not complete")
	}
	result := llm.AdvisoryResponse{Thinking: decoded.Message.Thinking, Content: decoded.Message.Content}
	if err := result.Validate(); err != nil {
		return llm.AdvisoryResponse{}, err
	}
	return result, nil
}
