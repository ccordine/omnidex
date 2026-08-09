package modelgauntlet

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

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

const maxOllamaResponseBytes = 2 * 1024 * 1024

type OllamaGenerator struct {
	baseURL string
	client  *http.Client
}

type ollamaMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	Thinking string `json:"thinking,omitempty"`
}

type ollamaChatOptions struct {
	NumPredict  int     `json:"num_predict"`
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
}

type ollamaChatRequest struct {
	Model     string            `json:"model"`
	Messages  []ollamaMessage   `json:"messages"`
	Stream    bool              `json:"stream"`
	Format    any               `json:"format,omitempty"`
	Think     *bool             `json:"think"`
	KeepAlive string            `json:"keep_alive"`
	Options   ollamaChatOptions `json:"options"`
}

type ollamaChatResponse struct {
	Model              string        `json:"model"`
	Message            ollamaMessage `json:"message"`
	Done               bool          `json:"done"`
	TotalDuration      int64         `json:"total_duration"`
	LoadDuration       int64         `json:"load_duration"`
	PromptEvalCount    int           `json:"prompt_eval_count"`
	PromptEvalDuration int64         `json:"prompt_eval_duration"`
	EvalCount          int           `json:"eval_count"`
	EvalDuration       int64         `json:"eval_duration"`
}

type ollamaRunningModels struct {
	Models []struct {
		Name          string `json:"name"`
		Size          int64  `json:"size"`
		SizeVRAM      int64  `json:"size_vram"`
		ContextLength int    `json:"context_length"`
		Digest        string `json:"digest"`
		Details       struct {
			QuantizationLevel string `json:"quantization_level"`
			ParameterSize     string `json:"parameter_size"`
		} `json:"details"`
	} `json:"models"`
}

func NewOllamaGenerator(baseURL string, client *http.Client) (*OllamaGenerator, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("model gauntlet requires a valid Ollama HTTP(S) base URL")
	}
	if client == nil {
		return nil, fmt.Errorf("model gauntlet requires an explicit HTTP client")
	}
	return &OllamaGenerator{baseURL: baseURL, client: client}, nil
}

func (generator *OllamaGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if ctx == nil {
		return GenerateResponse{}, fmt.Errorf("Ollama generation requires a context")
	}
	if err := validateGenerateRequest(request); err != nil {
		return GenerateResponse{}, err
	}
	think := request.Think
	payload := ollamaChatRequest{
		Model: request.Model,
		Messages: []ollamaMessage{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		Think: &think, KeepAlive: request.KeepAlive,
		Options: ollamaChatOptions{NumPredict: request.MaxOutputTokens, NumCtx: request.ContextTokens},
	}
	if len(request.ResponseSchema) > 0 {
		payload.Format = request.ResponseSchema
	}
	var response ollamaChatResponse
	if err := generator.callJSON(ctx, http.MethodPost, "/api/chat", payload, &response); err != nil {
		return GenerateResponse{}, fmt.Errorf("Ollama gauntlet chat failed: %w", err)
	}
	if !response.Done {
		return GenerateResponse{}, fmt.Errorf("Ollama gauntlet response did not complete")
	}
	if !ollama.MatchesOllamaModel(request.Model, response.Model) {
		return GenerateResponse{}, fmt.Errorf("Ollama returned model %q for requested model %q", response.Model, request.Model)
	}
	if request.Think {
		if strings.TrimSpace(response.Message.Thinking) == "" && strings.TrimSpace(response.Message.Content) == "" {
			return GenerateResponse{}, fmt.Errorf("Ollama thinking response has empty thinking and content")
		}
	} else if strings.TrimSpace(response.Message.Content) == "" {
		return GenerateResponse{}, fmt.Errorf("Ollama structured response has empty content")
	}

	runner, err := generator.inspectRunner(ctx, request.Model, request.ContextTokens)
	if err != nil {
		return GenerateResponse{}, err
	}
	return GenerateResponse{
		Model: response.Model, ModelDigest: runner.Digest,
		Quantization: runner.Quantization, ParameterSize: runner.ParameterSize,
		Thinking: response.Message.Thinking, Content: response.Message.Content,
		TotalDuration: response.TotalDuration, LoadDuration: response.LoadDuration,
		PromptEvalCount: response.PromptEvalCount, PromptEvalDuration: response.PromptEvalDuration,
		EvalCount: response.EvalCount, EvalDuration: response.EvalDuration,
		AllocatedBytes: runner.Size, VRAMBytes: runner.SizeVRAM, RunnerContextTokens: runner.ContextLength,
	}, nil
}

func validateGenerateRequest(request GenerateRequest) error {
	if strings.TrimSpace(request.CaseID) == "" || strings.TrimSpace(request.Model) == "" {
		return fmt.Errorf("Ollama gauntlet request requires a case ID and model")
	}
	if strings.TrimSpace(request.SystemPrompt) == "" || strings.TrimSpace(request.UserPrompt) == "" {
		return fmt.Errorf("Ollama gauntlet request requires system and user prompts")
	}
	if request.MaxOutputTokens <= 0 {
		return fmt.Errorf("Ollama gauntlet request requires a positive output-token budget")
	}
	switch request.Stage {
	case StageDirect, StageBriefing, StageDeliberation, StageSynthesis:
	default:
		return fmt.Errorf("Ollama gauntlet call stage %q is unsupported", request.Stage)
	}
	switch request.Variant {
	case VariantDirect, VariantDeliberated, VariantPerSplitAdvisory, VariantFinalPassAdvisory:
	default:
		return fmt.Errorf("Ollama gauntlet variant %q is unsupported", request.Variant)
	}
	if err := llm.ValidateInferenceBudget(request.ContextTokens, request.MaxOutputTokens, request.SystemPrompt, request.UserPrompt); err != nil {
		return err
	}
	retention, err := time.ParseDuration(request.KeepAlive)
	if err != nil || retention <= 0 {
		return fmt.Errorf("Ollama gauntlet keep alive must be a positive duration, received %q", request.KeepAlive)
	}
	if request.Stage == StageDeliberation {
		if !request.Think || len(request.ResponseSchema) != 0 {
			return fmt.Errorf("deliberation stage requires thinking and forbids a response schema")
		}
		return nil
	}
	if request.Think || len(request.ResponseSchema) == 0 {
		return fmt.Errorf("structured stage %q must disable thinking and require a response schema", request.Stage)
	}
	return nil
}

type ollamaRunnerEvidence struct {
	Size          int64
	SizeVRAM      int64
	ContextLength int
	Digest        string
	Quantization  string
	ParameterSize string
}

func (generator *OllamaGenerator) inspectRunner(ctx context.Context, model string, contextTokens int) (ollamaRunnerEvidence, error) {
	var running ollamaRunningModels
	if err := generator.callJSON(ctx, http.MethodGet, "/api/ps", nil, &running); err != nil {
		return ollamaRunnerEvidence{}, fmt.Errorf("Ollama gauntlet runner inspection failed: %w", err)
	}
	for _, candidate := range running.Models {
		if !ollama.MatchesOllamaModel(model, candidate.Name) {
			continue
		}
		if candidate.ContextLength != contextTokens {
			return ollamaRunnerEvidence{}, fmt.Errorf("Ollama runner context length is %d, requested %d", candidate.ContextLength, contextTokens)
		}
		if err := validateSHA256("Ollama model", candidate.Digest); err != nil {
			return ollamaRunnerEvidence{}, err
		}
		if strings.TrimSpace(candidate.Details.QuantizationLevel) == "" {
			return ollamaRunnerEvidence{}, fmt.Errorf("Ollama model %q has no quantization evidence in /api/ps", model)
		}
		return ollamaRunnerEvidence{
			Size: candidate.Size, SizeVRAM: candidate.SizeVRAM, ContextLength: candidate.ContextLength,
			Digest: candidate.Digest, Quantization: candidate.Details.QuantizationLevel,
			ParameterSize: candidate.Details.ParameterSize,
		}, nil
	}
	return ollamaRunnerEvidence{}, fmt.Errorf("Ollama model %q is not present in /api/ps after generation", model)
}

func (generator *OllamaGenerator) callJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, generator.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := generator.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOllamaResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxOllamaResponseBytes {
		return fmt.Errorf("response exceeds %d-byte hard limit", maxOllamaResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
