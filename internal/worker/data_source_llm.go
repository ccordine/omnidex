package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

type dataSourceLLMAdapter struct {
	client llm.Client
	model  string
}

func (s *Service) dataSourceLLMClient(job model.Job) (omni.DBManagerLLMClient, error) {
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("configured LLM client is required for data source queries")
	}
	routing, err := modelRoutingFromJobMetadata(job.Metadata, s.models)
	if err != nil {
		return nil, err
	}
	modelName := strings.TrimSpace(routing.Tagging)
	if modelName == "" {
		return nil, fmt.Errorf("tagging model is not configured for data source queries")
	}
	return &dataSourceLLMAdapter{client: s.llm, model: modelName}, nil
}

func (a *dataSourceLLMAdapter) ChatRaw(ctx context.Context, request omni.OllamaChatRequest) (omni.OllamaChatResponse, error) {
	if a == nil || a.client == nil {
		return omni.OllamaChatResponse{}, fmt.Errorf("data source LLM adapter requires a client")
	}
	if ctx == nil {
		return omni.OllamaChatResponse{}, fmt.Errorf("data source LLM adapter requires a context")
	}
	modelName := strings.TrimSpace(a.model)
	if modelName == "" {
		return omni.OllamaChatResponse{}, fmt.Errorf("data source LLM adapter requires a model")
	}
	prompt, err := dataSourcePrompt(request)
	if err != nil {
		return omni.OllamaChatResponse{}, err
	}
	content, err := a.client.Generate(ctx, modelName, prompt)
	if err != nil {
		return omni.OllamaChatResponse{}, fmt.Errorf("generate data source analysis with model %q: %w", modelName, err)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return omni.OllamaChatResponse{}, fmt.Errorf("data source analysis model %q returned empty output", modelName)
	}
	return omni.OllamaChatResponse{Model: modelName, Content: content, Done: true}, nil
}

func dataSourcePrompt(request omni.OllamaChatRequest) (string, error) {
	sections := make([]string, 0, len(request.Messages)+3)
	if system := strings.TrimSpace(request.ContextSystem); system != "" {
		sections = append(sections, "SYSTEM:\n"+system)
	}
	for index, message := range request.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case "system", "user", "assistant", "tool":
		default:
			return "", fmt.Errorf("data source message %d has unsupported role %q", index, message.Role)
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			return "", fmt.Errorf("data source message %d content is required", index)
		}
		sections = append(sections, strings.ToUpper(role)+":\n"+content)
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("data source LLM request requires at least one message")
	}
	if request.Format != nil {
		format, err := json.Marshal(request.Format)
		if err != nil {
			return "", fmt.Errorf("encode data source response schema: %w", err)
		}
		sections = append(sections, "RESPONSE CONTRACT:\nReturn only JSON matching this schema:\n"+string(format))
	}
	constraints, err := dataSourceGenerationConstraints(request.Options)
	if err != nil {
		return "", err
	}
	if constraints != "" {
		sections = append(sections, "GENERATION CONSTRAINTS:\n"+constraints)
	}
	if strings.TrimSpace(request.KeepAlive) != "" {
		return "", fmt.Errorf("data source LLM request cannot set provider-specific keep_alive")
	}
	return strings.Join(sections, "\n\n"), nil
}

func dataSourceGenerationConstraints(options map[string]interface{}) (string, error) {
	if len(options) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(options))
	for key, value := range options {
		switch strings.TrimSpace(key) {
		case "temperature":
			temperature, ok := numericOption(value)
			if !ok || temperature != 0 {
				return "", fmt.Errorf("data source temperature must be numeric zero")
			}
			lines = append(lines, "Produce deterministic output; do not introduce creative variation.")
		case "num_predict":
			limit, ok := positiveIntegerOption(value)
			if !ok {
				return "", fmt.Errorf("data source num_predict must be a positive integer")
			}
			lines = append(lines, "Keep the response within approximately "+strconv.FormatInt(limit, 10)+" tokens.")
		default:
			return "", fmt.Errorf("unsupported data source generation option %q", key)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func numericOption(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func positiveIntegerOption(value interface{}) (int64, bool) {
	number, ok := numericOption(value)
	if !ok || number < 1 || number != float64(int64(number)) {
		return 0, false
	}
	return int64(number), true
}
