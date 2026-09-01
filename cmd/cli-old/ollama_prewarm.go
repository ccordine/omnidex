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

type ollamaPrewarmOptions struct {
	Model     string
	KeepAlive string
	NumCtx    int
}

type ollamaPrewarmLoadOptions struct {
	NumCtx int `json:"num_ctx"`
}

type ollamaPrewarmRequest struct {
	Model     string                   `json:"model"`
	Stream    bool                     `json:"stream"`
	KeepAlive string                   `json:"keep_alive"`
	Options   ollamaPrewarmLoadOptions `json:"options"`
}

type ollamaPrewarmReport struct {
	Model           string  `json:"model"`
	BaseURL         string  `json:"base_url"`
	KeepAlive       string  `json:"keep_alive"`
	ContextTokens   int     `json:"context_tokens"`
	TotalDurationMS int64   `json:"total_duration_ms"`
	LoadDurationMS  int64   `json:"load_duration_ms"`
	AllocatedBytes  int64   `json:"allocated_bytes"`
	VRAMBytes       int64   `json:"vram_bytes"`
	OffloadPercent  float64 `json:"offload_percent"`
}

type ollamaPrewarmResponse struct {
	Model           string `json:"model"`
	Response        string `json:"response"`
	Thinking        string `json:"thinking"`
	Done            bool   `json:"done"`
	TotalDuration   int64  `json:"total_duration"`
	LoadDuration    int64  `json:"load_duration"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
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
	timeout := fs.Duration(
		"timeout", llm.MaximumModelRequestDuration,
		"complete model-load timeout; must be positive and no greater than 30 minutes",
	)
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	asJSON := fs.Bool("json", false, "print JSON report")
	_ = fs.Parse(args)

	if err := validateOllamaPrewarmTimeout(*timeout); err != nil {
		die(err.Error())
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

func validateOllamaPrewarmTimeout(timeout time.Duration) error {
	if timeout <= 0 || timeout > llm.MaximumModelRequestDuration {
		return fmt.Errorf(
			"ollama prewarm timeout must be positive and no greater than %s",
			llm.MaximumModelRequestDuration,
		)
	}
	return nil
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
		Model:     options.Model,
		KeepAlive: strings.TrimSpace(options.KeepAlive),
		Options:   ollamaPrewarmLoadOptions{NumCtx: options.NumCtx},
	}
	var response ollamaPrewarmResponse
	if err := ollamaPrewarmJSON(ctx, http.MethodPost, baseURL+"/api/generate", request, &response); err != nil {
		return report, fmt.Errorf("ollama prewarm load: %w", err)
	}
	if !response.Done {
		return report, fmt.Errorf("ollama prewarm response did not complete")
	}
	if response.Response != "" || response.Thinking != "" ||
		response.PromptEvalCount != 0 || response.EvalCount != 0 {
		return report, fmt.Errorf("ollama prewarm unexpectedly performed model inference")
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
			TotalDurationMS: response.TotalDuration / int64(time.Millisecond),
			LoadDurationMS:  response.LoadDuration / int64(time.Millisecond),
			AllocatedBytes:  candidate.Size, VRAMBytes: candidate.SizeVRAM,
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
	callCtx, cancel := ollamaPrewarmRequestContext(ctx)
	defer cancel()
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(callCtx, method, endpoint, body)
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

func ollamaPrewarmRequestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, llm.MaximumModelRequestDuration)
}

func ollamaPrewarmDefaultModel() string {
	return strings.TrimSpace(os.Getenv("OMNI_CODING_FRAGMENT_MODEL"))
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
		"ollama prewarm: ok model=%s context=%d load_ms=%d total_ms=%d allocated_gib=%.2f vram_gib=%.2f offload=%.1f%% keep_alive=%s\n",
		report.Model,
		report.ContextTokens,
		report.LoadDurationMS,
		report.TotalDurationMS,
		float64(report.AllocatedBytes)/(1024*1024*1024),
		float64(report.VRAMBytes)/(1024*1024*1024),
		report.OffloadPercent,
		report.KeepAlive,
	)
}
