package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

const ollamaPrewarmOutputTokens = 64

type ollamaPrewarmOptions struct {
	Model     string
	KeepAlive string
	NumCtx    int
}

type ollamaPrewarmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaPrewarmInferenceOptions struct {
	NumPredict  int     `json:"num_predict"`
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
}

type ollamaPrewarmRequest struct {
	Model     string                        `json:"model"`
	Messages  []ollamaPrewarmMessage        `json:"messages"`
	Stream    bool                          `json:"stream"`
	Think     bool                          `json:"think"`
	KeepAlive string                        `json:"keep_alive"`
	Options   ollamaPrewarmInferenceOptions `json:"options"`
}

type ollamaPrewarmReport struct {
	Model                 string  `json:"model"`
	BaseURL               string  `json:"base_url"`
	KeepAlive             string  `json:"keep_alive"`
	ContextTokens         int     `json:"context_tokens"`
	TotalDurationMS       int64   `json:"total_duration_ms"`
	LoadDurationMS        int64   `json:"load_duration_ms"`
	PromptTokens          int     `json:"prompt_tokens"`
	PromptTokensPerSecond float64 `json:"prompt_tokens_per_second"`
	EvalTokens            int     `json:"eval_tokens"`
	EvalTokensPerSecond   float64 `json:"eval_tokens_per_second"`
	AllocatedBytes        int64   `json:"allocated_bytes"`
	VRAMBytes             int64   `json:"vram_bytes"`
	OffloadPercent        float64 `json:"offload_percent"`
}

type ollamaPrewarmResponse struct {
	Model   string `json:"model"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done               bool  `json:"done"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

type ollamaRunningModels struct {
	Models []struct {
		Name          string `json:"name"`
		Size          int64  `json:"size"`
		SizeVRAM      int64  `json:"size_vram"`
		ContextLength int    `json:"context_length"`
	} `json:"models"`
}

func runOllamaPrewarm(args []string) {
	defaultContext, err := ollamaPrewarmDefaultContext()
	if err != nil {
		die(err.Error())
	}
	fs := flag.NewFlagSet("ollama:prewarm", flag.ExitOnError)
	modelName := fs.String("model", ollamaPrewarmDefaultModel(), "exact installed Ollama model")
	keepAlive := fs.String("keep-alive", "10m", "positive Ollama model retention duration")
	numCtx := fs.Int("num-ctx", defaultContext, "inference context tokens")
	timeout := fs.Duration("timeout", 10*time.Minute, "complete load and inference timeout")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	asJSON := fs.Bool("json", false, "print JSON report")
	_ = fs.Parse(args)

	if *timeout <= 0 {
		die("ollama prewarm timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := probeOllamaModel(ctx, *baseURL, ollamaPrewarmOptions{
		Model: *modelName, KeepAlive: *keepAlive, NumCtx: *numCtx,
	})
	if err != nil {
		die(err.Error())
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			die("encode Ollama prewarm report: " + err.Error())
		}
		return
	}
	printOllamaPrewarmReport(report)
}

func probeOllamaModel(ctx context.Context, baseURL string, options ollamaPrewarmOptions) (ollamaPrewarmReport, error) {
	var report ollamaPrewarmReport
	if ctx == nil {
		return report, fmt.Errorf("ollama prewarm requires a context")
	}
	options.Model = strings.TrimSpace(options.Model)
	if options.Model == "" {
		return report, fmt.Errorf("ollama prewarm model is required")
	}
	if err := llm.ValidateInferenceContextTokens(options.NumCtx); err != nil {
		return report, err
	}
	retention, err := time.ParseDuration(strings.TrimSpace(options.KeepAlive))
	if err != nil || retention <= 0 {
		return report, fmt.Errorf("ollama prewarm keep alive must be a positive duration, received %q", options.KeepAlive)
	}
	baseURL = normalizeStatusURL(baseURL, defaultOllamaBaseURL())
	request := ollamaPrewarmRequest{
		Model: options.Model,
		Messages: []ollamaPrewarmMessage{{
			Role: "user", Content: llm.MinimalGeneratePrompt,
		}},
		KeepAlive: strings.TrimSpace(options.KeepAlive),
		Options: ollamaPrewarmInferenceOptions{
			NumPredict: ollamaPrewarmOutputTokens, NumCtx: options.NumCtx,
		},
	}
	var response ollamaPrewarmResponse
	if err := ollamaPrewarmJSON(ctx, http.MethodPost, baseURL+"/api/chat", request, &response); err != nil {
		return report, fmt.Errorf("ollama prewarm chat: %w", err)
	}
	if !response.Done {
		return report, fmt.Errorf("ollama prewarm response did not complete")
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return report, fmt.Errorf("ollama prewarm response returned empty content")
	}

	var running ollamaRunningModels
	if err := ollamaPrewarmJSON(ctx, http.MethodGet, baseURL+"/api/ps", nil, &running); err != nil {
		return report, fmt.Errorf("ollama prewarm runner inspection: %w", err)
	}
	for _, candidate := range running.Models {
		if !ollama.MatchesOllamaModel(options.Model, candidate.Name) {
			continue
		}
		if candidate.ContextLength != options.NumCtx {
			return report, fmt.Errorf(
				"ollama runner context length is %d, requested %d",
				candidate.ContextLength, options.NumCtx,
			)
		}
		report = ollamaPrewarmReport{
			Model: strings.TrimSpace(response.Model), BaseURL: baseURL,
			KeepAlive: strings.TrimSpace(options.KeepAlive), ContextTokens: candidate.ContextLength,
			TotalDurationMS:       response.TotalDuration / int64(time.Millisecond),
			LoadDurationMS:        response.LoadDuration / int64(time.Millisecond),
			PromptTokens:          response.PromptEvalCount,
			PromptTokensPerSecond: ollamaTokenRate(response.PromptEvalCount, response.PromptEvalDuration),
			EvalTokens:            response.EvalCount,
			EvalTokensPerSecond:   ollamaTokenRate(response.EvalCount, response.EvalDuration),
			AllocatedBytes:        candidate.Size, VRAMBytes: candidate.SizeVRAM,
		}
		if report.Model == "" {
			report.Model = options.Model
		}
		if candidate.Size > 0 {
			report.OffloadPercent = float64(candidate.SizeVRAM) / float64(candidate.Size) * 100
		}
		return report, nil
	}
	return report, fmt.Errorf("ollama model %q is not present in /api/ps after prewarm", options.Model)
}

func ollamaPrewarmJSON(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, trimStatusBody(raw))
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func ollamaTokenRate(tokens int, durationNS int64) float64 {
	if tokens <= 0 || durationNS <= 0 {
		return 0
	}
	return float64(tokens) / (float64(durationNS) / float64(time.Second))
}

func ollamaPrewarmDefaultModel() string {
	for _, key := range []string{"OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT", "OLLAMA_MODEL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func ollamaPrewarmDefaultContext() (int, error) {
	raw := strings.TrimSpace(os.Getenv("INFERENCE_CONTEXT_TOKENS"))
	if raw == "" {
		return llm.DefaultInferenceContextTokens, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("INFERENCE_CONTEXT_TOKENS must be an integer, received %q", raw)
	}
	if err := llm.ValidateInferenceContextTokens(value); err != nil {
		return 0, fmt.Errorf("INFERENCE_CONTEXT_TOKENS is invalid: %w", err)
	}
	return value, nil
}

func printOllamaPrewarmReport(report ollamaPrewarmReport) {
	fmt.Printf(
		"ollama prewarm: ok model=%s context=%d load_ms=%d total_ms=%d prompt_tps=%.2f eval_tps=%.2f allocated_gib=%.2f vram_gib=%.2f offload=%.1f%% keep_alive=%s\n",
		report.Model,
		report.ContextTokens,
		report.LoadDurationMS,
		report.TotalDurationMS,
		report.PromptTokensPerSecond,
		report.EvalTokensPerSecond,
		float64(report.AllocatedBytes)/(1024*1024*1024),
		float64(report.VRAMBytes)/(1024*1024*1024),
		report.OffloadPercent,
		report.KeepAlive,
	)
}
